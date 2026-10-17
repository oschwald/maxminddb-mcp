package mcp

import (
	"net/http"
	"testing"

	"github.com/oschwald/maxminddb-mcp/internal/database"
)

func TestListDatabaseReadiness(t *testing.T) {
	server := testServer(t)
	call := modernTools(t, server)
	result := wireContent(t, call("list_databases", map[string]any{}))
	if result["status"] != "ready" || len(result["databases"].([]any)) != 1 ||
		result["update"] != nil {
		t.Fatalf("ready directory status: %v", result)
	}
	server.dbManager.RemoveDatabase("GeoLite2-ASN-Test.mmdb")
	result = wireContent(t, call("list_databases", map[string]any{}))
	if result["status"] != "empty" || result["message"] == "" {
		t.Fatalf("empty directory status: %v", result)
	}

	server = testUpdatingServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	call = modernTools(t, server)
	server.dbManager.RemoveDatabase("GeoLite2-ASN-Test.mmdb")
	var err error
	server.updater, err = database.NewUpdater(server.config, server.dbManager)
	if err != nil {
		t.Fatal(err)
	}
	result = wireContent(t, call("list_databases", map[string]any{}))
	if result["status"] != "downloading" ||
		result["update"].(map[string]any)["state"] != "pending" {
		t.Fatalf("initial download status: %v", result)
	}
	if _, err := server.updater.UpdateAll(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	result = wireContent(t, call("list_databases", map[string]any{}))
	if result["status"] != "error" || result["update"].(map[string]any)["state"] != "failed" {
		t.Fatalf("failed download status: %v", result)
	}
	if err := server.dbManager.LoadDatabase(
		"../../testdata/test-data/GeoLite2-ASN-Test.mmdb",
	); err != nil {
		t.Fatal(err)
	}
	result = wireContent(t, call("list_databases", map[string]any{}))
	if result["status"] != "ready" || result["update"].(map[string]any)["state"] != "failed" {
		t.Fatalf("loaded databases were hidden by an update failure: %v", result)
	}
}
