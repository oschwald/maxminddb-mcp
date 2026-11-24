// Package database provides MaxMind database management functionality.
package database

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Info holds metadata about a database.
type Info struct {
	Generation  string    `json:"-"`
	LastUpdated time.Time `json:"last_updated" jsonschema:"Database build time from MMDB metadata, not the time the file was downloaded or modified."`
	fileInfo    os.FileInfo
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Path        string `json:"-"`
	Size        int64  `json:"size"         jsonschema:"Database file size in bytes."`
}

// Manager handles MMDB database lifecycle.
type Manager struct {
	readers       map[string]*maxminddb.Reader
	databases     map[string]*Info
	displayToPath map[string]string // Fast lookup from display name to absolute path
	watcher       *fsnotify.Watcher
	installing    chan struct{}
	roots         map[string]struct{} // Resolved configured directories
	watchDirs     []string
	mu            sync.RWMutex
}

// New creates a new database manager.
func New() (*Manager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &Manager{
		readers:       make(map[string]*maxminddb.Reader),
		databases:     make(map[string]*Info),
		displayToPath: make(map[string]string),
		watcher:       watcher,
		installing:    make(chan struct{}, 1),
		roots:         make(map[string]struct{}),
		watchDirs:     make([]string, 0),
	}, nil
}

// LoadDirectory scans a directory for MMDB files and loads them. A symlinked
// directory is resolved first. Subdirectories that cannot be read are skipped.
func (m *Manager) LoadDirectory(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := resolveDirectory(dir)
	if err != nil {
		return err
	}
	m.roots[root] = struct{}{}

	// Walk the directory looking for .mmdb files and subdirectories to watch
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root ||
				(!errors.Is(err, fs.ErrPermission) && !errors.Is(err, fs.ErrNotExist)) {
				return err
			}
			slog.Warn("Skipping unreadable directory", "path", path, "err", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() && path != root {
			return m.watchSubdirectory(path)
		}

		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".mmdb") {
			return m.loadMMDBFile(path, d)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan directory %s: %w", dir, err)
	}

	return nil
}

// LoadDatabase loads a single MMDB file.
func (m *Manager) LoadDatabase(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	return m.loadDatabase(path, info)
}

// WatchDirectory adds a directory to be watched for file changes.
func (m *Manager) WatchDirectory(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	root, err := resolveDirectory(dir)
	if err != nil {
		return err
	}
	m.roots[root] = struct{}{}

	// Add to watcher
	if err := m.watcher.Add(root); err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	m.watchDirs = append(m.watchDirs, root)
	return nil
}

// StartWatching starts the file watcher goroutine.
func (m *Manager) StartWatching() {
	go func() {
		for {
			select {
			case event, ok := <-m.watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Create == fsnotify.Create {
					if strings.HasSuffix(strings.ToLower(event.Name), ".mmdb") {
						if err := m.LoadDatabase(event.Name); err != nil {
							slog.Warn(
								"Failed to load database on event",
								"path",
								event.Name,
								"err",
								err,
							)
						}
					}
				}

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					m.RemoveDatabaseByPath(event.Name)
				}

				if event.Op&fsnotify.Rename == fsnotify.Rename {
					// Handle rename as remove - the old path is no longer valid
					m.RemoveDatabaseByPath(event.Name)

					// Note: We don't try to reload here because event.Name is the old path.
					// If the file was renamed within a watched directory, we'll get a
					// subsequent Create event for the new path.
				}

			case err, ok := <-m.watcher.Errors:
				if !ok {
					return
				}
				slog.Error("Watcher error", "err", err)
			}
		}
	}()
}

// GetReader returns a reader for the specified database by display name.
func (m *Manager) GetReader(name string) (*maxminddb.Reader, bool) {
	reader, _, ok := m.Snapshot(name)
	return reader, ok
}

// Snapshot returns a reader and its generation under the same lock.
func (m *Manager) Snapshot(name string) (*maxminddb.Reader, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Fast O(1) lookup using display name to path map
	path, exists := m.displayToPath[name]
	if !exists {
		return nil, "", false
	}
	reader, exists := m.readers[path]
	return reader, m.databases[path].Generation, exists
}

// GetDatabase returns database info for the specified database by display name.
func (m *Manager) GetDatabase(name string) (*Info, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Fast O(1) lookup using display name to path map
	path, exists := m.displayToPath[name]
	if !exists {
		return nil, false
	}
	db, exists := m.databases[path]
	return db, exists
}

// ListDatabases returns all available databases.
func (m *Manager) ListDatabases() []*Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Collect(maps.Values(m.databases))
}

// RemoveDatabase removes a database by display name from the manager.
func (m *Manager) RemoveDatabase(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Fast O(1) lookup to find path by display name
	path, exists := m.displayToPath[name]
	if exists {
		// Remove from all maps but don't close reader - let GC handle it
		// since iterators may still be using the reader
		delete(m.readers, path)
		delete(m.databases, path)
		delete(m.displayToPath, name)
	}
}

