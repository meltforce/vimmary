import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { listVideos, searchVideos } from "../api.ts";
import VideoCard from "../components/VideoCard.tsx";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";
import { MicIcon } from "../components/SourceBadge.tsx";

const PAGE_SIZE = 20;

function SearchIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="var(--vim-ink-3)"
      strokeWidth="1.6"
    >
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  );
}

// Episodes arrive from cast2md through subscriptions or a deep link — there is
// no URL to paste here, which is why this page has no submit field.
function EmptyState() {
  return (
    <div
      className="vim-empty"
      style={{
        maxWidth: 640,
        margin: "0 auto",
        padding: "clamp(60px, 14vw, 120px) clamp(16px, 4vw, 40px)",
        textAlign: "center",
      }}
    >
      <div style={{ marginBottom: 32, opacity: 0.6, display: "flex", justifyContent: "center" }}>
        <MicIcon size={48} color="var(--vim-ink-4)" />
      </div>
      <div className="vim-kicker" style={{ marginBottom: 18 }}>
        — Nothing recorded yet
      </div>
      <h1 className="vim-h1-empty">
        No podcast summaries.
        <br />
        <em style={{ color: "var(--vim-accent-ink)", fontStyle: "italic", fontWeight: 400 }}>
          Pick a show to follow.
        </em>
      </h1>
      <p
        style={{
          fontSize: 16,
          lineHeight: 1.6,
          color: "var(--vim-ink-2)",
          margin: "0 auto 28px",
          maxWidth: 480,
        }}
      >
        Subscribe to a feed and every episode cast2md transcribes from then on
        gets summarized here. Individual episodes can be sent over from cast2md
        at any time.
      </p>
      <Link
        to="/settings#podcasts"
        className="vim-btn primary"
        style={{ padding: "10px 18px" }}
      >
        Choose podcasts →
      </Link>
    </div>
  );
}

