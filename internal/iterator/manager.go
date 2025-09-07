// Package iterator provides stateful network iteration for MaxMind databases.
package iterator

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

// ManagedIterator represents a stateful iterator with metadata.
type ManagedIterator struct {
	Created      time.Time
	LastAccess   time.Time
	networks     chan maxminddb.Result
	stop         chan struct{}
	FilterEngine *filter.Engine
	Reader       *maxminddb.Reader
	Network      netip.Prefix
	LastNetwork  netip.Prefix
	FilterMode   string
	Database     string
	ID           string
	Filters      []filter.Filter
	Processed    int64
	Matched      int64
	done         bool
}

// ResumeToken contains information needed to resume iteration.
type ResumeToken struct {
	LastNetwork string          `json:"last_network"`
	Database    string          `json:"database"`
	Network     string          `json:"network"`
	FilterMode  string          `json:"filter_mode"`
	Filters     []filter.Filter `json:"filters"`
	Processed   int64           `json:"processed"`
	Matched     int64           `json:"matched"`
}

// NetworkResult represents a single network result.
type NetworkResult struct {
	Data    map[string]any `json:"data"`
	Network netip.Prefix   `json:"network"`
}

// IterationResult contains the results of an iteration batch.
type IterationResult struct {
	IteratorID         string          `json:"iterator_id"`
	ResumeToken        string          `json:"resume_token"`
	Results            []NetworkResult `json:"results"`
	TotalProcessed     int64           `json:"total_processed"`
	TotalMatched       int64           `json:"total_matched"`
	EstimatedRemaining int64           `json:"estimated_remaining,omitempty"`
	HasMore            bool            `json:"has_more"`
}

// Manager manages stateful network iterators.
type Manager struct {
	iterators       map[string]*ManagedIterator
	stopCleanup     chan struct{}
	ttl             time.Duration
	cleanupInterval time.Duration
	bufferSize      int
	mu              sync.RWMutex
}

// New creates a new iterator manager.
func New(ttl, cleanupInterval time.Duration, bufferSize int) *Manager {
	return &Manager{
		iterators:       make(map[string]*ManagedIterator),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
		bufferSize:      bufferSize,
		stopCleanup:     make(chan struct{}),
	}
}

// StartCleanup starts the cleanup goroutine.
func (m *Manager) StartCleanup() {
	go func() {
		ticker := time.NewTicker(m.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCleanup:
				return
			case <-ticker.C:
				m.cleanupExpired()
			}
		}
	}()
}

// StopCleanup stops the cleanup goroutine.
func (m *Manager) StopCleanup() {
	close(m.stopCleanup)
}

// CreateIterator creates a new iterator for a network range.
func (m *Manager) CreateIterator(
	reader *maxminddb.Reader,
	database string,
	network netip.Prefix,
	filters []filter.Filter,
	filterMode string,
) (*ManagedIterator, error) {
	iterator, err := m.createIteratorNoStart(reader, database, network, filters, filterMode)
	if err != nil {
		return nil, err
	}

	// Start streaming networks in the background
	go iterator.startStreaming()

	return iterator, nil
}

// ResumeIterator creates a new iterator from a resume token.
func (m *Manager) ResumeIterator(reader *maxminddb.Reader, token string) (*ManagedIterator, error) {
	// Parse resume token
	resumeToken, err := m.parseResumeToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid resume token: %w", err)
	}

	// Parse network
	network, err := netip.ParsePrefix(resumeToken.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network in resume token: %w", err)
	}

	// Create iterator without starting streaming
	iterator, err := m.createIteratorNoStart(
		reader,
		resumeToken.Database,
		network,
		resumeToken.Filters,
		resumeToken.FilterMode,
	)
	if err != nil {
		return nil, err
	}

	// Restore state from token
	iterator.Processed = resumeToken.Processed
	iterator.Matched = resumeToken.Matched

	// Restore last network if available for resume point
	if resumeToken.LastNetwork != "" {
		lastNetwork, err := netip.ParsePrefix(resumeToken.LastNetwork)
		if err != nil {
			return nil, fmt.Errorf("invalid last network in resume token: %w", err)
		}
		iterator.LastNetwork = lastNetwork
	}

	// Start streaming from the resume point (only once, with proper state)
	go iterator.startStreaming()

	return iterator, nil
}

// GetIterator retrieves an existing iterator by ID.
func (m *Manager) GetIterator(id string) (*ManagedIterator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	iterator, exists := m.iterators[id]
	if exists {
		iterator.LastAccess = time.Now()
	}

	return iterator, exists
}

