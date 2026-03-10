package mcp

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/meltforce/vimmary/internal/service"
	mkmcp "github.com/meltforce/meltkit/pkg/mcp"
)

var (
	UserIDFromContext = mkmcp.UserIDFromContext
	WithUserID        = mkmcp.WithUserID
)

func New(svc *service.Service, version string, log *slog.Logger) *server.MCPServer {
	s := mkmcp.NewServer("vimmary", version, `Vimmary is a YouTube video summary service.

Search and browse summaries of bookmarked YouTube videos. Summaries are automatically generated when videos are bookmarked in Karakeep.

Use search_videos for semantic search across all video summaries and transcripts.
Use get_video to retrieve the full summary and transcript for a specific video.
Use list_recent to browse recently processed videos.
Use resummarize to regenerate a summary with a different detail level.
Use stats for aggregate statistics about the video library.`)

	h := &handlers{svc: svc, log: log}

	s.AddTools(
		server.ServerTool{Tool: toolSearchVideos, Handler: h.searchVideos},
		server.ServerTool{Tool: toolGetVideo, Handler: h.getVideo},
		server.ServerTool{Tool: toolResummarize, Handler: h.resummarize},
		server.ServerTool{Tool: toolListRecent, Handler: h.listRecent},
		server.ServerTool{Tool: toolStats, Handler: h.stats},
)

	return s
}

type handlers struct {
	svc *service.Service
	log *slog.Logger
}

var toolSearchVideos = mcp.NewTool("search_videos",
	mcp.WithDescription("Search video summaries and transcripts by semantic similarity. Returns the most relevant videos ranked by a hybrid keyword + semantic score."),
	mcp.WithString("query",
		mcp.Required(),
		mcp.Description("Natural language search query."),
	),
	mcp.WithNumber("limit",
		mcp.Description("Maximum number of results. Default: 10."),
	),
)

var toolGetVideo = mcp.NewTool("get_video",
	mcp.WithDescription("Get a specific video with its full summary, transcript, and metadata."),
	mcp.WithString("id",
		mcp.Required(),
		mcp.Description("UUID of the video."),
	),
)

var toolResummarize = mcp.NewTool("resummarize",
	mcp.WithDescription("Regenerate the summary for a video with a different detail level."),
	mcp.WithString("id",
		mcp.Required(),
		mcp.Description("UUID of the video to resummarize."),
	),
	mcp.WithString("level",
		mcp.Description("Detail level: 'medium' (3-5 paragraphs) or 'deep' (chapter-by-chapter). Default: deep."),
	),
	mcp.WithString("language",
		mcp.Description("Override language for the summary (e.g. 'de', 'en', 'fr'). If omitted, uses the detected transcript language."),
	),
	mcp.WithString("provider",
		mcp.Description("Summarizer provider to use (e.g. 'claude', 'mistral'). If omitted, uses the default provider."),
	),
)

var toolListRecent = mcp.NewTool("list_recent",
	mcp.WithDescription("Browse recently processed videos with optional filters."),
	mcp.WithString("channel",
		mcp.Description("Filter by channel name (partial match)."),
	),
	mcp.WithString("language",
		mcp.Description("Filter by language code (e.g. 'en', 'de')."),
	),
	mcp.WithString("topic",
		mcp.Description("Filter by topic tag."),
	),
	mcp.WithNumber("limit",
		mcp.Description("Number of results per page. Default: 20."),
	),
	mcp.WithNumber("offset",
		mcp.Description("Pagination offset. Default: 0."),
	),
)

var toolStats = mcp.NewTool("stats",
	mcp.WithDescription("Get aggregate statistics: total video count, status distribution, top channels, top topics, and daily activity."),
)

