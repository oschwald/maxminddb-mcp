// Package iterator provides stateful network iteration for MaxMind databases.
package iterator

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

// ManagedIterator represents a stateful iterator with metadata.
type ManagedIterator struct {
	LastAccess     time.Time
	Created        time.Time
	FilterEngine   *filter.Engine
	Reader         *maxminddb.Reader
	Network        netip.Prefix
	LastNetwork    netip.Prefix
	FilterMode     string
	ID             string
	Database       string
	Filters        []filter.Filter
	currentResults []maxminddb.Result
	Processed      int64
	Matched        int64
	resultIndex    int
	initialized    bool
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
	ResultIndex int             `json:"result_index"`
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
	mu              sync.RWMutex
}

// New creates a new iterator manager.
func New(ttl, cleanupInterval time.Duration) *Manager {
	return &Manager{
		iterators:       make(map[string]*ManagedIterator),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
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
	// Store reader for later iteration
	_ = reader // Reader will be used during iteration

	// Create filter engine
	var filterEngine *filter.Engine
	if len(filters) > 0 {
		filterEngine = filter.New(filters, filter.Mode(filterMode))
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
		FilterMode:   filterMode,
		FilterEngine: filterEngine,
		Created:      time.Now(),
		LastAccess:   time.Now(),
	}

	m.mu.Lock()
	m.iterators[id] = iterator
	m.mu.Unlock()

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

	// Create new iterator
	iterator, err := m.CreateIterator(
		reader,
		resumeToken.Database,
		network,
		resumeToken.Filters,
		resumeToken.FilterMode,
	)
	if err != nil {
		return nil, err
	}

	// Initialize the iterator and restore state
	iterator.currentResults = make([]maxminddb.Result, 0, 10000) // Pre-allocate reasonable size
	for result := range iterator.Reader.NetworksWithin(iterator.Network) {
		iterator.currentResults = append(iterator.currentResults, result)
	}
	iterator.initialized = true

	// Restore state from token
	iterator.Processed = resumeToken.Processed
	iterator.Matched = resumeToken.Matched
	iterator.resultIndex = resumeToken.ResultIndex

	// Restore last network if available
	if resumeToken.LastNetwork != "" {
		lastNetwork, err := netip.ParsePrefix(resumeToken.LastNetwork)
		if err != nil {
			return nil, fmt.Errorf("invalid last network in resume token: %w", err)
		}
		iterator.LastNetwork = lastNetwork
	}

	return iterator, nil
}

// GetIterator retrieves an existing iterator by ID.
func (m *Manager) GetIterator(id string) (*ManagedIterator, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	iterator, exists := m.iterators[id]
	if exists {
		iterator.LastAccess = time.Now()
	}

	return iterator, exists
}

// Iterate performs one iteration batch.
func (m *Manager) Iterate(iterator *ManagedIterator, maxResults int) (*IterationResult, error) {
	if iterator == nil {
		return nil, errors.New("iterator cannot be nil")
	}

	iterator.LastAccess = time.Now()

	results := make([]NetworkResult, 0, maxResults)

	// Initialize results if not done yet
	if !iterator.initialized {
		iterator.currentResults = make([]maxminddb.Result, 0, 10000) // Pre-allocate reasonable size
		for result := range iterator.Reader.NetworksWithin(iterator.Network) {
			iterator.currentResults = append(iterator.currentResults, result)
		}
		iterator.initialized = true
	}

	// Process results starting from current index
	for iterator.resultIndex < len(iterator.currentResults) && len(results) < maxResults {
		result := iterator.currentResults[iterator.resultIndex]
		iterator.resultIndex++
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
	}

	// Generate resume token
	resumeToken, err := m.generateResumeToken(iterator)
	if err != nil {
		return nil, fmt.Errorf("failed to generate resume token: %w", err)
	}

	hasMore := iterator.resultIndex < len(iterator.currentResults)

	return &IterationResult{
		Results:            results,
		IteratorID:         iterator.ID,
		ResumeToken:        resumeToken,
		HasMore:            hasMore,
		TotalProcessed:     iterator.Processed,
		TotalMatched:       iterator.Matched,
		EstimatedRemaining: int64(len(iterator.currentResults) - iterator.resultIndex),
	}, nil
}

// RemoveIterator removes an iterator.
func (m *Manager) RemoveIterator(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.iterators, id)
}

// cleanupExpired removes expired iterators.
func (m *Manager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, iterator := range m.iterators {
		if now.Sub(iterator.LastAccess) > m.ttl {
			delete(m.iterators, id)
		}
	}
}

// generateResumeToken creates a resume token from the current iterator state.
func (*Manager) generateResumeToken(iterator *ManagedIterator) (string, error) {
	token := ResumeToken{
		Database:    iterator.Database,
		Network:     iterator.Network.String(),
		Filters:     iterator.Filters,
		FilterMode:  iterator.FilterMode,
		Processed:   iterator.Processed,
		Matched:     iterator.Matched,
		ResultIndex: iterator.resultIndex,
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

// generateID generates a random iterator ID.
func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