// Iterate performs one iteration batch using streaming.
func (m *Manager) Iterate(iterator *ManagedIterator, maxResults int) (*IterationResult, error) {
	if iterator == nil {
		return nil, errors.New("iterator cannot be nil")
	}

	iterator.LastAccess = time.Now()

	results := make([]NetworkResult, 0, maxResults)

	// Stream results from the channel
	for len(results) < maxResults {
		select {
		case result, ok := <-iterator.networks:
			if !ok {
				// Channel closed, no more results
				iterator.done = true
				break
			}

			iterator.Processed++

			// Decode the result data
			var record map[string]any
			if err := result.Decode(&record); err != nil {
				// Skip records that can't be decoded
				continue
			}

			iterator.LastNetwork = result.Prefix()

			// Apply filters if present
			if iterator.FilterEngine != nil {
				if !iterator.FilterEngine.Matches(record) {
					continue // Skip non-matching records
				}
			}

			iterator.Matched++

			results = append(results, NetworkResult{
				Network: result.Prefix(),
				Data:    record,
			})
		default:
			// No more results available immediately, exit the loop
			goto endLoop
		}
	}

endLoop:
	// Generate resume token
	resumeToken, err := m.generateResumeToken(iterator)
	if err != nil {
		return nil, fmt.Errorf("failed to generate resume token: %w", err)
	}

	return &IterationResult{
		Results:            results,
		IteratorID:         iterator.ID,
		ResumeToken:        resumeToken,
		HasMore:            !iterator.done,
		TotalProcessed:     iterator.Processed,
		TotalMatched:       iterator.Matched,
		EstimatedRemaining: -1, // Can't estimate with streaming
	}, nil
}

// RemoveIterator removes an iterator.
func (m *Manager) RemoveIterator(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if iter, exists := m.iterators[id]; exists {
		// Signal the streaming goroutine to stop
		if !iter.done {
			close(iter.stop)
			iter.done = true
		}
		delete(m.iterators, id)
	}
}

// cleanupExpired removes expired iterators.
func (m *Manager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, iterator := range m.iterators {
		if now.Sub(iterator.LastAccess) > m.ttl {
			// Signal the streaming goroutine to stop
			if !iterator.done {
				close(iterator.stop)
				iterator.done = true
			}
			delete(m.iterators, id)
		}
	}
}

// generateResumeToken creates a resume token from the current iterator state.
func (*Manager) generateResumeToken(iterator *ManagedIterator) (string, error) {
	token := ResumeToken{
		Database:   iterator.Database,
		Network:    iterator.Network.String(),
		Filters:    iterator.Filters,
		FilterMode: iterator.FilterMode,
		Processed:  iterator.Processed,
		Matched:    iterator.Matched,
	}

	if iterator.LastNetwork.IsValid() {
		token.LastNetwork = iterator.LastNetwork.String()
	}

	data, err := json.Marshal(token)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// parseResumeToken parses a resume token.
func (*Manager) parseResumeToken(tokenStr string) (*ResumeToken, error) {
	data, err := base64.StdEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, err
	}

	var token ResumeToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// createIteratorNoStart creates a new iterator without starting streaming.
func (m *Manager) createIteratorNoStart(
	reader *maxminddb.Reader,
	database string,
	network netip.Prefix,
	filters []filter.Filter,
	filterMode string,
) (*ManagedIterator, error) {
	// Normalize filter mode to "and" or "or", default to "and"
	normalizedMode := normalizeFilterMode(filterMode)

	// Create filter engine
	var filterEngine *filter.Engine
	if len(filters) > 0 {
		filterEngine = filter.New(filters, filter.Mode(normalizedMode))
	}

	// Generate unique ID
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate iterator ID: %w", err)
	}

	iterator := &ManagedIterator{
		ID:           id,
		Reader:       reader,
		Database:     database,
		Network:      network,
		Filters:      filters,
		FilterMode:   normalizedMode,
		FilterEngine: filterEngine,
		Created:      time.Now(),
		LastAccess:   time.Now(),
		networks:     make(chan maxminddb.Result, m.bufferSize), // Configurable buffered channel
		stop:         make(chan struct{}),
		done:         false,
	}

	m.mu.Lock()
	m.iterators[id] = iterator
	m.mu.Unlock()

	return iterator, nil
}

// generateID generates a random iterator ID.
func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// normalizeFilterMode normalizes filter mode to "and" or "or", defaulting to "and".
func normalizeFilterMode(mode string) string {
	switch strings.ToLower(mode) {
	case string(filter.ModeAnd):
		return string(filter.ModeAnd)
	case string(filter.ModeOr):
		return string(filter.ModeOr)
	default:
		return string(filter.ModeAnd) // Default to "and" for unknown values
	}
}

// startStreaming begins streaming networks from the MMDB reader to the channel.
func (iter *ManagedIterator) startStreaming() {
	defer func() {
		iter.done = true
		close(iter.networks)
	}()

	// If we have a LastNetwork from resume token, we need to skip to that point
	skipUntil := iter.LastNetwork
	skipping := skipUntil.IsValid()

	for result := range iter.Reader.NetworksWithin(iter.Network) {
		// Check if we should stop early
		if iter.done {
			break
		}

		// Skip networks until we reach the resume point
		if skipping {
			if result.Prefix() != skipUntil {
				continue
			}
			skipping = false
			// Don't continue here - we want to emit the resume network
		}

		select {
		case iter.networks <- result:
			// Successfully sent result
		case <-iter.stop:
			// Iterator was stopped, exit goroutine
			return
		default:
			// Channel buffer is full, block until space is available or stop signal
			select {
			case iter.networks <- result:
				// Successfully sent result
			case <-iter.stop:
				// Iterator was stopped, exit goroutine
				return
			}
		}
	}
}
