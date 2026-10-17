// MaxMindDB MCP Server provides Model Context Protocol access to MaxMind databases.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
	"github.com/oschwald/maxminddb-mcp/internal/mcp"
)

// These variables are set by GoReleaser at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Check for version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("maxminddb-mcp %s (commit: %s, built: %s)\n", version, commit, date)
		return
	}

	// Check for help flag
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		return
	}
	setupLogger()

	// Load configuration using centralized loader
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Server failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, input io.Reader, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dbManager, err := database.New()
	if err != nil {
		return fmt.Errorf("create database manager: %w", err)
	}
	defer func() { _ = dbManager.Close() }()
	if err := initializeDatabases(cfg, dbManager); err != nil {
		return err
	}
	var updater *database.Updater
	if cfg.Mode == config.ModeMaxMind || cfg.Mode == config.ModeGeoIPCompat {
		updater, err = database.NewUpdater(cfg, dbManager)
		if err != nil {
			return fmt.Errorf("create updater: %w", err)
		}
	}
	iterMgr := iterator.New(cfg.IteratorTTLDuration, cfg.IteratorCleanupIntervalDuration)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()
	dbManager.StartWatching()
	if updater != nil {
		done := make(chan struct{})
		go func() { defer close(done); updater.RunUpdates(ctx) }()
		defer func() { cancel(); <-done }()
	}
	logStartupSummary(cfg, dbManager, updater != nil && cfg.AutoUpdate)
	return mcp.New(cfg, dbManager, updater, iterMgr, version).Serve(ctx, input, output)
}

// printHelp displays usage information.
func printHelp() {
	fmt.Printf(`MaxMindDB MCP Server %s

A powerful Model Context Protocol (MCP) server that provides comprehensive 
geolocation and network intelligence through MaxMindDB databases.

Usage:
  maxminddb-mcp [flags]

Flags:
  -h, --help     Show this help message
  -v, --version  Show version information

Environment Variables:
  MAXMINDDB_MCP_CONFIG      Path to configuration file
  MAXMINDDB_MCP_LOG_LEVEL   Logging level (debug|info|warn|error)
  MAXMINDDB_MCP_LOG_FORMAT  Log format (text|json)

Configuration:
  The server looks for configuration in this order:
  1. MAXMINDDB_MCP_CONFIG environment variable
  2. ~/.config/maxminddb-mcp/config.toml
  3. /etc/GeoIP.conf or ~/.config/maxminddb-mcp/GeoIP.conf

For more information, visit: https://github.com/oschwald/maxminddb-mcp

`, version)
}

// setupLogger configures a global slog logger with simple env controls.
func setupLogger() {
	format := strings.ToLower(os.Getenv("MAXMINDDB_MCP_LOG_FORMAT"))  // "text" (default) or "json"
	levelStr := strings.ToLower(os.Getenv("MAXMINDDB_MCP_LOG_LEVEL")) // debug|info|warn|error
	var lvl slog.Level
	switch levelStr {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func initializeDatabases(cfg *config.Config, dbManager *database.Manager) error {
	switch cfg.Mode {
	case config.ModeMaxMind, config.ModeGeoIPCompat:
		// Ensure database directory exists
		if err := os.MkdirAll(cfg.MaxMind.DatabaseDir, 0o750); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		// Load existing databases and watch directory
		if err := dbManager.LoadDirectory(cfg.MaxMind.DatabaseDir); err != nil {
			return fmt.Errorf("failed to load databases: %w", err)
		}
		return dbManager.WatchDirectory(cfg.MaxMind.DatabaseDir)

	case config.ModeDirectory:
		// Load all configured directories
		for _, path := range cfg.Directory.Paths {
			if err := dbManager.LoadDirectory(path); err != nil {
				return fmt.Errorf("failed to load directory %s: %w", path, err)
			}
			if err := dbManager.WatchDirectory(path); err != nil {
				return fmt.Errorf("failed to watch directory %s: %w", path, err)
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown mode: %s", cfg.Mode)
	}
}

func logStartupSummary(cfg *config.Config, dbManager *database.Manager, autoUpdateEnabled bool) {
	databases := dbManager.ListDatabases()

	slog.Info("MaxMindDB MCP Server starting",
		"mode", cfg.Mode,
		"databases_loaded", len(databases),
		"auto_update_enabled", autoUpdateEnabled,
		"iterator_ttl", cfg.IteratorTTL,
		"iterator_cleanup_interval", cfg.IteratorCleanupInterval,
	)

	switch cfg.Mode {
	case config.ModeMaxMind, config.ModeGeoIPCompat:
		slog.Info("MaxMind mode configuration",
			"database_dir", cfg.MaxMind.DatabaseDir,
			"editions_count", len(cfg.MaxMind.Editions),
			"update_interval", cfg.UpdateInterval,
		)
		if len(cfg.MaxMind.Editions) > 0 {
			slog.Debug("Configured editions", "editions", cfg.MaxMind.Editions)
		}
	case config.ModeDirectory:
		slog.Info("Directory mode configuration",
			"watched_paths_count", len(cfg.Directory.Paths),
		)
		if len(cfg.Directory.Paths) > 0 {
			slog.Debug("Watched directories", "paths", cfg.Directory.Paths)
		}
	default:
		slog.Warn("Unknown mode configuration", "mode", cfg.Mode)
	}

	if len(databases) > 0 {
		dbNames := make([]string, len(databases))
		for i, db := range databases {
			dbNames[i] = db.Name
		}
		slog.Info("Loaded databases", "databases", dbNames)
	} else {
		slog.Warn("No databases loaded - server may not function correctly")
	}
}
