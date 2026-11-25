//go:build unix

package database

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestNamedPipeDatabaseIsNotOpened(t *testing.T) {
	dir := resolvedTempDir(t)
	copyTestDB(t, dir, "good.mmdb")
	pipe := filepath.Join(dir, "pipe.mmdb")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Skipf("named pipes are unavailable: %v", err)
	}

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	done := make(chan error, 2)
	go func() {
		done <- manager.LoadDirectory(dir)
		done <- manager.LoadDatabase(pipe)
	}()
	for i, wantErr := range []bool{false, true} {
		select {
		case err := <-done:
			if (err != nil) != wantErr {
				t.Fatalf("load %d error = %v, want error %t", i, err, wantErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("opening a named pipe blocked")
		}
	}
	if _, ok := manager.GetDatabase("good.mmdb"); !ok {
		t.Fatal("database beside the named pipe was not loaded")
	}
	if _, ok := manager.GetDatabase("pipe.mmdb"); ok {
		t.Fatal("named pipe was loaded")
	}
}
