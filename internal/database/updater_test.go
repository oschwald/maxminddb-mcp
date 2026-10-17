package database

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5" //nolint:gosec // The update API uses MD5 for file integrity.
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-mcp/internal/config"
)

func testUpdater(t *testing.T, handler http.Handler) *Updater {
	t.Helper()
	endpoint := httptest.NewServer(handler)
	t.Cleanup(endpoint.Close)
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	cfg := config.DefaultConfig()
	cfg.MaxMind.AccountID = 1
	cfg.MaxMind.LicenseKey = "test"
	cfg.MaxMind.Editions = []string{"GeoLite2-ASN"}
	cfg.MaxMind.DatabaseDir = t.TempDir()
	cfg.MaxMind.Endpoint = endpoint.URL
	updater, err := NewUpdater(cfg, manager)
	if err != nil {
		t.Fatal(err)
	}
	return updater
}

func downloadHandler(
	t *testing.T,
	data []byte,
	checksum string,
	downloads *atomic.Int32,
) http.Handler {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(
		&tar.Header{Name: "test/GeoLite2-ASN.mmdb", Mode: 0o600, Size: int64(len(data))},
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/metadata") {
			if err := json.NewEncoder(w).
				Encode(map[string]any{"databases": []map[string]string{{"edition_id": "GeoLite2-ASN", "md5": checksum, "date": "2026-10-01"}}}); err != nil {
				t.Error(err)
			}
			return
		}
		downloads.Add(1)
		w.Header().Set("Last-Modified", "Thu, 01 Oct 2026 00:00:00 GMT")
		if _, err := w.Write(archive.Bytes()); err != nil {
			t.Error(err)
		}
	})
}

func TestUpdateRepairsMissingAndChangedFiles(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data) //nolint:gosec // Matches the update API checksum.
	checksum := hex.EncodeToString(sum[:])
	var downloads atomic.Int32
	updater := testUpdater(t, downloadHandler(t, data, checksum, &downloads))
	dir := updater.config.MaxMind.DatabaseDir
	// A leftover checksum must not suppress a missing database download.
	if err := os.WriteFile( //nolint:gosec // The directory comes from t.TempDir.
		filepath.Join(dir, ".checksums"),
		[]byte("GeoLite2-ASN:"+checksum+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"missing", "unchanged", "deleted", "corrupted"} {
		path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
		switch action {
		case "deleted":
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		case "corrupted":
			if err := os.WriteFile(path, []byte("bad database"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		results, err := updater.UpdateAll(t.Context())
		if err != nil || len(results) != 1 || results[0].Error != "" {
			t.Fatalf("%s: %+v, %v", action, results, err)
		}
		if results[0].Updated != (action != "unchanged") {
			t.Fatalf("%s: wrong updated flag", action)
		}
		installed, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(installed, data) {
			t.Fatalf("%s: installed file differs: %v", action, err)
		}
		if _, ok := updater.manager.GetReader("GeoLite2-ASN.mmdb"); !ok {
			t.Fatal("download was not loaded")
		}
	}
	if downloads.Load() != 3 {
		t.Fatalf("got %d downloads, want 3", downloads.Load())
	}
}

func TestBadDownloadPreservesInstalledFile(t *testing.T) {
	for _, validChecksum := range []bool{false, true} {
		checksum := "wrong checksum"
		data := []byte("not an mmdb")
		if validChecksum {
			sum := md5.Sum(data) //nolint:gosec // Matches the update API checksum.
			checksum = hex.EncodeToString(sum[:])
		}
		var downloads atomic.Int32
		updater := testUpdater(t, downloadHandler(t, data, checksum, &downloads))
		path := filepath.Join(updater.config.MaxMind.DatabaseDir, "GeoLite2-ASN.mmdb")
		original, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
		if err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // The directory comes from t.TempDir.
		if err := os.WriteFile(
			path,
			original,
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		result, err := updater.UpdateDatabase(t.Context(), "GeoLite2-ASN")
		if err != nil || result.Error == "" || result.Updated {
			t.Fatalf("bad download accepted: %+v, %v", result, err)
		}
		installed, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(installed, original) {
			t.Fatal("bad download replaced installed file")
		}
		temporary, err := filepath.Glob(filepath.Join(updater.config.MaxMind.DatabaseDir, "*.tmp"))
		if err != nil || len(temporary) != 0 {
			t.Fatalf("temporary files left: %v %v", temporary, err)
		}
	}
}

func TestDownloadCancellation(t *testing.T) {
	started := make(chan struct{})
	updater := testUpdater(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan UpdateResult, 1)
	go func() { result, _ := updater.UpdateDatabase(ctx, "GeoLite2-ASN"); done <- result }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("download did not start")
	}
	cancel()
	select {
	case result := <-done:
		if result.Error == "" {
			t.Fatal("canceled download reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("download ignored cancellation")
	}
}

func TestQueuedUpdateCancellation(t *testing.T) {
	started := make(chan struct{})
	updater := testUpdater(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	active := make(chan struct{})
	go func() { _, _ = updater.UpdateAll(ctx); close(active) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first update did not start")
	}
	for _, all := range []bool{false, true} {
		queued, stop := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			if all {
				_, err := updater.UpdateAll(queued)
				done <- err
			} else {
				_, err := updater.UpdateDatabase(queued, "GeoLite2-ASN")
				done <- err
			}
		}()
		stop()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want cancellation", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued update ignored cancellation")
		}
	}
	cancel()
	<-active
}

func TestStartupUpdateCheck(t *testing.T) {
	for _, installed := range []bool{false, true} {
		for _, autoUpdate := range []bool{false, true} {
			t.Run(
				fmt.Sprintf("installed=%t/auto_update=%t", installed, autoUpdate),
				func(t *testing.T) {
					requested := make(chan struct{}, 1)
					updater := testUpdater(
						t,
						http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
							requested <- struct{}{}
							http.Error(w, "test endpoint", http.StatusUnauthorized)
						}),
					)
					if installed {
						// An installed database must not suppress checks for a missing configured edition.
						if err := updater.manager.LoadDatabase(
							"../../testdata/test-data/GeoIP2-City-Test.mmdb",
						); err != nil {
							t.Fatal(err)
						}
					}
					updater.config.AutoUpdate = autoUpdate
					updater.config.UpdateIntervalDuration = time.Hour
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					done := make(chan struct{})
					go func() { updater.RunUpdates(ctx); close(done) }()
					if autoUpdate {
						select {
						case <-requested:
						case <-time.After(5 * time.Second):
							t.Fatal("startup update check did not run")
						}
						cancel()
					}
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("update loop did not stop")
					}
					if !autoUpdate && (len(requested) > 0) != !installed {
						t.Fatal(
							"automatic updates disabled: only an empty installation should download",
						)
					}
				},
			)
		}
	}
}
