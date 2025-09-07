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
	mu           sync.RWMutex
}

// getLastNetwork safely gets the LastNetwork field.
func (iter *ManagedIterator) getLastNetwork() netip.Prefix {
	iter.mu.RLock()
	defer iter.mu.RUnlock()
	return iter.LastNetwork
}

// setLastNetwork safely sets the LastNetwork field.
func (iter *ManagedIterator) setLastNetwork(network netip.Prefix) {
	iter.mu.Lock()
	defer iter.mu.Unlock()
	iter.LastNetwork = network
}

// getProcessedMatched safely gets the Processed and Matched counters.
func (iter *ManagedIterator) getProcessedMatched() (processed, matched int64) {
	iter.mu.RLock()
	defer iter.mu.RUnlock()
	return iter.Processed, iter.Matched
}

// updateCounters safely updates the Processed and Matched counters.
func (iter *ManagedIterator) updateCounters(processed, matched int64) {
	iter.mu.Lock()
	defer iter.mu.Unlock()
	iter.Processed = processed
	iter.Matched = matched
}

// incrementProcessed safely increments the Processed counter.
func (iter *ManagedIterator) incrementProcessed() {
	iter.mu.Lock()
	defer iter.mu.Unlock()
	iter.Processed++
}

// incrementMatched safely increments the Matched counter.
func (iter *ManagedIterator) incrementMatched() {
	iter.mu.Lock()
	defer iter.mu.Unlock()
	iter.Matched++
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
	iterator, err := m.createIteratorNoStart(reader, database, network, filters, filterMode)
	if err != nil {
		return nil, err
	}
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

	// Create iterator from resume token state
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
	iterator.updateCounters(resumeToken.Processed, resumeToken.Matched)

	// Restore last network if available for resume point
	if resumeToken.LastNetwork != "" {
		lastNetwork, err := netip.ParsePrefix(resumeToken.LastNetwork)
		if err != nil {
			return nil, fmt.Errorf("invalid last network in resume token: %w", err)
		}
		iterator.setLastNetwork(lastNetwork)
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

// Iterate performs one iteration batch over the reader.
func (m *Manager) Iterate(iterator *ManagedIterator, maxResults int) (*IterationResult, error) {
	if iterator == nil {
		return nil, errors.New("iterator cannot be nil")
	}

	iterator.LastAccess = time.Now()

	results := make([]NetworkResult, 0, maxResults)

	// Pull results directly from the reader, supporting resume via LastNetwork
	skipUntil := iterator.getLastNetwork()
	skipping := skipUntil.IsValid()
	hasMore := false

	for result := range iterator.Reader.NetworksWithin(iterator.Network) {
		// Resume point handling: include LastNetwork again for continuity
		if skipping {
			if result.Prefix() != skipUntil {
				continue
			}
			skipping = false
		}

		iterator.incrementProcessed()

		// Decode the result data
		var record map[string]any
		if err := result.Decode(&record); err != nil {
			// Skip records that can't be decoded
			iterator.setLastNetwork(result.Prefix())
			continue
		}

		iterator.setLastNetwork(result.Prefix())

		// Apply filters if present
		if iterator.FilterEngine != nil {
			if !iterator.FilterEngine.Matches(record) {
				if len(results) >= maxResults {
					hasMore = true
					break
				}
				continue // Skip non-matching records
			}
		}

		iterator.incrementMatched()

		results = append(results, NetworkResult{
			Network: result.Prefix(),
			Data:    record,
		})

		if len(results) >= maxResults {
			hasMore = true
			break
		}
	}

	// Generate resume token
	resumeToken, err := m.generateResumeToken(iterator)
	if err != nil {
		return nil, fmt.Errorf("failed to generate resume token: %w", err)
	}

	totalProcessed, totalMatched := iterator.getProcessedMatched()

	return &IterationResult{
		Results:            results,
		IteratorID:         iterator.ID,
		ResumeToken:        resumeToken,
		HasMore:            hasMore,
		TotalProcessed:     totalProcessed,
		TotalMatched:       totalMatched,
		EstimatedRemaining: 0,
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
	processed, matched := iterator.getProcessedMatched()
	lastNetwork := iterator.getLastNetwork()

	token := ResumeToken{
		Database:   iterator.Database,
		Network:    iterator.Network.String(),
		Filters:    iterator.Filters,
		FilterMode: iterator.FilterMode,
		Processed:  processed,
		Matched:    matched,
	}

	if lastNetwork.IsValid() {
		token.LastNetwork = lastNetwork.String()
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

// createIteratorNoStart creates a new iterator without any background goroutines.
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
		// Normalize operator aliases (e.g., eq -> equals)
		norm := make([]filter.Filter, 0, len(filters))
		for _, f := range filters {
			f.Operator = normalizeOperator(f.Operator)
			norm = append(norm, f)
		}
		filterEngine = filter.New(norm, filter.Mode(normalizedMode))
		filters = norm
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

// normalizeOperator maps common aliases to canonical operator names used by the filter engine.
func normalizeOperator(op string) string {
	switch strings.ToLower(op) {
	case "eq":
		return "equals"
	case "ne":
		return "not_equals"
	case "gt":
		return "greater_than"
	case "lt":
		return "less_than"
	case "gte":
		return "greater_than_or_equal"
	case "lte":
		return "less_than_or_equal"
	default:
		return strings.ToLower(op)
	}
}
