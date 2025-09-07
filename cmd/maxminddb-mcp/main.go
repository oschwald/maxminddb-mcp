// MaxMind MMDB MCP Server provides Model Context Protocol access to MaxMind databases.
package main

import (
	"context"
	"fmt"
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

	// Create database manager
	dbManager, err := database.New()
	if err != nil {
		slog.Error("Failed to create database manager", "err", err)
		os.Exit(1)
	}

	// Initialize databases based on mode
	if err := initializeDatabases(cfg, dbManager); err != nil {
		slog.Error("Failed to initialize databases", "err", err)
		os.Exit(1)
	}

	// Create updater if needed
	var updater *database.Updater
	if cfg.Mode == config.ModeMaxMind || cfg.Mode == config.ModeGeoIPCompat {
		updater, err = database.NewUpdater(cfg, dbManager)
		if err != nil {
			slog.Warn("Failed to create updater", "err", err)
		}
	}

	// Create iterator manager with configurable buffer size
	iterMgr := iterator.New(
		cfg.IteratorTTLDuration,
		cfg.IteratorCleanupIntervalDuration,
		cfg.IteratorBuffer,
	)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check if databases need initial update
	if updater != nil && len(dbManager.ListDatabases()) == 0 {
		slog.Info("No databases found, triggering initial update...")
		if _, err := updater.UpdateAll(ctx); err != nil {
			slog.Warn("Initial database update failed", "err", err)
		}
	}

	// Start scheduled updates if configured
	if updater != nil {
		updater.StartScheduledUpdates(ctx)
	}

	// Start file watcher
	dbManager.StartWatching()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutting down...")
		cancel()
	}()

	// Log startup summary
	logStartupSummary(cfg, dbManager, updater != nil)

	// Create and start MCP server (blocks until client disconnects)
	server := mcp.New(cfg, dbManager, updater, iterMgr)
	err = server.Serve()
	cancel() // Always call cancel before exiting
	if err != nil {
		slog.Error("Server error", "err", err)
		return // Let main() exit naturally, defers will run
	}
}

// printHelp displays usage information.
func printHelp() {
	fmt.Printf(`MaxMind MMDB MCP Server %s

A powerful Model Context Protocol (MCP) server that provides comprehensive 
geolocation and network intelligence through MaxMind MMDB databases.

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

	slog.Info("MaxMind MMDB MCP Server starting",
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
