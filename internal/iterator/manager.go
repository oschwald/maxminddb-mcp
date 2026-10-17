// Package iterator provides stateful network iteration for MaxMind databases.
package iterator

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

// MaxResults limits the number of records returned in one page.
const MaxResults = 1000

// MaxIterators limits the retained queries in one manager.
const MaxIterators = 128

// ErrIteratorLimit means all retained query slots are in use.
var ErrIteratorLimit = errors.New(
	"iterator limit reached. Reuse an existing iterator or wait for idle iterators to expire",
)

// ManagedIterator represents a stateful iterator with metadata.
type ManagedIterator struct {
	LastAccess   time.Time
	Created      time.Time
	next         func() (maxminddb.Result, bool)
	FilterEngine *filter.Engine
	Reader       *maxminddb.Reader
	pending      *NetworkResult
	stop         func()
	busy         chan struct{}
	Network      netip.Prefix
	LastNetwork  netip.Prefix
	ID           string
	Database     string
	FilterMode   string
	Filters      []filter.Filter
	Processed    int64
	Matched      int64
	exhausted    bool
	closed       bool
}

// ResumeToken contains information needed to resume iteration.
type ResumeToken struct {
	DatabaseType string          `json:"database_type"`
	LastNetwork  string          `json:"last_network"`
	Database     string          `json:"database"`
	Network      string          `json:"network"`
	FilterMode   string          `json:"filter_mode"`
	Filters      []filter.Filter `json:"filters"`
	Version      int             `json:"version"`
	BuildEpoch   uint            `json:"build_epoch"`
	Processed    int64           `json:"processed"`
	Matched      int64           `json:"matched"`
	Exhausted    bool            `json:"exhausted"`
}

// NetworkResult represents a single network result.
type NetworkResult struct {
	Data    any          `json:"data"`
	Network netip.Prefix `json:"network"`
}

// IterationResult contains the results of an iteration batch.
type IterationResult struct {
	IteratorID     string          `json:"iterator_id"`
	ResumeToken    string          `json:"resume_token"`
	Results        []NetworkResult `json:"results"`
	TotalProcessed int64           `json:"total_processed"`
	TotalMatched   int64           `json:"total_matched"`
	HasMore        bool            `json:"has_more"`
}

// Manager manages stateful network iterators.
type Manager struct {
	iterators       map[string]*ManagedIterator
	stopCleanup     chan struct{}
	ttl             time.Duration
	cleanupInterval time.Duration
	mu              sync.RWMutex
	stopOnce        sync.Once
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
	m.stopOnce.Do(func() { close(m.stopCleanup) })
	m.mu.Lock()
	iterators := m.iterators
	m.iterators = make(map[string]*ManagedIterator)
	m.mu.Unlock()
	for _, iterator := range iterators {
		iterator.busy <- struct{}{}
		iterator.close()
		<-iterator.busy
	}
}

// CreateIterator creates a new iterator for a network range.
func (m *Manager) CreateIterator(
	reader *maxminddb.Reader,
	database string,
	network netip.Prefix,
	filters []filter.Filter,
	filterMode string,
) (*ManagedIterator, error) {
	iterator, err := m.newIterator(reader, database, network, filters, filterMode)
	if err != nil {
		return nil, err
	}
	return m.registerIterator(iterator)
}

// ResumeIterator creates a new iterator from a resume token.
func (m *Manager) ResumeIterator(
	reader *maxminddb.Reader,
	database, token string,
) (*ManagedIterator, error) {
	resume, err := m.parseResumeToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid resume token: %w", err)
	}
	if resume.Version != 1 || resume.Database != database || reader == nil ||
		resume.BuildEpoch != reader.Metadata.BuildEpoch || resume.DatabaseType != reader.Metadata.DatabaseType {
		return nil, errors.New(
			"resume token belongs to a different database or version; start a new query",
		)
	}
	network, err := netip.ParsePrefix(resume.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network in resume token: %w", err)
	}
	var last netip.Prefix
	if resume.LastNetwork != "" {
		last, err = netip.ParsePrefix(resume.LastNetwork)
		if err != nil || last != last.Masked() || !last.Overlaps(network) {
			return nil, errors.New("invalid last network in resume token")
		}
	}
	if resume.Processed < 0 || resume.Matched < 0 || resume.Matched > resume.Processed ||
		(resume.Processed > 0) != last.IsValid() {
		return nil, errors.New("invalid counters in resume token")
	}
	iterator, err := m.newIterator(reader, database, network, resume.Filters, resume.FilterMode)
	if err != nil {
		return nil, err
	}
	iterator.Processed, iterator.Matched = resume.Processed, resume.Matched
	iterator.LastNetwork, iterator.exhausted = last, resume.Exhausted
	return m.registerIterator(iterator)
}

