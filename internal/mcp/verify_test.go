package mcp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	sdk "github.com/mark3labs/mcp-go/mcp"
)

func TestVerifyDatabase(t *testing.T) {
	server := comparisonServer(t)
	call := modernTools(t, server)
	payload := wireContent(t, call("verify_database", map[string]any{"database": "old.mmdb"}))
	if payload["valid"] != true || payload["database"] != "old.mmdb" ||
		payload["completed_after_cancellation"] != false {
		t.Fatalf("verification: %v", payload)
	}
	for _, filename := range []string{"MaxMind-DB-test-broken-pointers-24.mmdb", "MaxMind-DB-test-broken-search-tree-24.mmdb"} {
		if err := server.dbManager.LoadDatabase(
			"../../testdata/test-data/" + filename,
		); err != nil {
			t.Fatal(err)
		}
		result, err := server.handleVerifyDatabase(
			t.Context(),
			networkRequest(map[string]any{"database": filename}),
		)
		assertToolError(t, result, err, "verification_failed")
	}
	result, err := server.handleVerifyDatabase(
		t.Context(),
		networkRequest(map[string]any{"database": "missing"}),
	)
	assertToolError(t, result, err, "db_not_found")
	server.verificationGate <- struct{}{}
	t.Cleanup(func() { <-server.verificationGate })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = server.handleVerifyDatabase(
		ctx,
		networkRequest(map[string]any{"database": "old.mmdb"}),
	)
	assertToolError(t, result, err, "canceled")
}

func TestVerificationRunsToCompletionAfterCancellation(t *testing.T) {
	server := testServer(t)
	for _, failure := range []error{nil, errors.New("corrupt database")} {
		ctx, cancel := context.WithCancel(t.Context())
		finished := false
		result := server.verifyDatabase(ctx, "test.mmdb", func() error {
			cancel()
			finished = true
			return failure
		}, nil)
		cancel()
		if !finished {
			t.Fatal("verification did not finish")
		}
		if len(server.verificationGate) != 0 {
			t.Fatal("completed verification did not release its slot")
		}
		if failure == nil {
			payload := result.StructuredContent.(verificationResult)
			if !payload.Valid || !payload.CompletedAfterCancellation || result.IsError {
				t.Fatalf("incorrect verification: %+v", payload)
			}
		} else {
			assertToolError(t, result, nil, "verification_failed")
			message := result.StructuredContent.(map[string]any)["error"].(map[string]any)["message"].(string)
			if !strings.Contains(message, "ran to completion") {
				t.Fatalf("incorrect cancellation message: %s", message)
			}
		}
	}
}

func TestVerificationQueue(t *testing.T) {
	server := testServer(t)
	synctest.Test(t, func(t *testing.T) {
		server.verificationGate = make(chan struct{}, 1)
		finish := make(chan struct{})
		release := sync.OnceFunc(func() { close(finish) })
		defer release()
		first := make(chan *sdk.CallToolResult, 1)
		go func() {
			first <- server.verifyDatabase(t.Context(), "first", func() error {
				<-finish
				return nil
			}, nil)
		}()
		synctest.Wait()
		if len(server.verificationGate) != 1 || len(first) != 0 {
			t.Fatal("active verification did not retain its slot")
		}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		canceled := make(chan *sdk.CallToolResult, 1)
		canceledStarted := false
		go func() {
			canceled <- server.verifyDatabase(ctx, "canceled", func() error {
				canceledStarted = true
				return nil
			}, nil)
		}()
		second := make(chan *sdk.CallToolResult, 1)
		secondStarted := false
		go func() {
			second <- server.verifyDatabase(t.Context(), "second", func() error {
				secondStarted = true
				return nil
			}, nil)
		}()
		synctest.Wait()
		if canceledStarted || secondStarted || len(canceled) != 0 || len(second) != 0 {
			t.Fatal("queued verification ran before the active request finished")
		}

		cancel()
		synctest.Wait()
		select {
		case result := <-canceled:
			assertToolError(t, result, nil, "canceled")
		default:
			t.Fatal("queued verification did not cancel")
		}
		if canceledStarted || secondStarted || len(first) != 0 ||
			len(server.verificationGate) != 1 {
			t.Fatal("canceling a waiter changed the active verification")
		}

		release()
		synctest.Wait()
		for _, results := range []chan *sdk.CallToolResult{first, second} {
			select {
			case result := <-results:
				if result.IsError || !result.StructuredContent.(verificationResult).Valid {
					t.Fatalf("verification failed: %+v", result)
				}
			default:
				t.Fatal("verification did not finish after the slot was released")
			}
		}
		if !secondStarted || len(server.verificationGate) != 0 {
			t.Fatal("verification did not release its slot")
		}
		result, err := server.handleVerifyDatabase(
			t.Context(),
			networkRequest(map[string]any{"database": "GeoLite2-ASN-Test.mmdb"}),
		)
		if err != nil || result.IsError || !result.StructuredContent.(verificationResult).Valid {
			t.Fatalf("verification slot could not be reused: %+v, %v", result, err)
		}
	})
}
