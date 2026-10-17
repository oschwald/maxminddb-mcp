package database

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5" //nolint:gosec // Matches the MaxMind update protocol.
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gofrs/flock"
)

func TestInstallEditionPersists(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data) //nolint:gosec // Matches the MaxMind update protocol.
	var downloads atomic.Int32
	u := testUpdater(t, downloadHandler(t, data, hex.EncodeToString(sum[:]), &downloads))
	u.config.MaxMind.Editions = []string{"GeoLite2-Country"}
	result, err := u.InstallEdition(t.Context(), "GeoLite2-ASN", nil)
	if err != nil || result.Error != "" || !result.Updated {
		t.Fatalf("install: %+v, %v", result, err)
	}
	if !slices.Equal(u.config.MaxMind.Editions, []string{"GeoLite2-Country"}) {
		t.Fatal("installation changed the caller's configuration")
	}
	restarted, err := NewUpdater(u.config, u.manager)
	if err != nil {
		t.Fatal(err)
	}
	editions, err := restarted.editions()
	if err != nil || !slices.Equal(editions, []string{"GeoLite2-Country", "GeoLite2-ASN"}) {
		t.Fatalf("editions after restart: %v, %v", editions, err)
	}
	results, err := restarted.UpdateAll(t.Context(), nil)
	if err != nil || len(results) != 2 || results[1].Database != "GeoLite2-ASN" ||
		results[1].Error != "" ||
		results[1].Updated {
		t.Fatalf("update remembered edition: %+v, %v", results, err)
	}
	for _, name := range []string{"../escape", `..\escape`, "x:y", "", ".", "a/b"} {
		result, err := u.InstallEdition(t.Context(), name, nil)
		if err != nil || result.Error == "" {
			t.Fatalf("invalid edition %q: %+v, %v", name, result, err)
		}
	}
	marker := filepath.Join(u.config.MaxMind.DatabaseDir, editionsDirectory, "GeoLite2-ASN")
	if content, err := os.ReadFile(marker); err != nil || len(content) != 0 {
		t.Fatalf("marker must contain no credentials: %v", err)
	}
}

func TestFailedEditionIsNotRemembered(t *testing.T) {
	u := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no subscription", http.StatusForbidden)
	}))
	result, err := u.InstallEdition(t.Context(), "GeoIP2-Country", nil)
	if err != nil || result.Error == "" {
		t.Fatalf("failed install: %+v, %v", result, err)
	}
	editions, err := u.editions()
	if err != nil || slices.Contains(editions, "GeoIP2-Country") {
		t.Fatalf("remembered failed edition: %v, %v", editions, err)
	}
}

func TestEditionRegistrationFailureReportsInstalledFile(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data) //nolint:gosec // Matches the MaxMind update protocol.
	var downloads atomic.Int32
	u := testUpdater(t, downloadHandler(t, data, hex.EncodeToString(sum[:]), &downloads))
	u.config.MaxMind.Editions = []string{"GeoLite2-Country"}
	if err := os.WriteFile(
		filepath.Join(u.config.MaxMind.DatabaseDir, editionsDirectory),
		nil,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	result, err := u.InstallEdition(t.Context(), "GeoLite2-ASN", nil)
	if err != nil || !result.Updated ||
		!strings.Contains(result.Error, "could not enable future updates") {
		t.Fatalf("registration failure hid installed file: %+v, %v", result, err)
	}
	if u.Status().State != "failed" {
		t.Fatal("registration failure was not reported in update status")
	}
	if _, ok := u.manager.GetDatabase("GeoLite2-ASN.mmdb"); !ok {
		t.Fatal("installed file was not loaded")
	}
}

func archiveData(t *testing.T, format string, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	switch format {
	case "gzip":
		gz := gzip.NewWriter(&buf)
		for _, data := range files {
			if _, err := gz.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	case "tar", "tar.gz":
		writer := io.Writer(&buf)
		var gz *gzip.Writer
		if format == "tar.gz" {
			gz = gzip.NewWriter(writer)
			writer = gz
		}
		tarWriter := tar.NewWriter(writer)
		for name, data := range files {
			if err := tarWriter.WriteHeader(
				&tar.Header{
					Name:   name,
					Mode:   0o600,
					Size:   int64(len(data)),
					Format: tar.FormatUSTAR,
				},
			); err != nil {
				t.Fatal(err)
			}
			if _, err := tarWriter.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if gz != nil {
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
		}
	case "zip":
		zipWriter := zip.NewWriter(&buf)
		for name, data := range files {
			writer, err := zipWriter.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown archive format: %s", format)
	}
	return buf.Bytes()
}

func TestInstallURLFormats(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"raw", "gzip", "tar", "tar.gz", "zip"} {
		t.Run(format, func(t *testing.T) {
			body := data
			if format != "raw" {
				// Archive paths are never used as filesystem paths.
				body = archiveData(t, format, map[string][]byte{"../outside.mmdb": data})
			}
			endpoint := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
						t.Error("download forwarded credentials or a signed referrer")
					}
					if r.URL.Path == "/redirect" {
						http.Redirect(w, r, "/download", http.StatusFound)
						return
					}
					_, _ = w.Write(body)
				}),
			)
			defer endpoint.Close()
			m, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			dir := t.TempDir()
			result := m.InstallURL(
				t.Context(),
				dir,
				"import.mmdb",
				endpoint.URL+"/redirect?signature=secret",
				false,
				nil,
			)
			if result.Error != "" || !result.Updated || result.Database != "import.mmdb" {
				t.Fatalf("install: %+v", result)
			}
			installed, err := os.ReadFile(filepath.Join(dir, "import.mmdb"))
			if err != nil || !bytes.Equal(installed, data) {
				t.Fatalf("installed bytes differ: %v", err)
			}
			if _, ok := m.GetDatabase("import.mmdb"); !ok {
				t.Fatal("installed database is not loaded")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatalf("temporary files remain: %v, %v", entries, err)
			}
			result = m.InstallURL(t.Context(), dir, "import.mmdb", endpoint.URL, false, nil)
			if result.Error == "" || result.Updated {
				t.Fatal("silently replaced an existing database")
			}
		})
	}
}

