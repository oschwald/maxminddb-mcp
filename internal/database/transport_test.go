package database

import (
	"context"
	"crypto/md5" //nolint:gosec // The update API uses MD5 for file integrity.
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortDownloadTimeouts shortens timeouts for clients built afterward. The idle
// timeout applies to every later request, but downloadClient keeps the header
// timeout it was built with.
func shortDownloadTimeouts(t *testing.T) {
	t.Helper()
	header, idle := downloadHeaderTimeout, downloadIdleTimeout
	downloadHeaderTimeout, downloadIdleTimeout = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { downloadHeaderTimeout, downloadIdleTimeout = header, idle })
}

// stallHalfway serves the first half of the handler's response, then stops sending.
func stallHalfway(handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recorded := httptest.NewRecorder()
		handler.ServeHTTP(recorded, r)
		for key, values := range recorded.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(recorded.Code)
		_, _ = w.Write(recorded.Body.Bytes()[:recorded.Body.Len()/2])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
}

func TestUpdateFailsWhenServerStopsResponding(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(data) //nolint:gosec // Matches the update API checksum.
	var downloads atomic.Int32
	served := downloadHandler(t, data, hex.EncodeToString(sum[:]), &downloads)
	for _, test := range []struct {
		name    string
		handler http.Handler
		want    string
	}{
		{
			name: "no headers",
			handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			}),
			want: "timeout awaiting response headers",
		},
		{
			name:    "metadata body",
			handler: stallHalfway(served),
			want:    "download stalled",
		},
		{
			name: "database body",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/metadata") {
					served.ServeHTTP(w, r)
					return
				}
				stallHalfway(served).ServeHTTP(w, r)
			}),
			want: "download stalled",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			shortDownloadTimeouts(t)
			updater := testUpdater(t, test.handler)
			for range 2 {
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				results, err := updater.UpdateAll(ctx, nil)
				cancel()
				if err != nil || len(results) != 1 ||
					!strings.Contains(results[0].Error, test.want) {
					t.Fatalf("stalled update: %+v, %v", results, err)
				}
				if status := updater.Status(); status.State != "failed" {
					t.Fatalf("stalled update left status %q", status.State)
				}
			}
		})
	}
}

func TestInstallURLFailsWhenBodyStalls(t *testing.T) {
	shortDownloadTimeouts(t)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer endpoint.Close()
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	result := m.InstallURL(ctx, t.TempDir(), "import.mmdb", endpoint.URL, false, nil)
	if !strings.HasPrefix(result.Error, "download stalled") || result.Updated {
		t.Fatalf("stalled import: %+v", result)
	}
}

func TestIdleTimeoutCountsOnlyServerWaits(t *testing.T) {
	shortDownloadTimeouts(t)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for range 10 {
			_, _ = w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer endpoint.Close()
	client := &http.Client{Transport: newDownloadTransport(true)}
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		endpoint.URL,
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	first := make([]byte, 5)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	// A consumer that pauses longer than the idle timeout is not a stalled server.
	time.Sleep(3 * downloadIdleTimeout)
	rest, err := io.ReadAll(response.Body)
	if errors.Is(err, errDownloadStalled) {
		t.Fatal("consumer pause counted as a stall")
	}
	if err != nil || len(first)+len(rest) != 50 {
		t.Fatalf("trickled download: %d bytes, %v", len(first)+len(rest), err)
	}
}

func TestURLDownloadsHaveHeaderTimeout(t *testing.T) {
	// downloadClient is built at package load, so check its transport directly.
	transport, ok := downloadClient.Transport.(idleTimeoutTransport)
	if !ok {
		t.Fatalf("URL downloads use %T without an idle timeout", downloadClient.Transport)
	}
	base, ok := transport.base.(*http.Transport)
	if !ok || base.ResponseHeaderTimeout != 30*time.Second || !base.DisableCompression {
		t.Fatalf("URL download transport: %+v", transport.base)
	}
}

// TestIdleTimeoutCountsNetworkBytes trickles one large chunk. Go's chunked
// reader keeps reading until the caller's buffer fills or the chunk ends, so a
// single Read outlasts the idle timeout while bytes still arrive.
func TestIdleTimeoutCountsNetworkBytes(t *testing.T) {
	shortDownloadTimeouts(t)
	server := httptest.NewServer(trickleChunkHandler(t))
	t.Cleanup(server.Close)

	readTrickledChunk(t, &http.Client{Transport: newDownloadTransport(true)}, server.URL)
}

// TestIdleTimeoutCountsNetworkBytesOverTLS repeats the trickled chunk over
// HTTPS, where progress reaches the tracked connection only through the TLS
// connection it wraps.
func TestIdleTimeoutCountsNetworkBytesOverTLS(t *testing.T) {
	shortDownloadTimeouts(t)
	server := httptest.NewUnstartedServer(trickleChunkHandler(t))
	server.StartTLS()
	t.Cleanup(server.Close)

	readTrickledChunk(t, tlsDownloadClient(t, server), server.URL)
}

// TestIdleTimeoutStopsStalledHTTP2Body checks that an HTTP/2 body that stops
// mid-download still fails with the stall error.
func TestIdleTimeoutStopsStalledHTTP2Body(t *testing.T) {
	shortDownloadTimeouts(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("partial"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		},
	))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	response, err := tlsDownloadClient(t, server).Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.ProtoMajor != 2 {
		t.Fatalf("got HTTP/%d, want HTTP/2", response.ProtoMajor)
	}
	if _, err := io.ReadAll(response.Body); !errors.Is(err, errDownloadStalled) {
		t.Fatalf("got %v, want a stall error", err)
	}
}

const trickledChunkSize = 4096

// trickleChunkHandler sends one chunk in small pieces, each well inside the
// idle timeout, while the whole chunk takes longer than it.
func trickleChunkHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = fmt.Fprintf(
			buffered,
			"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n",
			trickledChunkSize,
		)
		_ = buffered.Flush()
		piece := strings.Repeat("x", 256)
		for range trickledChunkSize / len(piece) {
			time.Sleep(40 * time.Millisecond)
			if _, err := conn.Write([]byte(piece)); err != nil {
				return
			}
		}
		_, _ = conn.Write([]byte("\r\n0\r\n\r\n"))
	})
}

func readTrickledChunk(t *testing.T, client *http.Client, url string) {
	t.Helper()
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	buffer := make([]byte, trickledChunkSize)
	if _, err := io.ReadFull(response.Body, buffer); err != nil {
		t.Fatalf("download with steady progress failed: %v", err)
	}
}

// tlsDownloadClient returns a download client that trusts the test server.
func tlsDownloadClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	transport := newDownloadTransport(true)
	base := transport.(idleTimeoutTransport).base.(*http.Transport)
	base.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport}
}
