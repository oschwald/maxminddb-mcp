package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

type verificationResult struct {
	Database                   string `json:"database"`
	Valid                      bool   `json:"valid"`
	CompletedAfterCancellation bool   `json:"completed_after_cancellation"`
}

func (s *Server) registerVerificationTool() {
	s.mcp.AddTool(mcp.NewTool(
		"verify_database",
		mcp.WithToolTitle("Verify a database"),
		mcp.WithDescription(
			"Verify the integrity of a loaded database. Only one verification runs at a time. Cancellation can stop a queued request, but verification runs to completion once started.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[verificationResult](),
		mcp.WithString("database", mcp.Required()),
	), s.handleVerifyDatabase)
}

func (s *Server) handleVerifyDatabase(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	name := request.GetString("database", "")
	reader, ok := s.dbManager.GetReader(name)
	if !ok {
		return toolError("db_not_found", "Database not found: "+name), nil
	}
	select {
	case <-ctx.Done():
		return toolError("canceled", "Verification did not start: "+ctx.Err().Error()), nil
	case s.verificationGate <- struct{}{}:
	}
	defer func() { <-s.verificationGate }()
	if err := ctx.Err(); err != nil {
		return toolError("canceled", "Verification did not start: "+err.Error()), nil
	}
	if report := s.progressReporter(ctx, request); report != nil {
		report("Verifying " + name + ". Verification cannot stop once started")
	}
	return verifyDatabase(ctx, name, reader.Verify), nil
}

func verifyDatabase(ctx context.Context, name string, verify func() error) *mcp.CallToolResult {
	// Verify has no context support. Keep the request and gate until it finishes.
	err := verify()
	if err != nil {
		message := "Verification finished with an error: " + err.Error()
		if ctx.Err() != nil {
			message += ". Cancellation was requested while verification ran to completion"
		}
		return toolError("verification_failed", message)
	}
	return mcp.NewToolResultStructuredOnly(verificationResult{
		Database: name, Valid: true, CompletedAfterCancellation: ctx.Err() != nil,
	})
}