export default function PodcastListPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get("q") || "";
  const [searchInput, setSearchInput] = useState(query);
  const page = parseInt(searchParams.get("page") || "1", 10);
  const offset = (page - 1) * PAGE_SIZE;

  const searchResult = useQuery({
    queryKey: ["search", "podcast", query],
    queryFn: () => searchVideos(query, undefined, "podcast"),
    enabled: query.length > 0,
  });

  const listResult = useQuery({
    queryKey: ["podcasts", offset],
    queryFn: () => listVideos({ source: "podcast", limit: PAGE_SIZE, offset }),
    enabled: query.length === 0,
    refetchInterval: (q) => {
      const data = q.state.data;
      if (data?.videos.some((v) => v.status === "pending" || v.status === "processing")) {
        return 3000;
      }
      return 10000;
    },
  });

  const isSearching = query.length > 0;
  const isLoading = isSearching ? searchResult.isLoading : listResult.isLoading;
  const errorObj = isSearching ? searchResult.error : listResult.error;

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const t = searchInput.trim();
    if (t) setSearchParams({ q: t });
    else setSearchParams({});
  };

  const total = isSearching
    ? searchResult.data?.results.length ?? 0
    : listResult.data?.total ?? 0;

  const rows = isSearching
    ? searchResult.data?.results.map((m) => ({
        id: m.id,
        youtube_id: m.youtube_id,
        source: m.source,
        title: m.title,
        channel: m.channel,
        summary: m.summary,
        metadata: m.metadata,
        score: m.score,
        match_type: m.match_type,
        created_at: m.created_at,
        thumbnail_url: undefined as string | undefined,
        status: undefined as string | undefined,
        error_message: undefined as string | undefined,
        duration_seconds: undefined as number | undefined,
      }))
    : listResult.data?.videos.map((v) => ({
        id: v.id,
        youtube_id: v.youtube_id,
        source: v.source,
        title: v.title,
        channel: v.channel,
        summary: v.summary,
        metadata: v.metadata,
        score: undefined as number | undefined,
        match_type: undefined as string | undefined,
        created_at: v.created_at,
        thumbnail_url: v.thumbnail_url,
        status: v.status,
        error_message: v.error_message,
        duration_seconds: v.duration_seconds,
      }));

  const isEmpty =
    !isSearching && !isLoading && page === 1 && listResult.data && listResult.data.total === 0;

  if (isEmpty) {
    return (
      <div className="vim-page" style={{ paddingTop: 0, paddingBottom: 0 }}>
        <EmptyState />
      </div>
    );
  }

  return (
    <div className="vim-page">
      {page === 1 && !isSearching && (
        <div style={{ marginBottom: 28 }}>
          <div className="vim-kicker" style={{ marginBottom: 10 }}>
            Your listening list · {total} episode{total === 1 ? "" : "s"}
          </div>
          <h1 className="vim-h1-page">
            Hours of{" "}
            <em style={{ color: "var(--vim-accent-ink)", fontStyle: "italic", fontWeight: 400 }}>
              conversation
            </em>
            ,
            <br />
            turned into something to read.
          </h1>
        </div>
      )}

      {isSearching && (
        <div style={{ marginBottom: 28 }}>
          <div className="vim-kicker" style={{ marginBottom: 10 }}>
            Search · {total} result{total === 1 ? "" : "s"} for "{query}"
          </div>
          <h1 className="vim-h1-page" style={{ fontSize: 36 }}>
            <em style={{ fontStyle: "italic", color: "var(--vim-accent-ink)" }}>{query}</em>
          </h1>
        </div>
      )}

      <form onSubmit={handleSearchSubmit} style={{ position: "relative", marginBottom: 36 }}>
        <span style={{ position: "absolute", left: 14, top: 14, lineHeight: 0 }}>
          <SearchIcon />
        </span>
        <input
          className="vim-input"
          type="text"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search across podcast summaries…"
          style={{ paddingLeft: 40, paddingRight: query ? 84 : 14 }}
        />
        {query && (
          <button
            type="button"
            onClick={() => {
              setSearchInput("");
              setSearchParams({});
            }}
            className="vim-btn ghost"
            style={{ position: "absolute", right: 6, top: 6, padding: "7px 12px", fontSize: 12 }}
          >
            Clear
          </button>
        )}
      </form>

      {errorObj && (
        <div
          style={{
            marginBottom: 16,
            padding: "10px 14px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-err) 10%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-err) 28%, transparent)",
            color: "var(--vim-err)",
            fontSize: 13,
          }}
        >
          {(errorObj as Error).message}
        </div>
      )}

      {isLoading ? (
        <LoadingSkeleton count={3} />
      ) : (
        <div>
          {rows && rows.length === 0 && (
            <p
              style={{
                color: "var(--vim-ink-3)",
                fontSize: 14,
                padding: "48px 0",
                textAlign: "center",
              }}
            >
              {isSearching ? `No results found for "${query}"` : "No episodes yet"}
            </p>
          )}
          {rows?.map((v, i) => (
            <VideoCard
              key={v.id}
              id={v.id}
              youtubeId={v.youtube_id}
              source={v.source}
              thumbnailUrl={v.thumbnail_url}
              title={v.title}
              channel={v.channel}
              durationSeconds={v.duration_seconds}
              summary={v.summary}
              topics={v.metadata?.topics}
              status={v.status}
              errorMessage={v.error_message}
              score={v.score}
              matchType={v.match_type}
              createdAt={v.created_at}
              index={!isSearching ? total - offset - i : undefined}
              isLast={i === rows.length - 1}
            />
          ))}

          {!isSearching && listResult.data && listResult.data.total > PAGE_SIZE && (
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 16,
                paddingTop: 28,
              }}
            >
              <button
                disabled={page <= 1}
                onClick={() => setSearchParams({ page: String(page - 1) })}
                className="vim-btn ghost"
                style={{ padding: "7px 14px", fontSize: 12 }}
              >
                ← Previous
              </button>
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11.5,
                  color: "var(--vim-ink-3)",
                  letterSpacing: "0.04em",
                }}
              >
                Page {page} of {Math.ceil(listResult.data.total / PAGE_SIZE)}
              </span>
              <button
                disabled={offset + PAGE_SIZE >= listResult.data.total}
                onClick={() => setSearchParams({ page: String(page + 1) })}
                className="vim-btn ghost"
                style={{ padding: "7px 14px", fontSize: 12 }}
              >
                Next →
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
