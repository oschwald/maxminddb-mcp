// Package database provides MaxMind database management functionality.
package database

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

	// Walk the directory looking for .mmdb files
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".mmdb") {
			if loadErr := m.loadDatabase(path, info); loadErr != nil {
				// Log error but continue processing other files
				fmt.Fprintf(os.Stderr, "Failed to load database %s: %v\n", path, loadErr)
			}
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
							fmt.Printf("Failed to load database %s: %v\n", event.Name, err)
						}
					}
				}

				if event.Op&fsnotify.Remove == fsnotify.Remove {
					m.RemoveDatabase(filepath.Base(event.Name))
				}

			case err, ok := <-m.watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
			}
		}
	}()
}

// GetReader returns a reader for the specified database.
func (m *Manager) GetReader(name string) (*maxminddb.Reader, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reader, exists := m.readers[name]
	return reader, exists
}

// GetDatabase returns database info for the specified database.
func (m *Manager) GetDatabase(name string) (*Info, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	db, exists := m.databases[name]
	return db, exists
}

// ListDatabases returns all available databases.
func (m *Manager) ListDatabases() []*Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Info, 0, len(m.databases))
	for _, db := range m.databases {
		result = append(result, db)
	}

	return result
}

// RemoveDatabase removes a database from the manager.
func (m *Manager) RemoveDatabase(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if reader, exists := m.readers[name]; exists {
		_ = reader.Close()
		delete(m.readers, name)
	}

	delete(m.databases, name)
}

// Close closes all database readers and the file watcher.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all readers
	for _, reader := range m.readers {
		_ = reader.Close()
	}

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

	name := filepath.Base(path)

	// Close existing reader if present
	if existingReader, exists := m.readers[name]; exists {
		_ = existingReader.Close()
	}

	// Create database info
	dbType := inferDatabaseType(name)
	description := getDatabaseDescription(dbType)

	dbInfo := &Info{
		Name:        name,
		Type:        dbType,
		Description: description,
		LastUpdated: info.ModTime(),
		Size:        info.Size(),
		Path:        path,
	}

	// Store reader and metadata
	m.readers[name] = reader
	m.databases[name] = dbInfo

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
