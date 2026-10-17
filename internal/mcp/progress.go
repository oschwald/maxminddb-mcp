package mcp

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// progressReporter is called synchronously while the tool request is active.
func (s *Server) progressReporter(ctx context.Context, request mcp.CallToolRequest) func(string) {
	meta := request.Params.Meta
	if meta == nil {
		return nil
	}
	switch token := meta.ProgressToken.(type) {
	case string, int, int64:
	case float64:
		if math.IsNaN(token) || math.IsInf(token, 0) || math.Trunc(token) != token {
			return nil
		}
	default:
		return nil
	}
	var last time.Time
	var sequence int64
	return func(message string) {
		if ctx.Err() != nil || time.Since(last) < time.Second {
			return
		}
		last = time.Now()
		sequence++
		if err := s.mcp.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": meta.ProgressToken,
			"progress":      sequence,
			"message":       message,
		}); err != nil {
			slog.Debug("Progress notification was not delivered", "err", err)
		}
	}
}
