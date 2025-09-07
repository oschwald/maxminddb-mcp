package database

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 used for file integrity checksums, not cryptographic security
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maxmind/geoipupdate/v7/client"
	"github.com/oschwald/maxminddb-mcp/internal/config"
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
	config    *config.Config
	client    *client.Client
	manager   *Manager
	checksums map[string]string
	mu        sync.RWMutex
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
		config:    cfg,
		client:    &mclient,
		manager:   manager,
		checksums: make(map[string]string),
	}

	// Load existing checksums
	updater.loadChecksums()

	return updater, nil
}

// UpdateAll updates all configured databases.
func (u *Updater) UpdateAll(ctx context.Context) ([]UpdateResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	results := make([]UpdateResult, 0, len(u.config.MaxMind.Editions))

	for _, edition := range u.config.MaxMind.Editions {
		result := u.updateDatabase(ctx, edition)
		results = append(results, result)
	}

	// Save updated checksums
	u.saveChecksums()

	return results, nil
}

// UpdateDatabase updates a specific database.
func (u *Updater) UpdateDatabase(ctx context.Context, edition string) (UpdateResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	result := u.updateDatabase(ctx, edition)
	u.saveChecksums()

	return result, nil
}

// StartScheduledUpdates starts a goroutine that periodically updates databases.
func (u *Updater) StartScheduledUpdates(ctx context.Context) {
	if !u.config.AutoUpdate {
		return
	}

	go func() {
		ticker := time.NewTicker(u.config.UpdateIntervalDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				results, err := u.UpdateAll(ctx)
				if err != nil {
					slog.Error("Scheduled update failed", "err", err)
					continue
				}

				// Log update results
				for _, result := range results {
					if result.Error != "" {
						slog.Error(
							"Database update error",
							"edition",
							result.Database,
							"error",
							result.Error,
						)
					} else if result.Updated {
						slog.Info("Database updated", "edition", result.Database, "size", result.Size)
					}
				}
			}
		}
	}()
}

// updateDatabase performs the actual update (must be called with lock held).
func (u *Updater) updateDatabase(ctx context.Context, edition string) UpdateResult {
	result := UpdateResult{
		Database: edition,
		Updated:  false,
	}

	// Get current MD5 for this edition
	currentMD5 := u.checksums[edition]

	// Attempt to download
	response, err := u.client.Download(ctx, edition, currentMD5)
	if err != nil {
		result.Error = fmt.Sprintf("download failed: %v", err)
		return result
	}
	defer func() { _ = response.Reader.Close() }()

	if !response.UpdateAvailable {
		result.LastUpdate = response.LastModified
		return result // No update needed
	}

	// Create database directory if it doesn't exist
	if err := os.MkdirAll(u.config.MaxMind.DatabaseDir, 0o750); err != nil {
		result.Error = fmt.Sprintf("failed to create database directory: %v", err)
		return result
	}

	// Write to temporary file first
	dbPath := filepath.Join(u.config.MaxMind.DatabaseDir, edition+".mmdb")
	tempPath := dbPath + ".tmp"

	file, err := os.Create(tempPath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		return result
	}

	// Copy data and calculate MD5
	hasher := md5.New() //nolint:gosec // MD5 used for file integrity checksums, not cryptographic security
	writer := io.MultiWriter(file, hasher)

	size, err := io.Copy(writer, response.Reader)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to write database: %v", err)
		return result
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to close file: %v", err)
		return result
	}

	// Verify MD5 if provided
	newMD5 := hex.EncodeToString(hasher.Sum(nil))
	if response.MD5 != "" && response.MD5 != newMD5 {
		_ = os.Remove(tempPath)
		result.Error = fmt.Sprintf("MD5 mismatch: expected %s, got %s", response.MD5, newMD5)
		return result
	}

	// Atomically replace the old file
	if err := os.Rename(tempPath, dbPath); err != nil {
		_ = os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to replace database file: %v", err)
		return result
	}

	// Update checksum and result
	u.checksums[edition] = newMD5
	result.Updated = true
	result.LastUpdate = response.LastModified
	result.Size = size

	// Reload in manager
	if err := u.manager.LoadDatabase(dbPath); err != nil {
		result.Error = fmt.Sprintf("warning: failed to reload database in manager: %v", err)
		// Don't fail the update for this
	}

	return result
}

// loadChecksums loads existing MD5 checksums from file.
func (u *Updater) loadChecksums() {
	checksumFile := filepath.Join(u.config.MaxMind.DatabaseDir, ".checksums")

	data, err := os.ReadFile(checksumFile)
	if err != nil {
		// File doesn't exist or can't be read, start fresh
		return
	}

	// Parse simple format: edition:md5
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			u.checksums[parts[0]] = parts[1]
		}
	}
}

// saveChecksums saves current MD5 checksums to file.
func (u *Updater) saveChecksums() {
	checksumFile := filepath.Join(u.config.MaxMind.DatabaseDir, ".checksums")

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(checksumFile), 0o750); err != nil {
		slog.Error(
			"Failed to create checksum directory",
			"dir",
			filepath.Dir(checksumFile),
			"err",
			err,
		)
		return
	}

	file, err := os.Create(checksumFile)
	if err != nil {
		slog.Error("Failed to create checksum file", "path", checksumFile, "err", err)
		return
	}
	defer func() { _ = file.Close() }()

	for edition, checksum := range u.checksums {
		if _, err := fmt.Fprintf(file, "%s:%s\n", edition, checksum); err != nil {
			slog.Error("Failed to write checksum", "edition", edition, "err", err)
		}
	}
}