// RemoveDatabaseByPath removes the database at path from the manager. If path
// is a subdirectory of a configured directory, it also removes the databases
// below it. Databases stay loaded when a configured directory itself is
// removed, as its files are reported separately.
func (m *Manager) RemoveDatabaseByPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if resolved, err := databasePath(path); err == nil {
		path = resolved
	}
	_, isRoot := m.roots[path]
	prefix := path + string(filepath.Separator)
	below := func(p string) bool { return !isRoot && strings.HasPrefix(p, prefix) }

	// Remove from all maps but don't close reader - let GC handle it
	// since iterators may still be using the reader
	for dbPath, db := range m.databases {
		if dbPath != path && !below(dbPath) {
			continue
		}
		if m.displayToPath[db.Name] == dbPath {
			delete(m.displayToPath, db.Name)
		}
		delete(m.readers, dbPath)
		delete(m.databases, dbPath)
	}

	// The watcher drops watches on deleted directories itself. Calling
	// watcher.Remove here could deadlock with the event loop on Windows.
	m.watchDirs = slices.DeleteFunc(m.watchDirs, func(dir string) bool {
		return dir == path || below(dir)
	})
}

// Close closes the file watcher and clears the database maps.
// Readers are not explicitly closed to avoid races with active iterators;
// they will be garbage collected when no longer referenced.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all maps but don't close readers - let GC handle them
	// since iterators may still be using the readers
	m.readers = make(map[string]*maxminddb.Reader)
	m.databases = make(map[string]*Info)
	m.displayToPath = make(map[string]string)

	// Close watcher
	return m.watcher.Close()
}

// loadDatabase loads a database file (must be called with lock held).
func (m *Manager) loadDatabase(path string, info os.FileInfo) error {
	absPath, err := databasePath(path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	name := filepath.Base(path)
	if existing, ok := m.displayToPath[name]; ok && existing != absPath {
		return fmt.Errorf(
			"duplicate database name %s: rename one of %s and %s",
			name,
			existing,
			absPath,
		)
	}
	if previous := m.databases[absPath]; previous != nil && previous.fileInfo != nil &&
		m.readers[absPath] != nil && os.SameFile(previous.fileInfo, info) &&
		previous.fileInfo.Size() == info.Size() && previous.fileInfo.ModTime().Equal(info.ModTime()) {
		return nil
	}
	fileInfo := info
	// Resolve Windows file IDs before the path can refer to a replacement.
	if !os.SameFile(info, info) {
		fileInfo = nil
	}
	var reader *maxminddb.Reader
	if runtime.GOOS == "windows" {
		// Windows mappings prevent replacement while readers retain the old database.
		var data []byte
		data, err = os.ReadFile(path)
		if err == nil {
			reader, err = maxminddb.OpenBytes(data)
		}
	} else {
		reader, err = maxminddb.Open(path)
	}
	if err != nil {
		return fmt.Errorf("failed to open MMDB file %s: %w", path, err)
	}

	dbInfo := &Info{
		Generation:  rand.Text(),
		fileInfo:    fileInfo,
		Name:        name, // Display name remains the base filename
		Type:        reader.Metadata.DatabaseType,
		Description: reader.Metadata.Description["en"],
		LastUpdated: reader.Metadata.BuildTime(),
		Size:        info.Size(),
		Path:        absPath, // Store absolute path
	}

	// Store reader and metadata using absolute path as key
	m.readers[absPath] = reader
	m.databases[absPath] = dbInfo

	// Update display name to path mapping for O(1) lookups
	m.displayToPath[dbInfo.Name] = absPath

	return nil
}

// resolveDirectory returns the absolute path of dir with symlinks resolved.
// If symlinks cannot be resolved, as on some Windows volumes, it returns the
// absolute path.
func resolveDirectory(dir string) (string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("directory does not exist: %s", dir)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		slog.Debug("Failed to resolve directory symlinks", "path", dir, "err", err)
		resolved = dir
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve directory %s: %w", dir, err)
	}
	return resolved, nil
}

// databasePath returns the key used for a database file: an absolute path
// whose parent directory has its symlinks resolved. The filename itself is
// kept, so a symlinked database keeps its own name. When the parent directory
// cannot be resolved, such as after it was removed, the absolute path is
// returned, matching resolveDirectory.
func databasePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absPath))
	if err != nil {
		return absPath, nil //nolint:nilerr // Fall back like resolveDirectory.
	}
	return filepath.Join(parent, filepath.Base(absPath)), nil
}

// watchSubdirectory adds a subdirectory to the watcher.
func (m *Manager) watchSubdirectory(path string) error {
	if watchErr := m.watcher.Add(path); watchErr != nil {
		slog.Warn("Failed to watch subdirectory", "path", path, "err", watchErr)
		return nil // Continue processing other directories
	}
	m.watchDirs = append(m.watchDirs, path)
	return nil
}

// loadMMDBFile loads a single MMDB file entry.
func (m *Manager) loadMMDBFile(path string, d os.DirEntry) error {
	info, statErr := d.Info()
	if statErr != nil {
		slog.Warn("Failed to get file info", "path", path, "err", statErr)
		return nil // Continue processing other files
	}
	if loadErr := m.loadDatabase(path, info); loadErr != nil {
		// Log error but continue processing other files
		slog.Warn("Failed to load database", "path", path, "err", loadErr)
	}
	return nil
}
