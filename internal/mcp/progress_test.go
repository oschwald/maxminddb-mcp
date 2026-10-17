package mcp

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
)

type progressSession struct {
	notifications chan sdk.JSONRPCNotification
}

func (*progressSession) Initialize()       {}
func (*progressSession) Initialized() bool { return true }
func (*progressSession) SessionID() string { return "progress-test" }
func (s *progressSession) NotificationChannel() chan<- sdk.JSONRPCNotification {
	return s.notifications
}

func TestProgressOptIn(t *testing.T) {
	server := testServer(t)
	for _, token := range []any{nil, true, 1.5, math.NaN(), math.Inf(1), map[string]any{}} {
		request := networkRequest(nil)
		request.Params.Meta = &sdk.Meta{ProgressToken: token}
		if server.progressReporter(t.Context(), request) != nil {
			t.Fatalf("accepted invalid or absent progress token: %v", token)
		}
	}
	if server.progressReporter(t.Context(), networkRequest(nil)) != nil {
		t.Fatal("enabled progress without request metadata")
	}
}

func TestProgressThrottlingAndCancellation(t *testing.T) {
	server := testServer(t)
	for _, token := range []any{"scan", float64(0)} {
		synctest.Test(t, func(t *testing.T) {
			session := &progressSession{notifications: make(chan sdk.JSONRPCNotification, 4)}
			ctx, cancel := context.WithCancel(server.mcp.WithContext(t.Context(), session))
			defer cancel()
			request := networkRequest(nil)
			request.Params.Meta = &sdk.Meta{ProgressToken: token}
			report := server.progressReporter(ctx, request)
			report("Scanning")
			report("Throttled")
			time.Sleep(time.Second)
			report("1024 records scanned")
			cancel()
			time.Sleep(time.Second)
			report("Canceled")
			if len(session.notifications) != 2 {
				t.Fatalf("got %d notifications, want 2", len(session.notifications))
			}
			for i, message := range []string{"Scanning", "1024 records scanned"} {
				notification := <-session.notifications
				fields := notification.Params.AdditionalFields
				if notification.Method != "notifications/progress" ||
					fields["progressToken"] != token ||
					fields["progress"] != int64(i+1) ||
					fields["message"] != message ||
					fields["total"] != nil {
					t.Fatalf("unexpected progress: %+v", notification)
				}
			}
		})
	}
}

func TestProgressDeliveryFailureDoesNotBlock(t *testing.T) {
	server := testServer(t)
	session := &progressSession{notifications: make(chan sdk.JSONRPCNotification)}
	request := networkRequest(map[string]any{"network": "1.0.0.0/8", "max_results": 1})
	request.Params.Meta = &sdk.Meta{ProgressToken: "scan"}
	result, err := server.handleLookupNetwork(server.mcp.WithContext(t.Context(), session), request)
	if err != nil || result.IsError {
		t.Fatalf("undeliverable progress failed the query: %+v, %v", result, err)
	}
}

func TestStdioUpdateProgress(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			release := make(chan struct{})
			endpoint := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-release:
					case <-r.Context().Done():
					}
					w.WriteHeader(http.StatusUnauthorized)
				}),
			)
			t.Cleanup(endpoint.Close)
			server := testServer(t)
			cfg := server.config
			cfg.Mode = config.ModeMaxMind
			cfg.MaxMind.AccountID, cfg.MaxMind.LicenseKey = 1, "test"
			cfg.MaxMind.Editions = []string{"GeoLite2-ASN"}
			cfg.MaxMind.DatabaseDir = t.TempDir()
			cfg.MaxMind.Endpoint = endpoint.URL
			updater, err := database.NewUpdater(cfg, server.dbManager)
			if err != nil {
				t.Fatal(err)
			}
			server = New(cfg, server.dbManager, updater, server.iterMgr, "test")
			send, receive := connectStdio(t, server)
			meta := map[string]any{"progressToken": "download"}
			if protocol == "2026-07-28" {
				meta["io.modelcontextprotocol/protocolVersion"] = protocol
				meta["io.modelcontextprotocol/clientCapabilities"] = map[string]any{}
			} else {
				send(
					map[string]any{
						"jsonrpc": "2.0",
						"id":      "init",
						"method":  "initialize",
						"params": map[string]any{
							"protocolVersion": protocol, "capabilities": map[string]any{},
							"clientInfo": map[string]any{"name": "test", "version": "1"},
						},
					},
				)
				if response := receive(); response["error"] != nil {
					t.Fatalf("initialize: %v", response)
				}
				send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
			}
			send(
				map[string]any{
					"jsonrpc": "2.0",
					"id":      "update",
					"method":  "tools/call",
					"params": map[string]any{
						"name": "update_databases", "arguments": map[string]any{}, "_meta": meta,
					},
				},
			)
			notification := receive()
			if notification["method"] != "notifications/progress" {
				t.Fatalf("expected progress while download is active: %v", notification)
			}
			params := notification["params"].(map[string]any)
			if params["progressToken"] != "download" || params["progress"] != float64(1) ||
				!strings.Contains(
					params["message"].(string),
					"GeoLite2-ASN",
				) || params["total"] != nil {
				t.Fatalf("unexpected progress: %v", notification)
			}
			close(release)
			response := receive()
			if response["id"] != "update" ||
				response["result"].(map[string]any)["isError"] != true {
				t.Fatalf("download failure was not returned: %v", response)
			}
		})
	}
}
