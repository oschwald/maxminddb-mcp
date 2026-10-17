package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

func TestStdioProtocol(t *testing.T) {
	for _, protocol := range []string{"2024-11-05", "2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			server := testServer(t)
			input, writeInput := io.Pipe()
			readOutput, output := io.Pipe()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx, input, output) }()
			t.Cleanup(func() {
				cancel()
				_ = writeInput.Close()
				_ = input.Close()
				_ = readOutput.Close()
				_ = output.Close()
				select {
				case err := <-done:
					if err != nil && !errors.Is(err, context.Canceled) &&
						!errors.Is(err, io.ErrClosedPipe) {
						t.Error(err)
					}
				case <-time.After(2 * time.Second):
					t.Error("stdio shutdown timed out")
				}
			})
			responses := make(chan map[string]any, 1)
			go func() {
				decoder := json.NewDecoder(readOutput)
				for {
					var message map[string]any
					if err := decoder.Decode(&message); err != nil {
						return
					}
					select {
					case responses <- message:
					case <-ctx.Done():
						return
					}
				}
			}()
			call := func(method string, params map[string]any) map[string]any {
				t.Helper()
				if protocol == "2026-07-28" {
					params["_meta"] = map[string]any{
						"io.modelcontextprotocol/protocolVersion": protocol,
						"io.modelcontextprotocol/clientInfo": map[string]any{
							"name":    "integration-test",
							"version": "1",
						},
						"io.modelcontextprotocol/clientCapabilities": map[string]any{},
					}
				}
				request := map[string]any{
					"jsonrpc": "2.0",
					"id":      method,
					"method":  method,
					"params":  params,
				}
				go func() { _ = json.NewEncoder(writeInput).Encode(request) }()
				select {
				case response := <-responses:
					if response["id"] != method || response["error"] != nil {
						t.Fatalf("%s: %v", method, response)
					}
					result, ok := response["result"].(map[string]any)
					if !ok {
						t.Fatalf("%s returned no result: %v", method, response)
					}
					return result
				case <-time.After(2 * time.Second):
					t.Fatalf("%s timed out", method)
					return nil
				}
			}
			if protocol == "2026-07-28" {
				discovery := call("server/discover", map[string]any{})
				if discovery["supportedVersions"].([]any)[0] != protocol {
					t.Fatal("current protocol not advertised")
				}
			} else {
				initialized := call(
					"initialize",
					map[string]any{
						"protocolVersion": protocol,
						"capabilities":    map[string]any{},
						"clientInfo":      map[string]any{"name": "test", "version": "1"},
					},
				)
				if initialized["serverInfo"].(map[string]any)["version"] != "test" {
					t.Fatal("incorrect server version")
				}
				if initialized["protocolVersion"] != protocol {
					t.Fatalf("wrong negotiation: %v", initialized)
				}
				if err := json.NewEncoder(writeInput).
					Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
					t.Fatal(err)
				}
			}
			listed := call("tools/list", map[string]any{})
			tools := listed["tools"].([]any)
			if len(tools) != 3 {
				t.Fatalf("got %d tools, want 3", len(tools))
			}
			for _, item := range tools {
				tool := item.(map[string]any)
				if protocol != "2024-11-05" && tool["outputSchema"] == nil {
					t.Errorf("%s has no output schema", tool["name"])
				}
				annotations := tool["annotations"].(map[string]any)
				if annotations["readOnlyHint"] != true || annotations["openWorldHint"] != false {
					t.Fatalf("incorrect annotations: %v", annotations)
				}
			}
			toolCall := func(name string, arguments map[string]any) map[string]any {
				t.Helper()
				return call("tools/call", map[string]any{"name": name, "arguments": arguments})
			}
			invalid := toolCall(
				"lookup_network",
				map[string]any{"network": "1.0.0.0/8", "max_results": -1},
			)
			if invalid["isError"] != true {
				t.Fatalf("invalid size accepted: %v", invalid)
			}
			invalid = toolCall(
				"lookup_network",
				map[string]any{
					"network": "1.0.0.0/8",
					"filters": []any{map[string]any{"field": "asn"}},
				},
			)
			if invalid["isError"] != true {
				t.Fatalf("invalid filter accepted: %v", invalid)
			}
			invalid = toolCall("lookup_ip", map[string]any{"ip": "1.0.0.1", "databaze": "typo"})
			if invalid["isError"] != true {
				t.Fatalf("unknown parameter accepted: %v", invalid)
			}
			all := toolCall("list_databases", map[string]any{})
			payload := wireContent(t, all)
			if len(payload["databases"].([]any)) != 1 {
				t.Fatal("expected one database")
			}
			first := wireContent(
				t,
				toolCall(
					"lookup_network",
					map[string]any{"network": "1.0.0.0/8", "max_results": 1},
				),
			)
			if first["has_more"] != true || len(first["results"].([]any)) != 1 {
				t.Fatalf("invalid first page: %v", first)
			}
			second := wireContent(
				t,
				toolCall(
					"lookup_network",
					map[string]any{
						"network":     "1.0.0.0/8",
						"iterator_id": first["iterator_id"],
						"max_results": 1,
					},
				),
			)
			if second["total_matched"] != float64(2) ||
				second["results"].([]any)[0].(map[string]any)["network"] == first["results"].([]any)[0].(map[string]any)["network"] {
				t.Fatal("wire pagination repeated a record")
			}
			for _, ip := range []string{"1.0.0.1", "127.0.0.1"} {
				single := wireContent(
					t,
					toolCall(
						"lookup_ip",
						map[string]any{"ip": ip, "database": "GeoLite2-ASN-Test.mmdb"},
					),
				)
				if single["found"] != (ip == "1.0.0.1") {
					t.Fatalf("incorrect found flag: %v", single)
				}
				if ip == "1.0.0.1" && single["network"] == nil {
					t.Fatal("matched prefix missing")
				}
				wireContent(t, toolCall("lookup_ip", map[string]any{"ip": ip}))
			}
		})
	}
}

func wireContent(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if result["isError"] == true {
		t.Fatalf("tool failed: %v", result)
	}
	content := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("missing text fallback")
	}
	var text map[string]any
	if err := json.Unmarshal(
		[]byte(content[0].(map[string]any)["text"].(string)),
		&text,
	); err != nil {
		t.Fatal(err)
	}
	if structured, ok := result["structuredContent"].(map[string]any); ok {
		return structured
	}
	return text
}
