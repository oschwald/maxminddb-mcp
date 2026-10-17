package database

import (
	"crypto/md5" //nolint:gosec // The update API uses MD5 for file integrity.
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadStatusAndRecovery(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	var downloads atomic.Int32
	//nolint:gosec // The update API uses MD5 for file integrity.
	handler := downloadHandler(t, data, fmt.Sprintf("%x", md5.Sum(data)), &downloads)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var fail atomic.Bool
	fail.Store(true)
	updater := testUpdater(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			once.Do(func() { close(started) })
			select {
			case <-release:
			case <-r.Context().Done():
			}
			http.Error(w, "invalid credential: test-secret", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	updater.config.MaxMind.LicenseKey = "test-secret"
	if updater.Status().State != "pending" {
		t.Fatal("initial download was not pending")
	}
	ctx := t.Context()
	done := make(chan error, 1)
	go func() { _, updateErr := updater.UpdateAll(ctx, nil); done <- updateErr }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	if updater.Status().State != "updating" {
		t.Error("blocked download was not reported")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed download did not finish")
	}
	status := updater.Status()
	if status.State != "failed" || len(status.Results) != 1 || status.Results[0].Error == "" {
		t.Fatalf("failed download status: %+v", status)
	}
	if strings.Contains(status.Results[0].Error, "test-secret") {
		t.Fatal("status exposed the license key")
	}
	status.Results[0].Error = "modified"
	if updater.Status().Results[0].Error == "modified" {
		t.Fatal("status did not return an independent snapshot")
	}
	fail.Store(false)
	if _, err := updater.UpdateAll(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	status = updater.Status()
	if status.State != "idle" || len(updater.manager.ListDatabases()) != 1 ||
		len(status.Results) != 1 || !status.Results[0].Updated || status.Results[0].Error != "" {
		t.Fatalf("download did not recover: %+v", status)
	}
}
