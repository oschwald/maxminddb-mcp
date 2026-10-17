package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oschwald/maxminddb-mcp/internal/config"
	"github.com/oschwald/maxminddb-mcp/internal/database"
)

func (s *Server) registerInstallTools() {
	s.mcp.AddTool(mcp.NewTool(
		"install_database",
		mcp.WithToolTitle("Install a database from a URL"),
		mcp.WithDescription(
			"Download and load a database from an HTTP or HTTPS URL. Accepts MMDB, gzip, tar, tar.gz, or ZIP with exactly one MMDB. Downloads and expanded data are limited to 1 GiB. Uses the configured download folder, or the first folder in directory mode. Does not use MaxMind credentials or save the URL for updates.",
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString(
			"url",
			mcp.Required(),
			mcp.Description(
				"Database download URL. Requests can reach any HTTP(S) host accessible to this server.",
			),
		),
		mcp.WithString(
			"database",
			mcp.Required(),
			mcp.Description("Destination filename ending in .mmdb, without a directory path."),
		),
		mcp.WithBoolean(
			"overwrite",
			mcp.Description("Replace an existing database with this name. Defaults to false."),
		),
		mcp.WithOutputSchema[database.UpdateResult](),
	), s.handleInstallDatabase)
	if s.config.Mode == config.ModeMaxMind || s.config.Mode == config.ModeGeoIPCompat {
		s.mcp.AddTool(mcp.NewTool(
			"install_edition",
			mcp.WithToolTitle("Install a MaxMind edition"),
			mcp.WithDescription(
				"Download a MaxMind MMDB edition using configured credentials, load it immediately, and remember it for future updates. The edition need not be in the server configuration. Replaces an existing file for that edition.",
			),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString(
				"edition",
				mcp.Required(),
				mcp.Description(
					"MaxMind edition ID, for example GeoLite2-Country or GeoIP2-Anonymous-IP. Requires account access to that edition.",
				),
			),
			mcp.WithOutputSchema[database.UpdateResult](),
		), s.handleInstallEdition)
	}
}

func (s *Server) handleInstallDatabase(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	dir := s.config.MaxMind.DatabaseDir
	if s.config.Mode == config.ModeDirectory {
		dir = ""
		if len(s.config.Directory.Paths) > 0 {
			dir = s.config.Directory.Paths[0]
		}
	}
	result := s.dbManager.InstallURL(
		ctx,
		dir,
		request.GetString("database", ""),
		request.GetString("url", ""),
		request.GetBool("overwrite", false),
		s.progressReporter(ctx, request),
	)
	response := mcp.NewToolResultStructuredOnly(result)
	response.IsError = result.Error != ""
	return response, nil
}

func (s *Server) handleInstallEdition(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	if s.updater == nil {
		return toolError(
			"updates_not_available",
			"Configure MaxMind credentials before installing an edition",
		), nil
	}
	result, err := s.updater.InstallEdition(
		ctx,
		request.GetString("edition", ""),
		s.progressReporter(ctx, request),
	)
	if err != nil {
		return toolError("install_failed", err.Error()), nil
	}
	response := mcp.NewToolResultStructuredOnly(result)
	response.IsError = result.Error != ""
	return response, nil
}
