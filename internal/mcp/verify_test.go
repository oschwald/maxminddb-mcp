package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	for _, failure := range []error{nil, errors.New("corrupt database")} {
		ctx, cancel := context.WithCancel(t.Context())
		finished := false
		result := verifyDatabase(ctx, "test.mmdb", func() error {
			cancel()
			finished = true
			return failure
		})
		cancel()
		if !finished {
			t.Fatal("verification did not finish")
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
