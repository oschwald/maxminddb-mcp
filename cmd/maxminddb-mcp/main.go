// MaxMind MMDB MCP Server provides Model Context Protocol access to MaxMind databases.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
	"github.com/oschwald/maxminddb-mcp/internal/iterator"
	"github.com/oschwald/maxminddb-mcp/internal/mcp"
)

func main() {
	// Look for config file
	var cfg *config.Config
	var err error

	// Try to load from standard locations
	configPath := findConfigFile()
	if configPath != "" {
		cfg, err = config.LoadConfig(configPath)
		if err != nil {
			log.Fatalf("Failed to load config from %s: %v", configPath, err)
		}
	} else {
		// Use default config
		cfg = config.DefaultConfig()
	}

	// Expand paths
	if err := cfg.ExpandPaths(); err != nil {
		log.Fatalf("Failed to expand paths: %v", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}

	// Create database manager
	dbManager, err := database.New()
	if err != nil {
		log.Fatalf("Failed to create database manager: %v", err)
	}

	// Initialize databases based on mode
	if err := initializeDatabases(cfg, dbManager); err != nil {
		log.Fatalf("Failed to initialize databases: %v", err)
	}

	// Create updater if needed
	var updater *database.Updater
	if cfg.Mode == "maxmind" || cfg.Mode == "geoip_compat" {
		updater, err = database.NewUpdater(cfg, dbManager)
		if err != nil {
			log.Printf("Warning: Failed to create updater: %v", err)
		}
	}

	// Create iterator manager
	iterMgr := iterator.New(cfg.IteratorTTLDuration, cfg.IteratorCleanupIntervalDuration)
	iterMgr.StartCleanup()
	defer iterMgr.StopCleanup()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		log.Println("Shutting down...")
		cancel()
	}()

	// Create and start MCP server (blocks until client disconnects)
	server := mcp.New(cfg, dbManager, updater, iterMgr)
	err = server.Serve()
	cancel() // Always call cancel before exiting
	if err != nil {
		log.Printf("Server error: %v", err)
		return // Let main() exit naturally, defers will run
	}
}

func findConfigFile() string {
	// Check environment variable first (highest precedence)
	if envConfig := os.Getenv("MAXMINDDB_MCP_CONFIG"); envConfig != "" {
		if _, err := os.Stat(envConfig); err == nil {
			return envConfig
		}
		log.Printf(
			"Warning: Config file specified in MAXMINDDB_MCP_CONFIG not found: %s",
			envConfig,
		)
	}

	// Standard config locations
	locations := []string{
		"./maxminddb-mcp.toml",
		"./config.toml",
	}

	// Add user config directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		userConfig := filepath.Join(homeDir, ".config", "maxminddb-mcp", "config.toml")
		locations = append(locations, userConfig)
	}

	// Add system config
	locations = append(locations, "/etc/maxminddb-mcp/config.toml")

	for _, path := range locations {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func initializeDatabases(cfg *config.Config, dbManager *database.Manager) error {
	switch cfg.Mode {
	case "maxmind", "geoip_compat":
		// Ensure database directory exists
		if err := os.MkdirAll(cfg.MaxMind.DatabaseDir, 0o750); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		// Load existing databases and watch directory
		if err := dbManager.LoadDirectory(cfg.MaxMind.DatabaseDir); err != nil {
			return fmt.Errorf("failed to load databases: %w", err)
		}
		return dbManager.WatchDirectory(cfg.MaxMind.DatabaseDir)

	case "directory":
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
