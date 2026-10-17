package mcp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5" //nolint:gosec // The MaxMind update API uses MD5.
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallTools(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(
		&tar.Header{Name: "GeoLite2-ASN.mmdb", Size: int64(len(data)), Mode: 0o600},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data) //nolint:gosec // The MaxMind update API uses MD5.
	server := testUpdatingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, key, ok := r.BasicAuth()
		if !ok || account != "1" || key != "test" {
			t.Error("edition request did not use configured credentials")
		}
		if strings.HasSuffix(r.URL.Path, "/metadata") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"databases": []map[string]string{
					{
						"edition_id": "GeoLite2-ASN",
						"date":       "2026-10-01",
						"md5":        hex.EncodeToString(sum[:]),
					},
				},
			})
			return
		}
		w.Header().Set("Last-Modified", "Thu, 01 Oct 2026 00:00:00 GMT")
		_, _ = w.Write(archive.Bytes())
	}))
	server.config.MaxMind.Editions = []string{"GeoLite2-Country"}
	call := modernTools(t, server)
	installed := wireContent(t, call("install_edition", map[string]any{"edition": "GeoLite2-ASN"}))
	if installed["database"] != "GeoLite2-ASN" || installed["updated"] != true ||
		installed["error"] != nil {
		t.Fatalf("edition installation: %v", installed)
	}
	lookup := wireContent(
		t,
		call("lookup_ip", map[string]any{"ip": "1.0.0.1", "database": "GeoLite2-ASN.mmdb"}),
	)
	if lookup["found"] != true {
		t.Fatalf("installed edition is not queryable: %v", lookup)
	}
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("MaxMind credentials leaked to URL download")
		}
		_, _ = w.Write(data)
	}))
	defer endpoint.Close()
	installed = wireContent(
		t,
		call("install_database", map[string]any{"url": endpoint.URL, "database": "custom.mmdb"}),
	)
	if installed["database"] != "custom.mmdb" || installed["updated"] != true ||
		installed["error"] != nil {
		t.Fatalf("URL installation: %v", installed)
	}
	lookup = wireContent(
		t,
		call("lookup_ip", map[string]any{"ip": "1.0.0.1", "database": "custom.mmdb"}),
	)
	if lookup["found"] != true {
		t.Fatalf("URL database is not queryable: %v", lookup)
	}
	failed := call(
		"install_database",
		map[string]any{"url": endpoint.URL, "database": "custom.mmdb"},
	)
	if failed["isError"] != true {
		t.Fatalf("overwrite refusal is not an MCP error: %v", failed)
	}
	installed = wireContent(
		t,
		call(
			"install_database",
			map[string]any{"url": endpoint.URL, "database": "custom.mmdb", "overwrite": true},
		),
	)
	if installed["updated"] != true || installed["error"] != nil {
		t.Fatalf("explicit overwrite: %v", installed)
	}
	server = testServer(t)
	server.config.Directory.Paths = []string{t.TempDir(), t.TempDir()}
	call = modernTools(t, server)
	installed = wireContent(
		t,
		call("install_database", map[string]any{"url": endpoint.URL, "database": "custom.mmdb"}),
	)
	if installed["error"] != nil {
		t.Fatalf("directory installation: %v", installed)
	}
	if _, err := os.Stat(
		filepath.Join(server.config.Directory.Paths[0], "custom.mmdb"),
	); err != nil {
		t.Fatalf("database was not installed into the first configured folder: %v", err)
	}
}
