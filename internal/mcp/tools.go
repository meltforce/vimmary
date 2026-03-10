package mcp

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/meltforce/vimmary/internal/storage"
)

func (h *handlers) searchVideos(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := req.GetInt("limit", 0)

	userID := UserIDFromContext(ctx)
	matches, warnings, err := h.svc.Search(ctx, userID, query, limit)
	if err != nil {
		h.log.Error("search failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	result := map[string]any{
		"count":   len(matches),
		"results": matches,
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}

	resp, err := mcp.NewToolResultJSON(result)
	if err != nil {
		return mcp.NewToolResultError("serialization failed"), nil
	}
	return resp, nil
}

func (h *handlers) getVideo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return mcp.NewToolResultError("invalid video ID"), nil
	}

	userID := UserIDFromContext(ctx)
	video, err := h.svc.GetVideo(ctx, userID, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return mcp.NewToolResultError("video not found"), nil
		}
		h.log.Error("get video failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("get video failed: %v", err)), nil
	}

	resp, err := mcp.NewToolResultJSON(video)
	if err != nil {
		return mcp.NewToolResultError("serialization failed"), nil
	}
	return resp, nil
}

func (h *handlers) resummarize(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return mcp.NewToolResultError("invalid video ID"), nil
	}

	level := req.GetString("level", "deep")
	language := req.GetString("language", "")
	provider := req.GetString("provider", "")
	userID := UserIDFromContext(ctx)

	if err := h.svc.Resummarize(ctx, userID, id, level, language, provider); err != nil {
		h.log.Error("resummarize failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("resummarize failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Video resummarized at '%s' detail level.", level)), nil
}

func (h *handlers) listRecent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filters := storage.ListFilters{
		Channel:  req.GetString("channel", ""),
		Language: req.GetString("language", ""),
		Topic:    req.GetString("topic", ""),
	}
	limit := req.GetInt("limit", 0)
	offset := req.GetInt("offset", 0)

	userID := UserIDFromContext(ctx)
	videos, total, err := h.svc.ListRecent(ctx, userID, filters, limit, offset)
	if err != nil {
		h.log.Error("list recent failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("list failed: %v", err)), nil
	}

	resp, err := mcp.NewToolResultJSON(map[string]any{
		"total":  total,
		"count":  len(videos),
		"videos": videos,
	})
	if err != nil {
		return mcp.NewToolResultError("serialization failed"), nil
	}
	return resp, nil
}

func (h *handlers) stats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	userID := UserIDFromContext(ctx)
	stats, err := h.svc.Stats(ctx, userID)
	if err != nil {
		h.log.Error("stats failed", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("stats failed: %v", err)), nil
	}

	resp, err := mcp.NewToolResultJSON(stats)
	if err != nil {
		return mcp.NewToolResultError("serialization failed"), nil
	}
	return resp, nil
}
