package iterator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"sync"
	"testing"
	"time"

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
					result, err := manager.Iterate(t.Context(), iterator, size)
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
				result, err := manager.Iterate(t.Context(), iterator, size)
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
		if _, err := manager.Iterate(t.Context(), iterator, size); err == nil {
			t.Errorf("accepted size %d", size)
		}
	}
	if _, err := manager.Iterate(t.Context(), nil, 1); err == nil {
		t.Error("accepted nil iterator")
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
	_, err := manager.Iterate(ctx, iterator, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	result, err := manager.Iterate(t.Context(), iterator, MaxResults)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Results, expected) {
		t.Fatal("cancellation lost or duplicated records")
	}
}

func TestResumeValidation(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	result, err := manager.Iterate(t.Context(), iterator, 1)
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
	if _, err := manager.Iterate(t.Context(), resumed, 1); err == nil {
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
				result, err := manager.Iterate(t.Context(), current, 1)
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

func TestIteratorExpiry(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		manager, iterator, _ := testIterator(t)
		if _, err := manager.Iterate(t.Context(), iterator, 1); err != nil {
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
		if _, err := manager.Iterate(t.Context(), iterator, 1); err == nil {
			t.Fatal("used expired iterator")
		}
	}
}

func TestStopCleanup(t *testing.T) {
	manager, iterator, _ := testIterator(t)
	manager.StartCleanup()
	if _, err := manager.Iterate(t.Context(), iterator, 1); err != nil {
		t.Fatal(err)
	}
	manager.StopCleanup()
	manager.StopCleanup()
	if _, ok := manager.GetIterator(iterator.ID); ok {
		t.Fatal("stopped manager retained iterator")
	}
}
