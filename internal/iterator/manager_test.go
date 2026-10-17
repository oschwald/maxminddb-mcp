package iterator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/filter"

	"github.com/oschwald/maxminddb-golang/v2"
)

func testIterator(t *testing.T) (*Manager, *ManagedIterator, []NetworkResult) {
	t.Helper()
	reader, err := maxminddb.Open("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	manager := New(time.Minute, time.Second)
	t.Cleanup(manager.StopCleanup)
	iterator, err := manager.CreateIterator(
		reader,
		"asn",
		netip.MustParsePrefix("1.0.0.0/8"),
		nil,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	var expected []NetworkResult
	for result := range reader.NetworksWithin(iterator.Network) {
		var data map[string]any
		if err := result.Decode(&data); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, NetworkResult{Network: result.Prefix(), Data: data})
	}
	if len(expected) < 2 {
		t.Fatal("fixture must have multiple networks")
	}
	return manager, iterator, expected
}

func TestPagination(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, size := range []int{1, 2, MaxResults} {
			t.Run(fmt.Sprintf("resume=%t/size=%d", resume, size), func(t *testing.T) {
				manager, iterator, expected := testIterator(t)
				var got []NetworkResult
				for page := 0; ; page++ {
					if page > len(expected) {
						t.Fatal("pagination did not finish")
					}
					result, err := manager.Iterate(t.Context(), iterator, size, nil)
					if err != nil {
						t.Fatal(err)
					}
					got = append(got, result.Results...)
					if result.TotalMatched != int64(len(got)) ||
						result.TotalProcessed != int64(len(got)) {
						t.Fatalf("incorrect counters: %+v", result)
					}
					if result.HasMore != (len(got) < len(expected)) {
						t.Fatalf("incorrect has_more at %d records", len(got))
					}
					if !result.HasMore {
						break
					}
					if resume {
						manager.RemoveIterator(iterator.ID)
						iterator, err = manager.ResumeIterator(
							iterator.Reader,
							iterator.Database,
							result.ResumeToken,
						)
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				if !reflect.DeepEqual(got, expected) {
					t.Fatalf("pages differ: got %d records, want %d", len(got), len(expected))
				}
				result, err := manager.Iterate(t.Context(), iterator, size, nil)
				if err != nil || len(result.Results) != 0 || result.HasMore {
					t.Fatalf("exhausted page: %+v, %v", result, err)
				}
			})
		}
	}
}

func TestPageBounds(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	for _, size := range []int{-1, 0, MaxResults + 1} {
		if _, err := manager.Iterate(t.Context(), iterator, size, nil); err == nil {
			t.Errorf("accepted size %d", size)
		}
	}
	if _, err := manager.Iterate(t.Context(), nil, 1, nil); err == nil {
		t.Error("accepted nil iterator")
	}
}

func TestScanProgress(t *testing.T) {
	manager, iterator, expected := testIterator(t)
	iterator.FilterEngine = filter.New([]filter.Filter{
		{Field: "absent", Operator: "exists", Value: true},
	}, filter.ModeAnd)
	remaining := 2048
	iterator.next = func() (maxminddb.Result, bool) {
		remaining--
		return iterator.Reader.Lookup(expected[0].Network.Addr()), remaining >= 0
	}
	var messages []string
	page, err := manager.Iterate(
		t.Context(),
		iterator,
		1,
		func(message string) { messages = append(messages, message) },
	)
	if err != nil || len(page.Results) != 0 || page.TotalProcessed != 2048 {
		t.Fatalf("scan with progress: %+v, %v", page, err)
	}
	if !slices.Equal(messages, []string{
		"Scanning 1.0.0.0/8",
		"Scanning 1.0.0.0/8: 1024 records processed",
		"Scanning 1.0.0.0/8: 2048 records processed",
	}) {
		t.Fatalf("unexpected scan progress: %v", messages)
	}
}

func TestIteratorLimit(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	page, err := manager.Iterate(t.Context(), iterator, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	create := func(resume bool) (*ManagedIterator, error) {
		if resume {
			return manager.ResumeIterator(iterator.Reader, iterator.Database, page.ResumeToken)
		}
		return manager.CreateIterator(
			iterator.Reader,
			iterator.Database,
			iterator.Network,
			nil,
			"and",
		)
	}
	var wg sync.WaitGroup
	for i := range 2 * MaxIterators {
		wg.Go(func() {
			if _, err := create(i%2 == 0); err != nil && !errors.Is(err, ErrIteratorLimit) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(manager.iterators) != MaxIterators {
		t.Fatalf("got %d iterators, want %d", len(manager.iterators), MaxIterators)
	}
	for _, resume := range []bool{false, true} {
		if _, err := create(resume); !errors.Is(err, ErrIteratorLimit) {
			t.Fatalf("resume=%t: expected iterator limit, got %v", resume, err)
		}
	}
	if _, err := manager.Iterate(t.Context(), iterator, 1, nil); err != nil {
		t.Fatalf("limit blocked an existing iterator: %v", err)
	}
	iterator.LastAccess = time.Now().Add(-2 * time.Minute)
	iterator.busy <- struct{}{}
	_, err = create(false)
	<-iterator.busy
	if !errors.Is(err, ErrIteratorLimit) {
		t.Fatalf("busy iterator was evicted: %v", err)
	}
	if _, err := create(true); err != nil {
		t.Fatalf("expired iterator did not free a slot: %v", err)
	}
	if !iterator.closed || iterator.next != nil || len(manager.iterators) != MaxIterators {
		t.Fatal("expired iterator was not released before admission")
	}
}

func TestCanceledPageCanBeRetried(t *testing.T) {
	manager, iterator, expected := testIterator(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	// Cancel after consuming a record to test rollback of a partial page.
	iterator.next = func() (maxminddb.Result, bool) {
		calls++
		if calls == 2 {
			cancel()
		}
		return iterator.Reader.Lookup(expected[calls-1].Network.Addr()), true
	}
	_, err := manager.Iterate(ctx, iterator, 2, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	result, err := manager.Iterate(t.Context(), iterator, MaxResults, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Results, expected) {
		t.Fatal("cancellation lost or duplicated records")
	}
}

func TestResumeValidation(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	result, err := manager.Iterate(t.Context(), iterator, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	original, err := manager.parseResumeToken(result.ResumeToken)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ResumeToken){
		"version":    func(token *ResumeToken) { token.Version = 0 },
		"database":   func(token *ResumeToken) { token.Database = "other" },
		"generation": func(token *ResumeToken) { token.BuildEpoch++ },
		"type":       func(token *ResumeToken) { token.DatabaseType = "other" },
		"network":    func(token *ResumeToken) { token.Network = "invalid" },
		"position":   func(token *ResumeToken) { token.LastNetwork = "8.8.8.0/24" },
		"counter":    func(token *ResumeToken) { token.Processed = -1 },
		"matched":    func(token *ResumeToken) { token.Matched = token.Processed + 1 },
		"mode":       func(token *ResumeToken) { token.FilterMode = "typo" },
	} {
		t.Run(name, func(t *testing.T) {
			token := *original
			mutate(&token)
			data, err := json.Marshal(token)
			if err != nil {
				t.Fatal(err)
			}
			_, err = manager.ResumeIterator(
				iterator.Reader,
				iterator.Database,
				base64.StdEncoding.EncodeToString(data),
			)
			if err == nil {
				t.Fatal("accepted invalid token")
			}
			if len(manager.iterators) != 1 {
				t.Fatal("invalid resume leaked an iterator")
			}
		})
	}
	original.LastNetwork = "1.255.255.0/24"
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.ResumeIterator(
		iterator.Reader,
		iterator.Database,
		base64.StdEncoding.EncodeToString(data),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Iterate(t.Context(), resumed, 1, nil); err == nil {
		t.Fatal("accepted absent resume position")
	}
}

func TestConcurrentPages(t *testing.T) {
	manager, iterator, expected := testIterator(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	got := make(map[netip.Prefix]bool)
	for range 4 {
		wg.Go(func() {
			for {
				current, ok := manager.GetIterator(iterator.ID)
				if !ok {
					t.Error("active iterator disappeared")
					return
				}
				result, err := manager.Iterate(t.Context(), current, 1, nil)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				for _, record := range result.Results {
					if got[record.Network] {
						t.Errorf("duplicate network %s", record.Network)
					}
					got[record.Network] = true
				}
				mu.Unlock()
				manager.cleanupExpired()
				if !result.HasMore {
					return
				}
			}
		})
	}
	wg.Wait()
	if len(got) != len(expected) {
		t.Fatalf("got %d records, want %d", len(got), len(expected))
	}
}

func TestBusyIteratorDoesNotBlockOtherQueries(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	iterator.busy <- struct{}{}
	defer func() { <-iterator.busy }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if current, ok := manager.GetIterator(iterator.ID); !ok || current != iterator {
			t.Error("busy iterator disappeared")
		}
		other, err := manager.CreateIterator(
			iterator.Reader,
			iterator.Database,
			iterator.Network,
			nil,
			"",
		)
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := manager.Iterate(t.Context(), other, 1, nil); err != nil {
			t.Error(err)
		}
		manager.cleanupExpired()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("busy iterator blocked another query")
	}
}

func TestQueuedPageCancellation(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	iterator.busy <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := manager.Iterate(ctx, iterator, 1, nil); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("queued page ignored cancellation")
	}
	<-iterator.busy
	page, err := manager.Iterate(t.Context(), iterator, 1, nil)
	if err != nil || len(page.Results) != 1 || page.TotalMatched != 1 {
		t.Fatalf("canceled waiter changed the next page: %+v, %v", page, err)
	}
}

func TestIteratorExpiry(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		manager, iterator, _ := testIterator(t)
		if _, err := manager.Iterate(t.Context(), iterator, 1, nil); err != nil {
			t.Fatal(err)
		}
		iterator.LastAccess = time.Now().Add(-2 * time.Minute)
		if cleanup {
			manager.cleanupExpired()
		}
		if _, ok := manager.GetIterator(iterator.ID); ok {
			t.Fatal("expired iterator returned")
		}
		if iterator.next != nil {
			t.Fatal("expired iterator retained cursor")
		}
		if _, err := manager.Iterate(t.Context(), iterator, 1, nil); err == nil {
			t.Fatal("used expired iterator")
		}
	}
}

func TestStopCleanup(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	manager.StartCleanup()
	if _, err := manager.Iterate(t.Context(), iterator, 1, nil); err != nil {
		t.Fatal(err)
	}
	manager.StopCleanup()
	manager.StopCleanup()
	if _, ok := manager.GetIterator(iterator.ID); ok {
		t.Fatal("stopped manager retained iterator")
	}
}
