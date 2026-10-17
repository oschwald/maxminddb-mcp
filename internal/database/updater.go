package database

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 used for file integrity checksums, not cryptographic security
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/maxmind/geoipupdate/v8/client"
	"github.com/oschwald/maxminddb-mcp/internal/config"

	"github.com/oschwald/maxminddb-golang/v2"
)

// UpdateResult represents the result of an update operation.
type UpdateResult struct {
	LastUpdate time.Time `json:"last_update"`
	Database   string    `json:"database"`
	Error      string    `json:"error,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Updated    bool      `json:"updated"`
}

// Updater handles downloading and updating MaxMind databases.
type Updater struct {
	config   *config.Config
	client   *client.Client
	manager  *Manager
	updating chan struct{}
}

// NewUpdater creates a new database updater.
func NewUpdater(cfg *config.Config, manager *Manager) (*Updater, error) {
	if cfg.Mode != "maxmind" && cfg.Mode != "geoip_compat" {
		return nil, errors.New("updater only supports maxmind and geoip_compat modes")
	}

	// Create MaxMind client
	mclient, err := client.New(
		cfg.MaxMind.AccountID,
		cfg.MaxMind.LicenseKey,
		client.WithEndpoint(cfg.MaxMind.Endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MaxMind client: %w", err)
	}

	updater := &Updater{
		updating: make(chan struct{}, 1),
		config:   cfg,
		client:   &mclient,
		manager:  manager,
	}

	return updater, nil
}

// UpdateAll updates all configured databases.
func (u *Updater) UpdateAll(ctx context.Context) ([]UpdateResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case u.updating <- struct{}{}:
	}
	defer func() { <-u.updating }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	results := make([]UpdateResult, 0, len(u.config.MaxMind.Editions))

	for _, edition := range u.config.MaxMind.Editions {
		result := u.updateDatabase(ctx, edition)
		results = append(results, result)
	}

	return results, nil
}

// UpdateDatabase updates a specific database.
func (u *Updater) UpdateDatabase(ctx context.Context, edition string) (UpdateResult, error) {
	select {
	case <-ctx.Done():
		return UpdateResult{}, ctx.Err()
	case u.updating <- struct{}{}:
	}
	defer func() { <-u.updating }()
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, err
	}

	result := u.updateDatabase(ctx, edition)
	return result, nil
}

// RunUpdates checks for updates at startup and on schedule until canceled.
func (u *Updater) RunUpdates(ctx context.Context) {
	if u.config.AutoUpdate || len(u.manager.ListDatabases()) == 0 {
		u.updateAndLog(ctx)
	}
	if !u.config.AutoUpdate {
		return
	}
	ticker := time.NewTicker(u.config.UpdateIntervalDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.updateAndLog(ctx)
		}
	}
}

func (u *Updater) updateAndLog(ctx context.Context) {
	results, err := u.UpdateAll(ctx)
	if err != nil {
		slog.Error("Database update failed", "err", err)
	}
	for _, result := range results {
		if result.Error != "" {
			slog.Error("Database update failed", "edition", result.Database, "error", result.Error)
		} else if result.Updated {
			slog.Info("Database updated", "edition", result.Database, "size", result.Size)
		}
	}
}

// updateDatabase performs the actual update (must be called with lock held).
func (u *Updater) updateDatabase(ctx context.Context, edition string) UpdateResult {
	result := UpdateResult{
		Database: edition,
		Updated:  false,
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if edition == "" || filepath.Base(edition) != edition || edition == "." || edition == ".." {
		result.Error = "edition must be a database edition ID without a path"
		return result
	}
	dbPath := filepath.Join(u.config.MaxMind.DatabaseDir, edition+".mmdb")
	currentMD5, err := databaseChecksum(dbPath)
	if err != nil {
		result.Error = fmt.Sprintf("read current database: %v", err)
		return result
	}

	// Attempt to download
	response, err := u.client.Download(ctx, edition, currentMD5)
	if err != nil {
		result.Error = fmt.Sprintf("download failed: %v", err)
		return result
	}
	defer func() { _ = response.Reader.Close() }()

	if !response.UpdateAvailable {
		if err := u.manager.LoadDatabase(dbPath); err != nil {
			result.Error = fmt.Sprintf("load current database: %v", err)
		}
		if info, ok := u.manager.GetDatabase(edition + ".mmdb"); ok {
			result.LastUpdate, result.Size = info.LastUpdated, info.Size
		}
		return result
	}

	// Create database directory if it doesn't exist
	if err := os.MkdirAll(u.config.MaxMind.DatabaseDir, 0o750); err != nil {
		result.Error = fmt.Sprintf("failed to create database directory: %v", err)
		return result
	}

	// Write to temporary file first
	file, err := os.CreateTemp(u.config.MaxMind.DatabaseDir, edition+"-*.tmp")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		return result
	}
	tempPath := file.Name()
	defer func() { _ = os.Remove(tempPath) }()

	// Copy data and calculate MD5
	hasher := md5.New() //nolint:gosec // MD5 used for file integrity checksums, not cryptographic security
	writer := io.MultiWriter(file, hasher)

	size, err := io.Copy(writer, response.Reader)
	if err != nil {
		_ = file.Close()
		result.Error = fmt.Sprintf("failed to write database: %v", err)
		return result
	}

	if err := file.Close(); err != nil {
		result.Error = fmt.Sprintf("failed to close file: %v", err)
		return result
	}

	// Verify MD5 if provided
	newMD5 := hex.EncodeToString(hasher.Sum(nil))
	if response.MD5 != "" && response.MD5 != newMD5 {
		result.Error = fmt.Sprintf("MD5 mismatch: expected %s, got %s", response.MD5, newMD5)
		return result
	}

	reader, err := maxminddb.Open(tempPath)
	if err != nil {
		result.Error = fmt.Sprintf("invalid downloaded database: %v", err)
		return result
	}
	err = reader.Verify()
	_ = reader.Close()
	if err != nil {
		result.Error = fmt.Sprintf("invalid downloaded database: %v", err)
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Error = err.Error()
		return result
	}

	// Atomically replace the old file
	if err := os.Rename(tempPath, dbPath); err != nil {
		result.Error = fmt.Sprintf("failed to replace database file: %v", err)
		return result
	}

	// Report the installed file, including any reload failure.
	result.Updated = true
	result.LastUpdate = response.LastModified
	result.Size = size

	// Reload in manager
	if err := u.manager.LoadDatabase(dbPath); err != nil {
		result.Error = fmt.Sprintf("failed to reload database in manager: %v", err)
	}

	return result
}

// databaseChecksum reads the installed file so stale sidecar files cannot suppress repairs.
func databaseChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hasher := md5.New() //nolint:gosec // The update API requires MD5 for file integrity checks.
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