func TestInstallURLPreservesExistingOnFailure(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	badGzip := archiveData(t, "gzip", map[string][]byte{"test.mmdb": data})
	badGzip[len(badGzip)-8] ^= 0xff
	for _, format := range []string{"invalid", "gzip", "tar", "tar.gz", "zip", "empty", "large"} {
		t.Run(format, func(t *testing.T) {
			body := []byte("not an MMDB")
			switch format {
			case "gzip":
				body = badGzip
			case "tar", "tar.gz", "zip":
				body = archiveData(t, format, map[string][]byte{"one.mmdb": data, "two.mmdb": data})
			case "empty":
				body = archiveData(t, "zip", map[string][]byte{"README": []byte("no database")})
			}
			endpoint := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if format == "large" {
						w.Header().Set("Content-Length", "1073741825")
					}
					_, _ = w.Write(body)
				}),
			)
			defer endpoint.Close()
			m, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			dir := t.TempDir()
			path := filepath.Join(dir, "import.mmdb")
			//nolint:gosec // The destination is a fixed filename inside t.TempDir.
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			result := m.InstallURL(t.Context(), dir, "import.mmdb", endpoint.URL, true, nil)
			if result.Error == "" || result.Updated {
				t.Fatalf("invalid download installed: %+v", result)
			}
			current, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(current, data) {
				t.Fatalf("existing database was changed: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatalf("temporary files remain after failure: %v, %v", entries, err)
			}
		})
	}
}

func TestInstallURLValidationAndCancellation(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	dir := t.TempDir()
	for _, name := range []string{"../escape.mmdb", `..\escape.mmdb`, "/tmp/escape.mmdb", "file.mmdb:stream", "file", ".mmdb"} {
		result := m.InstallURL(t.Context(), dir, name, "https://example.com/db", false, nil)
		if result.Error == "" {
			t.Fatalf("accepted filename: %q", name)
		}
	}
	for _, source := range []string{"file:///etc/passwd", "ftp://example.com/db", "http://user:secret@example.com/db", "https:///missing", "https://example.com/%zz?secret=value"} {
		result := m.InstallURL(t.Context(), dir, "import.mmdb", source, false, nil)
		if result.Error == "" || strings.Contains(result.Error, "secret") {
			t.Fatalf("URL validation: %+v", result)
		}
	}
	started := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan UpdateResult, 1)
	go func() { done <- m.InstallURL(ctx, dir, "import.mmdb", endpoint.URL+"/?secret=hidden", false, nil) }()
	<-started
	cancel()
	result := <-done
	if result.Error != context.Canceled.Error() || result.Updated {
		t.Fatalf("cancellation: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "import.mmdb")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled download installed a database")
	}
	lock := flock.New(filepath.Join(dir, "locked.mmdb.lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	result = m.InstallURL(ctx, dir, "locked.mmdb", endpoint.URL, false, nil)
	if result.Error != context.Canceled.Error() {
		t.Fatalf("canceled lock wait: %+v", result)
	}
}

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) { return len(p), nil }

func TestDownloadSizeLimit(t *testing.T) {
	n, err := copyDatabase(t.Context(), io.Discard, endlessReader{})
	if n != maxDatabaseBytes+1 || err == nil {
		t.Fatalf("unbounded stream did not stop at limit: %d, %v", n, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := copyDatabase(ctx, io.Discard, endlessReader{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy ignored cancellation: %v", err)
	}
}

func TestDownloadRedirectPolicyAndErrors(t *testing.T) {
	original, err := http.NewRequest(
		http.MethodGet,
		"https://example.com/?signature=hidden",
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"http://example.com/db", "file:///tmp/db", "https://user:password@example.com/db"} {
		parsed, err := url.Parse(target)
		if err != nil {
			t.Fatal(err)
		}
		request := &http.Request{URL: parsed, Header: make(http.Header)}
		if err := downloadClient.CheckRedirect(request, []*http.Request{original}); err == nil {
			t.Fatalf("accepted redirect to %s", target)
		}
	}
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "reflected-secret", http.StatusForbidden)
	}))
	defer endpoint.Close()
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	result := m.InstallURL(
		t.Context(),
		t.TempDir(),
		"db.mmdb",
		endpoint.URL+"/?signature=secret",
		false,
		nil,
	)
	if result.Error != "download returned HTTP 403" {
		t.Fatalf("HTTP error included a URL or response body: %+v", result)
	}
}
