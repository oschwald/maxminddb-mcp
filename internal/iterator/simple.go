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

// SimpleIterator is a basic iterator for network scanning.
type SimpleIterator struct {
	Created      time.Time
	LastAccess   time.Time
	FilterEngine *filter.Engine
	Network      netip.Prefix
	LastNetwork  netip.Prefix
	ID           string
	Database     string
	FilterMode   string
	Filters      []filter.Filter
	Processed    int64
	Matched      int64
}

// SimpleManager manages basic network iterators.
type SimpleManager struct {
	iterators       map[string]*SimpleIterator
	stopCleanup     chan struct{}
	ttl             time.Duration
	cleanupInterval time.Duration
	mu              sync.RWMutex
}

// NewSimple creates a new simple iterator manager.
func NewSimple(ttl, cleanupInterval time.Duration) *SimpleManager {
	return &SimpleManager{
		iterators:       make(map[string]*SimpleIterator),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
		stopCleanup:     make(chan struct{}),
	}
}

// CreateSimpleIterator creates a basic iterator.
func (m *SimpleManager) CreateSimpleIterator(
	database string,
	network netip.Prefix,
	filters []filter.Filter,
	filterMode string,
) (*SimpleIterator, error) {
	// Create filter engine
	var filterEngine *filter.Engine
	if len(filters) > 0 {
		filterEngine = filter.New(filters, filter.Mode(filterMode))
	}

	// Generate unique ID
	id, err := generateSimpleID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate iterator ID: %w", err)
	}

	iterator := &SimpleIterator{
		ID:           id,
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

// IterateSimple performs basic network iteration.
func (m *SimpleManager) IterateSimple(
	reader *maxminddb.Reader,
	iterator *SimpleIterator,
	maxResults int,
) (*IterationResult, error) {
	if reader == nil {
		return nil, errors.New("reader cannot be nil")
	}
	if iterator == nil {
		return nil, errors.New("iterator cannot be nil")
	}

	iterator.LastAccess = time.Now()

	results := make([]NetworkResult, 0, maxResults)

	// Use the new v2 API to iterate over networks within the prefix
	for result := range reader.NetworksWithin(iterator.Network) {
		if len(results) >= maxResults {
			break
		}

		iterator.Processed++
		iterator.LastNetwork = result.Prefix()

		// Decode the result data
		var record map[string]any
		if err := result.Decode(&record); err != nil {
			// Skip records that can't be decoded
			continue
		}

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
	resumeToken, err := m.generateSimpleResumeToken(iterator)
	if err != nil {
		return nil, fmt.Errorf("failed to generate resume token: %w", err)
	}

	hasMore := len(results) == maxResults // Heuristic

	return &IterationResult{
		Results:            results,
		IteratorID:         iterator.ID,
		ResumeToken:        resumeToken,
		HasMore:            hasMore,
		TotalProcessed:     iterator.Processed,
		TotalMatched:       iterator.Matched,
		EstimatedRemaining: 0,
	}, nil
}

// generateSimpleResumeToken creates a basic resume token.
func (*SimpleManager) generateSimpleResumeToken(iterator *SimpleIterator) (string, error) {
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

// generateSimpleID generates a random ID.
func generateSimpleID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
