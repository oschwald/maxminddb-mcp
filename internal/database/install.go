package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/oschwald/maxminddb-golang/v2"
)

const maxDatabaseBytes = 1 << 30

func lockDatabase(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	// Keep the lock file so waiting processes always lock the same file.
	lock := flock.New(path + ".lock")
	if _, err := lock.TryLockContext(ctx, 100*time.Millisecond); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("lock database update: %w", err)
	}
	return func() {
		if err := lock.Close(); err != nil {
			slog.Error("Failed to unlock database update", "err", err)
		}
	}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func copyDatabase(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	n, err := io.Copy(dst, io.LimitReader(contextReader{ctx: ctx, reader: src}, maxDatabaseBytes+1))
	if n > maxDatabaseBytes {
		return n, errors.New("database download or expanded archive exceeds 1 GiB")
	}
	return n, err
}

func (m *Manager) replaceDatabase(
	ctx context.Context,
	temporary, path, checksum string,
	modified time.Time,
	size int64,
) UpdateResult {
	result := UpdateResult{Database: strings.TrimSuffix(filepath.Base(path), ".mmdb")}
	if checksum != "" {
		actual, err := databaseChecksum(temporary)
		if err != nil {
			result.Error = "read downloaded checksum: " + err.Error()
			return result
		}
		if actual != checksum {
			result.Error = fmt.Sprintf("MD5 mismatch: expected %s, got %s", checksum, actual)
			return result
		}
	}
	reader, err := maxminddb.Open(temporary)
	if err == nil {
		err = reader.Verify()
		_ = reader.Close()
	}
	if err != nil {
		result.Error = "invalid downloaded database: " + err.Error()
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := os.Rename(temporary, path); err != nil {
		result.Error = "failed to replace database file: " + err.Error()
		return result
	}
	result.Updated, result.LastUpdate, result.Size = true, modified, size
	if err := m.LoadDatabase(path); err != nil {
		result.Error = "failed to reload database in manager: " + err.Error()
	}
	return result
}
