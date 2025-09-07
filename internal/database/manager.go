// Package database provides MaxMind database management functionality.
package database

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Info holds metadata about a database.
type Info struct {
	LastUpdated time.Time `json:"last_updated"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Path        string    `json:"-"`
	Size        int64     `json:"size"`
}

// Manager handles MMDB database lifecycle.
type Manager struct {
	readers   map[string]*maxminddb.Reader
	databases map[string]*Info
	watcher   *fsnotify.Watcher
	watchDirs []string
	mu        sync.RWMutex
}

// New creates a new database manager.
func New() (*Manager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &Manager{
		readers:   make(map[string]*maxminddb.Reader),
		databases: make(map[string]*Info),
		watcher:   watcher,
		watchDirs: make([]string, 0),
	}, nil
}

// LoadDirectory scans a directory for MMDB files and loads them.
func (m *Manager) LoadDirectory(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure directory exists
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	// Walk the directory looking for .mmdb files and subdirectories to watch
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != dir {
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

	// Add to watcher
	if err := m.watcher.Add(dir); err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	m.watchDirs = append(m.watchDirs, dir)
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
					// Convert to absolute path for removal
					absPath, err := filepath.Abs(event.Name)
					if err != nil {
						absPath = event.Name
					}
					m.RemoveDatabaseByPath(absPath)
				}

				if event.Op&fsnotify.Rename == fsnotify.Rename {
					// Handle rename as remove - the old path is no longer valid
					absPath, err := filepath.Abs(event.Name)
					if err != nil {
						absPath = event.Name
					}
					m.RemoveDatabaseByPath(absPath)

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
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find database by display name and return its reader
	for path, db := range m.databases {
		if db.Name == name {
			reader, exists := m.readers[path]
			return reader, exists
		}
	}
	return nil, false
}

// GetDatabase returns database info for the specified database by display name.
func (m *Manager) GetDatabase(name string) (*Info, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find database by display name
	for _, db := range m.databases {
		if db.Name == name {
			return db, true
		}
	}
	return nil, false
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

	// Find database by display name
	for path, db := range m.databases {
		if db.Name == name {
			// Remove from maps but don't close reader - let GC handle it
			// since iterators may still be using the reader
			delete(m.readers, path)
			delete(m.databases, path)
			break
		}
	}
}

// RemoveDatabaseByPath removes a database by absolute path from the manager.
func (m *Manager) RemoveDatabaseByPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove from maps but don't close reader - let GC handle it
	// since iterators may still be using the reader
	delete(m.readers, path)
	delete(m.databases, path)
}

// Close closes the file watcher and clears the database maps.
// Readers are not explicitly closed to avoid races with active iterators;
// they will be garbage collected when no longer referenced.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear maps but don't close readers - let GC handle them
	// since iterators may still be using the readers
	m.readers = make(map[string]*maxminddb.Reader)
	m.databases = make(map[string]*Info)

	// Close watcher
	return m.watcher.Close()
}

// loadDatabase loads a database file (must be called with lock held).
func (m *Manager) loadDatabase(path string, info os.FileInfo) error {
	// Open the database
	reader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open MMDB file %s: %w", path, err)
	}

	// Use absolute path as key to avoid collisions
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path // fallback to original path
	}

	// Don't close existing reader - let GC handle it since iterators may be using it
	// Just replace it in the map

	name := filepath.Base(path)
	dbType := inferDatabaseType(name)
	description := getDatabaseDescription(dbType)

	dbInfo := &Info{
		Name:        name, // Display name remains the base filename
		Type:        dbType,
		Description: description,
		LastUpdated: info.ModTime(),
		Size:        info.Size(),
		Path:        absPath, // Store absolute path
	}

	// Store reader and metadata using absolute path as key
	m.readers[absPath] = reader
	m.databases[absPath] = dbInfo

	return nil
}

// inferDatabaseType infers the database type from filename.
func inferDatabaseType(filename string) string {
	lower := strings.ToLower(filename)

	if strings.Contains(lower, "city") {
		return "City"
	}
	if strings.Contains(lower, "country") {
		return "Country"
	}
	if strings.Contains(lower, "asn") {
		return "ASN"
	}
	if strings.Contains(lower, "isp") {
		return "ISP"
	}
	if strings.Contains(lower, "domain") {
		return "Domain"
	}
	if strings.Contains(lower, "enterprise") {
		return "Enterprise"
	}
	if strings.Contains(lower, "anonymous") {
		return "Anonymous IP"
	}
	if strings.Contains(lower, "connection") {
		return "Connection Type"
	}

	return "Unknown"
}

// getDatabaseDescription returns a description for the database type.
func getDatabaseDescription(dbType string) string {
	descriptions := map[string]string{
		"City":            "IP geolocation with city-level precision",
		"Country":         "IP geolocation with country-level precision",
		"ASN":             "Autonomous system number and organization",
		"ISP":             "Internet service provider information",
		"Domain":          "Domain name information",
		"Enterprise":      "Enterprise-level IP intelligence",
		"Anonymous IP":    "Anonymous proxy and VPN detection",
		"Connection Type": "Connection type classification",
		"Unknown":         "MaxMind database file",
	}

	if desc, exists := descriptions[dbType]; exists {
		return desc
	}

	return descriptions["Unknown"]
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
