package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
)

func TestInitializeWhileDownloadIsBlocked(t *testing.T) {
	started := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer endpoint.Close()
	cfg := config.DefaultConfig()
	cfg.MaxMind.AccountID, cfg.MaxMind.LicenseKey = 1, "test"
	cfg.MaxMind.Editions = []string{"GeoLite2-ASN"}
	cfg.MaxMind.DatabaseDir, cfg.MaxMind.Endpoint = t.TempDir(), endpoint.URL
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	input, clientInput := io.Pipe()
	clientOutput, output := io.Pipe()
	defer func() { _ = input.Close() }()
	defer func() { _ = clientInput.Close() }()
	defer func() { _ = clientOutput.Close() }()
	defer func() { _ = output.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, input, output) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("initial download did not start")
	}
	responses := make(chan map[string]any, 1)
	go func() {
		if _, err := fmt.Fprintln(
			clientInput,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		); err != nil {
			return
		}
		var response map[string]any
		if err := json.NewDecoder(clientOutput).Decode(&response); err == nil {
			responses <- response
		}
	}()
	select {
	case response := <-responses:
		if response["error"] != nil || response["result"] == nil {
			t.Fatalf("initialize failed: %v", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initialize blocked on database download")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel download and stdio")
	}
}
