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
	s := mkmcp.NewServer("vimmary", version, `Vimmary is the user's personal YouTube video library with AI-generated summaries.

It contains summaries and transcripts of YouTube videos the user has watched, bookmarked, or saved. Use this whenever the user asks about videos they've seen, topics from YouTube content, or wants to recall something from a video.

ALWAYS use this when the user:
- Asks what they watched, bookmarked, or saved ("what did I watch about X", "videos about X")
- Wants to find or recall a video ("that video about Docker", "the one from Lex Fridman")
- Asks about topics covered in their videos ("what do I know about self-hosting")
- Wants statistics about their viewing habits ("how many videos", "top channels")
- References YouTube content in any way

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
	mcp.WithDescription("Search the user's YouTube video library by topic, keyword, or meaning. Finds videos the user has watched or bookmarked. Use for questions like 'videos about Docker', 'that talk about productivity', 'what did I watch about AI'."),
	mcp.WithString("query",
		mcp.Required(),
		mcp.Description("Natural language search query."),
	),
	mcp.WithNumber("limit",
		mcp.Description("Maximum number of results. Default: 10."),
	),
)

var toolGetVideo = mcp.NewTool("get_video",
	mcp.WithDescription("Get the full summary, transcript, and metadata of a specific video from the user's library. Use after finding a video via search or list to get complete details."),
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
	mcp.WithDescription("Browse the user's recently watched or bookmarked YouTube videos. Use for 'what did I watch recently', 'latest videos', 'show me my videos'. Supports filtering by channel, language, or topic."),
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
	mcp.WithDescription("Get statistics about the user's video library: total count, top channels, top topics, and daily activity. Use for 'how many videos do I have', 'what channels do I watch most', 'my top topics'."),
)

