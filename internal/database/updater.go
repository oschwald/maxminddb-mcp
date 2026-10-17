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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/maxmind/geoipupdate/v8/client"
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
	config   *config.Config
	client   *client.Client
	manager  *Manager
	updating chan struct{}
	status   UpdateStatus
	statusMu sync.RWMutex
}

// UpdateStatus describes the active or most recent download attempt.
type UpdateStatus struct {
	State   string         `json:"state"`
	Error   string         `json:"error,omitempty"`
	Results []UpdateResult `json:"results,omitempty"`
}

// Status returns a snapshot without waiting for an active download.
func (u *Updater) Status() UpdateStatus {
	u.statusMu.RLock()
	defer u.statusMu.RUnlock()
	status := u.status
	status.Results = slices.Clone(status.Results)
	return status
}

func (u *Updater) setStatus(status UpdateStatus) {
	u.statusMu.Lock()
	defer u.statusMu.Unlock()
	u.status = status
}

func (u *Updater) finishUpdate(results []UpdateResult, err error) {
	status := UpdateStatus{State: "idle", Results: slices.Clone(results)}
	if err != nil {
		status.State, status.Error = "failed", err.Error()
	}
	for _, result := range status.Results {
		if result.Error != "" {
			status.State = "failed"
		}
	}
	status.Error = u.redact(status.Error)
	u.setStatus(status)
}

func (u *Updater) redact(message string) string {
	if key := u.config.MaxMind.LicenseKey; key != "" {
		return strings.ReplaceAll(message, key, "[redacted]")
	}
	return message
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
		status:   UpdateStatus{State: "idle"},
	}
	if cfg.AutoUpdate || len(manager.ListDatabases()) == 0 {
		updater.status.State = "pending"
	}

	return updater, nil
}

// UpdateAll updates all configured databases.
func (u *Updater) UpdateAll(
	ctx context.Context,
	report func(string),
) (results []UpdateResult, err error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case u.updating <- struct{}{}:
	}
	defer func() { <-u.updating }()
	u.setStatus(UpdateStatus{State: "updating"})
	defer func() { u.finishUpdate(results, err) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	editions, err := u.editions()
	if err != nil {
		return nil, err
	}
	results = make([]UpdateResult, 0, len(editions))

	for _, edition := range editions {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result := u.updateDatabase(ctx, edition, report)
		results = append(results, result)
	}

	return results, nil
}

// InstallEdition downloads an edition and remembers it for future updates.
func (u *Updater) InstallEdition(
	ctx context.Context,
	edition string,
	report func(string),
) (result UpdateResult, err error) {
	select {
	case <-ctx.Done():
		return UpdateResult{}, ctx.Err()
	case u.updating <- struct{}{}:
	}
	defer func() { <-u.updating }()
	u.setStatus(UpdateStatus{State: "updating"})
	defer func() { u.finishUpdate([]UpdateResult{result}, err) }()
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, err
	}

	result = u.updateDatabase(ctx, edition, report)
	if result.Error == "" && !slices.Contains(u.config.MaxMind.Editions, edition) {
		if err := u.rememberEdition(edition); err != nil {
			result.Error = "Database installed, but could not enable future updates: " + err.Error()
		}
	}
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
	slog.Info("Checking for database updates")
	results, err := u.UpdateAll(ctx, nil)
	if err != nil {
		slog.Error("Database update failed", "err", u.redact(err.Error()))
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
func (u *Updater) updateDatabase(
	ctx context.Context,
	edition string,
	report func(string),
) (result UpdateResult) {
	defer func() { result.Error = u.redact(result.Error) }()
	result = UpdateResult{
		Database: edition,
		Updated:  false,
	}

	if !validEdition(edition) {
		result.Error = "edition must be a database edition ID without a path"
		return result
	}
	if report != nil {
		report("Checking " + edition)
	}
	dbPath := filepath.Join(u.config.MaxMind.DatabaseDir, edition+".mmdb")
	unlock, err := lockDatabase(ctx, dbPath)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer unlock()

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

	file, err := os.CreateTemp(u.config.MaxMind.DatabaseDir, edition+"-*.tmp")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		return result
	}
	defer func() { _ = os.Remove(file.Name()) }()
	writer := io.Writer(file)
	if report != nil {
		report("Downloading " + edition)
		writer = io.MultiWriter(writer, &downloadProgress{report: report, edition: edition})
	}
	size, copyErr := io.Copy(writer, contextReader{ctx: ctx, reader: response.Reader})
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		result.Error = fmt.Sprintf("failed to write database: %v", err)
		return result
	}
	if report != nil {
		report("Validating " + edition)
	}
	return u.manager.replaceDatabase(
		ctx,
		file.Name(),
		dbPath,
		response.MD5,
		response.LastModified,
		size,
	)
}

type downloadProgress struct {
	last    time.Time
	report  func(string)
	edition string
	bytes   int64
}

func (p *downloadProgress) Write(data []byte) (int, error) {
	p.bytes += int64(len(data))
	if time.Since(p.last) >= time.Second {
		p.last = time.Now()
		p.report(fmt.Sprintf("Downloading %s: %d bytes received", p.edition, p.bytes))
	}
	return len(data), nil
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
