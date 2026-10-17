package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestStdioEOF(t *testing.T) {
	server := testServer(t)
	input, writer := io.Pipe()
	defer func() { _ = input.Close() }()
	done := make(chan error, 1)
	go func() { done <- server.Serve(t.Context(), input, io.Discard) }()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stdio server did not stop at EOF")
	}
}

func TestStdioProtocol(t *testing.T) {
	for _, protocol := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			server := testServer(t)
			send, receive := connectStdio(t, server)
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
				send(request)
				response := receive()
				if response["id"] != method || response["error"] != nil {
					t.Fatalf("%s: %v", method, response)
				}
				result, ok := response["result"].(map[string]any)
				if !ok {
					t.Fatalf("%s returned no result: %v", method, response)
				}
				if protocol == "2026-07-28" {
					assertModernResult(t, result)
					if method == "server/discover" || method == "tools/list" {
						if result["ttlMs"] != float64(300000) || result["cacheScope"] != "private" {
							t.Fatalf("incorrect metadata cache hints: %v", result)
						}
					} else if result["ttlMs"] != nil || result["cacheScope"] != nil {
						t.Fatalf("query result advertised metadata caching: %v", result)
					}
				} else if result["resultType"] != nil || result["ttlMs"] != nil || result["cacheScope"] != nil {
					t.Fatalf("modern response fields leaked into %s: %v", protocol, result)
				}
				return result
			}
			if protocol == "2026-07-28" {
				// Discovery is optional. A tool call must work as the first request.
				direct := call(
					"tools/call",
					map[string]any{
						"name":      "lookup_ip",
						"arguments": map[string]any{"ip": "1.0.0.1"},
					},
				)
				wireContent(t, direct)
				discovery := call("server/discover", map[string]any{})
				if discovery["supportedVersions"].([]any)[0] != protocol {
					t.Fatal("current protocol not advertised")
				}
				if discovery["instructions"] != serverInstructions {
					t.Fatal("discovery omitted the server instructions")
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
				if initialized["instructions"] != serverInstructions {
					t.Fatal("initialization omitted the server instructions")
				}
				send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
			}
			listed := call("tools/list", map[string]any{})
			tools := listed["tools"].([]any)
			if len(tools) != 4 {
				t.Fatalf("got %d tools, want 4", len(tools))
			}
			var names []string
			for _, item := range tools {
				tool := item.(map[string]any)
				names = append(names, tool["name"].(string))
				titles := map[string]string{
					"lookup_ip":      "Look up an IP address",
					"lookup_ips":     "Look up IP addresses",
					"lookup_network": "Query a network",
					"list_databases": "List databases",
				}
				if tool["title"] != titles[tool["name"].(string)] {
					t.Fatalf("incorrect display title: %v", tool)
				}
				if protocol >= "2025-06-18" && tool["outputSchema"] == nil {
					t.Errorf("%s has no output schema", tool["name"])
				}
				annotations := tool["annotations"].(map[string]any)
				if annotations["readOnlyHint"] != true || annotations["openWorldHint"] != false {
					t.Fatalf("incorrect annotations: %v", annotations)
				}
			}
			if !slices.IsSorted(names) {
				t.Fatalf("tool definitions are not in a stable order: %v", names)
			}
			toolCall := func(name string, arguments map[string]any) map[string]any {
				t.Helper()
				return call("tools/call", map[string]any{"name": name, "arguments": arguments})
			}
			batch := wireContent(
				t,
				toolCall("lookup_ips", map[string]any{"ips": []string{"1.0.0.1", "127.0.0.1"}}),
			)
			if len(batch["results"].([]any)) != 2 {
				t.Fatalf("invalid batch response: %v", batch)
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

func assertModernResult(t *testing.T, result map[string]any) {
	t.Helper()
	if result["resultType"] != "complete" {
		t.Fatalf("missing resultType: %v", result)
	}
	meta, _ := result["_meta"].(map[string]any)
	info, _ := meta["io.modelcontextprotocol/serverInfo"].(map[string]any)
	if info["name"] != "MaxMindDB Server" || info["version"] != "test" {
		t.Fatalf("incorrect server identity: %v", result)
	}
}

func TestStdioInvalidProtocolMetadata(t *testing.T) {
	server := testServer(t)
	send, receive := connectStdio(t, server)
	for _, test := range []struct {
		name string
		meta map[string]any
		code int
	}{
		{"missing capabilities", map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28"}, -32602},
		{"invalid capabilities", map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": "invalid"}, -32602},
		{"null capabilities", map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": nil}, -32602},
		{"unsupported version", map[string]any{"io.modelcontextprotocol/protocolVersion": "2099-01-01", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}, -32022},
	} {
		send(
			map[string]any{
				"jsonrpc": "2.0",
				"id":      test.name,
				"method":  "tools/list",
				"params":  map[string]any{"_meta": test.meta},
			},
		)
		response := receive()
		failure, ok := response["error"].(map[string]any)
		if response["id"] != test.name || !ok || failure["code"] != float64(test.code) {
			t.Fatalf("%s: %v", test.name, response)
		}
		if test.code == -32022 {
			data, _ := failure["data"].(map[string]any)
			versions, _ := data["supported"].([]any)
			if data["requested"] != "2099-01-01" || !slices.Contains(versions, any("2026-07-28")) {
				t.Fatalf("unsupported version error omitted supported versions: %v", failure)
			}
		}
	}
	// Invalid requests must not poison later requests on the same transport.
	send(
		map[string]any{
			"jsonrpc": "2.0",
			"id":      "valid",
			"method":  "tools/list",
			"params": map[string]any{
				"_meta": map[string]any{
					"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
					"io.modelcontextprotocol/clientCapabilities": map[string]any{},
				},
			},
		},
	)
	response := receive()
	result, ok := response["result"].(map[string]any)
	if response["id"] != "valid" || !ok {
		t.Fatalf("valid request after protocol errors: %v", response)
	}
	assertModernResult(t, result)
}

func TestStdioCancellation(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := testUpdatingServer(
				t,
				http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					close(started)
					<-r.Context().Done()
					close(canceled)
				}),
			)
			send, receive := connectStdio(t, server)
			meta := map[string]any{}
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
							"protocolVersion": protocol,
							"capabilities":    map[string]any{},
							"clientInfo":      map[string]any{"name": "test", "version": "1"},
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
					"id":      7,
					"method":  "tools/call",
					"params": map[string]any{
						"name": "update_databases", "arguments": map[string]any{}, "_meta": meta,
					},
				},
			)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("update did not start")
			}
			send(
				map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/cancelled", //nolint:misspell // MCP defines this method name.
					"params":  map[string]any{"requestId": 7},
				},
			)
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("stdio cancellation did not reach the download")
			}
			send(
				map[string]any{
					"jsonrpc": "2.0",
					"id":      "after",
					"method":  "tools/call",
					"params": map[string]any{
						"name": "list_databases", "arguments": map[string]any{}, "_meta": meta,
					},
				},
			)
			for {
				response := receive()
				if response["id"] == float64(7) {
					if response["result"].(map[string]any)["isError"] != true {
						t.Fatalf("canceled update reported success: %v", response)
					}
					continue
				}
				result, ok := response["result"].(map[string]any)
				if response["id"] != "after" || !ok || result["isError"] == true {
					t.Fatalf("request after cancellation: %v", response)
				}
				if len(wireContent(t, result)["databases"].([]any)) != 1 {
					t.Fatal("cancellation changed the loaded databases")
				}
				break
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

func connectStdio(t *testing.T, server *Server) (func(map[string]any), func() map[string]any) {
	t.Helper()
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
	send := func(message map[string]any) {
		t.Helper()
		written := make(chan error, 1)
		go func() { written <- json.NewEncoder(writeInput).Encode(message) }()
		select {
		case err := <-written:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("stdio write timed out")
		}
	}
	receive := func() map[string]any {
		t.Helper()
		select {
		case message := <-responses:
			return message
		case <-time.After(5 * time.Second):
			t.Fatal("stdio read timed out")
			return nil
		}
	}
	return send, receive
}
