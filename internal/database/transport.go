package database

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Downloads have no total deadline, but a server that stops responding must not
// hold the update or installation gate until the process exits.
var (
	downloadHeaderTimeout = 30 * time.Second
	downloadIdleTimeout   = time.Minute
)

var errDownloadStalled = errors.New("download stalled")

// newDownloadTransport returns a transport that fails requests whose response
// headers or body bytes stop arriving.
func newDownloadTransport(disableCompression bool) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = downloadHeaderTimeout
	transport.DisableCompression = disableCompression
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &activityConn{Conn: conn}, nil
	}
	return idleTimeoutTransport{base: transport}
}

// activityConn reports bytes read from the network to the response body that
// is reading through it.
type activityConn struct {
	net.Conn
	body atomic.Pointer[idleTimeoutBody]
}

func (c *activityConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		if body := c.body.Load(); body != nil {
			body.progressed()
		}
	}
	return n, err
}

// idleTimeoutTransport cancels a request when a read from its response body
// waits longer than downloadIdleTimeout without bytes arriving.
type idleTimeoutTransport struct {
	base http.RoundTripper
}

func (t idleTimeoutTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(request.Context())
	var conn *activityConn
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			netConn := info.Conn
			if tlsConn, ok := netConn.(*tls.Conn); ok {
				netConn = tlsConn.NetConn()
			}
			conn, _ = netConn.(*activityConn)
		},
	})
	response, err := t.base.RoundTrip(request.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	body := &idleTimeoutBody{
		body:    response.Body,
		cancel:  cancel,
		timeout: downloadIdleTimeout,
	}
	body.timer = time.AfterFunc(body.timeout, func() {
		body.stalled.Store(true)
		cancel()
	})
	body.timer.Stop()
	// HTTP/1 body reads can loop over several network reads, such as within one
	// chunk or while decompressing. HTTP/2 connections carry other streams and
	// their body reads return as soon as any data arrives.
	if conn != nil && response.ProtoMajor == 1 {
		body.conn = conn
		conn.body.Store(body)
	}
	response.Body = body
	return response, nil
}

type idleTimeoutBody struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	conn    *activityConn
	timer   *time.Timer // guarded by mu
	timeout time.Duration
	mu      sync.Mutex
	stalled atomic.Bool
	waiting bool // guarded by mu
}

// Read times only the wait for the server, so a slow consumer is not mistaken
// for a stall. Bytes arriving on the connection restart the timer.
func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	if b.stalled.Load() {
		return 0, b.stallError()
	}
	b.mu.Lock()
	b.waiting = true
	b.timer.Reset(b.timeout)
	b.mu.Unlock()

	n, err := b.body.Read(p)

	b.mu.Lock()
	b.waiting = false
	if !b.timer.Stop() {
		// The timer fired and is canceling the request. Fail this read so a late
		// cancellation cannot abort a later one.
		b.stalled.Store(true)
	}
	b.mu.Unlock()
	if b.stalled.Load() {
		return n, b.stallError()
	}
	if err != nil {
		b.detach()
	}
	return n, err
}

// progressed restarts the idle timer while a Read waits for the server.
func (b *idleTimeoutBody) progressed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waiting && b.timer.Stop() {
		b.timer.Reset(b.timeout)
	}
}

// detach stops connection reads for a later response from counting toward this body.
func (b *idleTimeoutBody) detach() {
	if b.conn != nil {
		b.conn.body.CompareAndSwap(b, nil)
	}
}

func (b *idleTimeoutBody) stallError() error {
	seconds := strconv.FormatFloat(b.timeout.Seconds(), 'f', -1, 64)
	return fmt.Errorf("%w: no data received for %s seconds", errDownloadStalled, seconds)
}

func (b *idleTimeoutBody) Close() error {
	b.mu.Lock()
	b.timer.Stop()
	b.mu.Unlock()
	b.detach()
	err := b.body.Close()
	b.cancel()
	return err
}