func (m *Manager) registerIterator(iterator *ManagedIterator) (*ManagedIterator, error) {
	m.cleanupExpired()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.iterators) >= MaxIterators {
		return nil, ErrIteratorLimit
	}
	m.iterators[iterator.ID] = iterator
	return iterator, nil
}

// GetIterator retrieves an iterator if its idle timeout has not expired.
func (m *Manager) GetIterator(id string) (*ManagedIterator, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	iterator, exists := m.iterators[id]
	if !exists {
		return nil, false
	}
	select {
	case iterator.busy <- struct{}{}:
		defer func() { <-iterator.busy }()
	default:
		// A running page is active. Its caller refreshes LastAccess when it finishes.
		return iterator, true
	}
	if time.Since(iterator.LastAccess) > m.ttl {
		iterator.close()
		delete(m.iterators, id)
		return nil, false
	}
	iterator.LastAccess = time.Now()
	return iterator, true
}

// Iterate returns a page and retains one matching record to detect the end.
func (m *Manager) Iterate(
	ctx context.Context,
	iterator *ManagedIterator,
	maxResults int,
	report func(string),
) (_ *IterationResult, err error) {
	if iterator == nil {
		return nil, errors.New("iterator cannot be nil")
	}
	if maxResults < 1 || maxResults > MaxResults {
		return nil, fmt.Errorf("max_results must be between 1 and %d", MaxResults)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case iterator.busy <- struct{}{}:
	}
	defer func() { <-iterator.busy }()
	if iterator.closed {
		return nil, errors.New("iterator expired; resume with its token or start a new query")
	}
	last, processed, matched, exhausted := iterator.LastNetwork, iterator.Processed, iterator.Matched, iterator.exhausted
	defer func() {
		iterator.LastAccess = time.Now()
		if err != nil {
			// Retry from the last successful page after cancellation or a decode error.
			iterator.resetCursor()
			iterator.LastNetwork, iterator.Processed, iterator.Matched, iterator.exhausted = last, processed, matched, exhausted
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if report != nil {
		report("Scanning " + iterator.Network.String())
	}
	if iterator.next == nil && !iterator.exhausted {
		iterator.next, iterator.stop = iter.Pull(iterator.Reader.NetworksWithin(iterator.Network))
		if iterator.LastNetwork.IsValid() {
			found := false
			var scanned int64
			for !found {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				result, ok := iterator.next()
				if !ok {
					return nil, errors.New(
						"resume position is absent from the database; start a new query",
					)
				}
				if err := result.Err(); err != nil {
					return nil, err
				}
				found = result.Prefix() == iterator.LastNetwork
				scanned++
				if report != nil && scanned%1024 == 0 {
					report(fmt.Sprintf("Restoring cursor: %d records scanned", scanned))
				}
			}
		}
	}
	results := make([]NetworkResult, 0, maxResults)
	for !iterator.exhausted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record := iterator.pending
		if record == nil {
			result, ok := iterator.next()
			if !ok {
				iterator.exhausted = true
				iterator.resetCursor()
				break
			}
			var data any
			if err := result.Decode(&data); err != nil {
				return nil, fmt.Errorf("decode network: %w", err)
			}
			record = &NetworkResult{Network: result.Prefix(), Data: data}
		}
		matches := iterator.FilterEngine == nil || iterator.FilterEngine.Matches(record.Data)
		if matches && len(results) == maxResults {
			iterator.pending = record
			break
		}
		iterator.pending = nil
		if iterator.Processed == math.MaxInt64 {
			return nil, errors.New("iterator counter overflow; start a new query")
		}
		iterator.Processed++
		if report != nil && iterator.Processed%1024 == 0 {
			report(
				fmt.Sprintf(
					"Scanning %s: %d records processed",
					iterator.Network,
					iterator.Processed,
				),
			)
		}
		iterator.LastNetwork = record.Network
		if matches {
			iterator.Matched++
			results = append(results, *record)
		}
	}
	token, err := m.generateResumeToken(iterator)
	if err != nil {
		return nil, fmt.Errorf("encode resume token: %w", err)
	}
	return &IterationResult{
		Results:        results,
		IteratorID:     iterator.ID,
		ResumeToken:    token,
		HasMore:        !iterator.exhausted,
		TotalProcessed: iterator.Processed,
		TotalMatched:   iterator.Matched,
	}, nil
}

func (iterator *ManagedIterator) resetCursor() {
	if iterator.stop != nil {
		iterator.stop()
	}
	iterator.next, iterator.stop, iterator.pending = nil, nil, nil
}

func (iterator *ManagedIterator) close() {
	iterator.resetCursor()
	iterator.closed = true
}

// RemoveIterator removes an iterator.
func (m *Manager) RemoveIterator(id string) {
	m.mu.Lock()
	iterator := m.iterators[id]
	delete(m.iterators, id)
	m.mu.Unlock()
	if iterator != nil {
		iterator.busy <- struct{}{}
		iterator.close()
		<-iterator.busy
	}
}

// cleanupExpired removes expired iterators.
func (m *Manager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, iterator := range m.iterators {
		select {
		case iterator.busy <- struct{}{}:
		default:
			continue
		}
		if now.Sub(iterator.LastAccess) > m.ttl {
			iterator.close()
			delete(m.iterators, id)
		}
		<-iterator.busy
	}
}

// generateResumeToken creates a resume token from the current iterator state.
func (*Manager) generateResumeToken(iterator *ManagedIterator) (string, error) {
	lastNetwork := iterator.LastNetwork

	token := ResumeToken{
		Version: 1, BuildEpoch: iterator.Reader.Metadata.BuildEpoch,
		DatabaseType: iterator.Reader.Metadata.DatabaseType, Exhausted: iterator.exhausted,
		Database:   iterator.Database,
		Network:    iterator.Network.String(),
		Filters:    iterator.Filters,
		FilterMode: iterator.FilterMode,
		Processed:  iterator.Processed,
		Matched:    iterator.Matched,
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
	if len(tokenStr) > 1024*1024 {
		return nil, errors.New("resume token exceeds 1 MiB")
	}
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

// newIterator validates a query before it is registered.
func (*Manager) newIterator(
	reader *maxminddb.Reader,
	database string,
	network netip.Prefix,
	filters []filter.Filter,
	filterMode string,
) (*ManagedIterator, error) {
	if reader == nil || !network.IsValid() {
		return nil, errors.New("a reader and valid network are required")
	}
	normalizedMode := strings.ToLower(filterMode)
	if normalizedMode == "" {
		normalizedMode = "and"
	}
	if normalizedMode != "and" && normalizedMode != "or" {
		return nil, errors.New("filter_mode must be and or or")
	}
	network = network.Masked()

	// Create filter engine
	var filterEngine *filter.Engine
	if len(filters) > 0 {
		// Normalize operator aliases (e.g., eq -> equals)
		norm := make([]filter.Filter, 0, len(filters))
		for _, f := range filters {
			f.Operator = normalizeOperator(f.Operator)
			norm = append(norm, f)
		}
		if err := filter.Validate(norm); err != nil {
			return nil, err
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
		busy:         make(chan struct{}, 1),
		Reader:       reader,
		Database:     database,
		Network:      network,
		Filters:      filters,
		FilterMode:   normalizedMode,
		FilterEngine: filterEngine,
		Created:      time.Now(),
		LastAccess:   time.Now(),
	}

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
