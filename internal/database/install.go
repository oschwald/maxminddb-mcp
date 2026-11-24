package database

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	maxDatabaseBytes = 1 << 30
	// newDatabaseMode is the mode geoipupdate requests for new databases. The
	// umask still applies, so a restrictive umask keeps new files private.
	newDatabaseMode fs.FileMode = 0o644
)

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
	if err := setDatabaseMode(temporary, path); err != nil {
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

// setDatabaseMode gives temporary the permission bits of the installed file at
// path, so a replacement does not change who can read it. A new database gets
// newDatabaseMode less the umask. Staging files stay private (os.CreateTemp
// uses 0600) until this runs, just before the rename that publishes them.
func setDatabaseMode(temporary, path string) error {
	if runtime.GOOS == "windows" {
		// Windows only has a read-only attribute. Copying it would leave a
		// temporary file that cannot be cleaned up if the rename fails.
		return nil
	}
	var mode fs.FileMode
	info, err := os.Stat(path)
	switch {
	case err == nil:
		mode = info.Mode().Perm()
	case errors.Is(err, fs.ErrNotExist):
		mode, err = newFileMode(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("determine new database mode: %w", err)
		}
	default:
		return fmt.Errorf("read installed database mode: %w", err)
	}
	if err := os.Chmod(temporary, mode); err != nil {
		return fmt.Errorf("set database file mode: %w", err)
	}
	return nil
}

// newFileMode returns the mode a file created in dir with newDatabaseMode gets
// under the current umask. It creates and removes an empty probe file, so no
// database contents are ever readable through a broader mode.
func newFileMode(dir string) (fs.FileMode, error) {
	for range 100 {
		name := filepath.Join(dir, ".mode-"+rand.Text()+".tmp")
		probe, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE|os.O_EXCL, newDatabaseMode)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		info, statErr := probe.Stat()
		closeErr := probe.Close()
		removeErr := os.Remove(name)
		if err := errors.Join(statErr, closeErr, removeErr); err != nil {
			return 0, err
		}
		return info.Mode().Perm(), nil
	}
	return 0, fmt.Errorf("create mode probe in %s: %w", dir, fs.ErrExist)
}
