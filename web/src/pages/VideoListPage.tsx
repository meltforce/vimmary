import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  listVideos,
  searchVideos,
  submitVideo,
  retryAllFailed,
  transcribeAllNoCaptions,
  fetchStats,
} from "../api.ts";
import VideoCard from "../components/VideoCard.tsx";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";
import { formatDuration, stripMarkdown } from "../utils.ts";
import { Link } from "react-router-dom";

const PAGE_SIZE = 20;

function formatDate(iso?: string): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function PlayIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="var(--vim-ink-3)"
      strokeWidth="1.6"
    >
      <rect x="3" y="6" width="18" height="12" rx="3" />
      <path d="M10 9v6l5-3z" fill="var(--vim-ink-3)" />
    </svg>
  );
}

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

function EmptyState({
  onSubmit,
  pending,
  error,
}: {
  onSubmit: (url: string) => void;
  pending: boolean;
  error?: string;
}) {
  const [url, setUrl] = useState("");
  return (
    <div
      className="vim-empty"
      style={{
        maxWidth: 720,
        margin: "0 auto",
        padding: "clamp(60px, 14vw, 120px) clamp(16px, 4vw, 40px)",
        textAlign: "center",
      }}
    >
      {/* Tape-frame motif */}
      <div
        style={{
          display: "flex",
          justifyContent: "center",
          gap: 8,
          marginBottom: 40,
          opacity: 0.7,
        }}
      >
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            style={{
              width: 52,
              height: 38,
              borderRadius: 3,
              background:
                "linear-gradient(135deg, var(--vim-surface-2) 0%, var(--vim-surface) 100%)",
              border: "1px solid var(--vim-line)",
              position: "relative",
              transform: `rotate(${(i - 1) * 4}deg) translateY(${Math.abs(i - 1) * 2}px)`,
            }}
          >
            <div
              style={{
                position: "absolute",
                inset: 4,
                borderRadius: 2,
                background: "var(--vim-bg)",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <div
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  background: i === 1 ? "var(--vim-accent)" : "var(--vim-ink-4)",
                }}
              />
            </div>
          </div>
        ))}
      </div>

      <div className="vim-kicker" style={{ marginBottom: 18 }}>
        — A quiet beginning
      </div>
      <h1 className="vim-h1-empty">
        Nothing here yet.
        <br />
        <em
          style={{
            color: "var(--vim-accent-ink)",
            fontStyle: "italic",
            fontWeight: 400,
          }}
        >
          Paste a link to begin.
        </em>
      </h1>
      <p
        style={{
          fontSize: 16,
          lineHeight: 1.6,
          color: "var(--vim-ink-2)",
          margin: "0 auto 32px",
          maxWidth: 480,
        }}
      >
        Drop in any YouTube URL and we'll turn the transcript into a short,
        readable summary. Karakeep webhooks and bulk import live in Settings.
      </p>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const t = url.trim();
          if (t) onSubmit(t);
        }}
        style={{ position: "relative", maxWidth: 520, margin: "0 auto 24px" }}
      >
        <input
          className="vim-input"
          placeholder="https://youtube.com/watch?v=…"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          style={{ paddingLeft: 18, paddingRight: 110, textAlign: "center", fontSize: 14 }}
        />
        <button
          type="submit"
          disabled={pending || !url.trim()}
          className="vim-btn primary"
          style={{ position: "absolute", right: 6, top: 6, padding: "8px 16px" }}
        >
          {pending ? "Adding…" : "Add →"}
        </button>
      </form>

      {error && (
        <p style={{ fontSize: 12.5, color: "var(--vim-err)", marginBottom: 12 }}>
          {error}
        </p>
      )}

      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          gap: 8,
          color: "var(--vim-ink-3)",
          fontSize: 12,
        }}
      >
        <span>Or wire it up to</span>
        <Link
          to="/settings"
          style={{
            fontFamily: "var(--font-mono)",
            color: "var(--vim-accent-ink)",
            fontSize: 11.5,
            letterSpacing: "0.04em",
            textDecoration: "underline",
            textUnderlineOffset: 3,
            textDecorationColor: "var(--vim-line)",
          }}
        >
          Karakeep webhooks →
        </Link>
      </div>
    </div>
  );
}

export default function VideoListPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get("q") || "";
  const [searchInput, setSearchInput] = useState(query);
  const [youtubeUrl, setYoutubeUrl] = useState("");
  const page = parseInt(searchParams.get("page") || "1", 10);
  const offset = (page - 1) * PAGE_SIZE;

  const searchResult = useQuery({
    queryKey: ["search", query],
    queryFn: () => searchVideos(query),
    enabled: query.length > 0,
  });

  const listResult = useQuery({
    queryKey: ["videos", offset],
    queryFn: () => listVideos({ limit: PAGE_SIZE, offset }),
    enabled: query.length === 0,
    refetchInterval: (q) => {
      const data = q.state.data;
      if (data?.videos.some((v) => v.status === "pending" || v.status === "processing")) {
        return 3000;
      }
      return 10000;
    },
  });

  const submit = useMutation({
    mutationFn: (url: string) => submitVideo(url),
    onSuccess: () => {
      setYoutubeUrl("");
      queryClient.invalidateQueries({ queryKey: ["videos"] });
    },
  });

  const retryAll = useMutation({
    mutationFn: () => retryAllFailed(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
    },
  });

  const transcribeAll = useMutation({
    mutationFn: () => transcribeAllNoCaptions(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
    },
  });

  const statsResult = useQuery({
    queryKey: ["stats"],
    queryFn: () => fetchStats(),
    enabled: query.length === 0,
    refetchInterval: 10000,
  });

  const isSearching = query.length > 0;
  const isLoading = isSearching ? searchResult.isLoading : listResult.isLoading;
  const errorObj = isSearching ? searchResult.error : listResult.error;
  const failedCount = statsResult.data?.by_status?.failed ?? 0;
  const noCaptionsCount = statsResult.data?.by_status?.no_captions ?? 0;

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const t = searchInput.trim();
    if (t) setSearchParams({ q: t });
    else setSearchParams({});
  };

  const handleAddSubmit = (urlOrEvent: string | React.FormEvent) => {
    if (typeof urlOrEvent === "string") {
      submit.mutate(urlOrEvent);
      return;
    }
    urlOrEvent.preventDefault();
    const t = youtubeUrl.trim();
    if (t) submit.mutate(t);
  };

  type Row = {
    id: string;
    youtube_id: string;
    title: string;
    channel: string;
    summary?: string;
    metadata?: { topics?: string[]; key_points?: string[]; action_items?: string[] };
    created_at: string;
    status?: string;
    error_message?: string;
    duration_seconds?: number;
    language?: string;
    score?: number;
    match_type?: string;
  };

  const total = isSearching
    ? searchResult.data?.results.length ?? 0
    : listResult.data?.total ?? 0;
  const videos: Row[] | undefined = isSearching
    ? searchResult.data?.results.map((m) => ({
        id: m.id,
        youtube_id: m.youtube_id,
        title: m.title,
        channel: m.channel,
        summary: m.summary,
        metadata: m.metadata,
        score: m.score,
        match_type: m.match_type,
        created_at: m.created_at,
      }))
    : listResult.data?.videos;

  const onFirstPage = page === 1 && !isSearching;
  const totalForIntro = statsResult.data?.total_count ?? listResult.data?.total ?? 0;
  const isEmpty =
    !isSearching &&
    !isLoading &&
    page === 1 &&
    listResult.data &&
    listResult.data.total === 0;

  if (isEmpty) {
    return (
      <div className="vim-page" style={{ paddingTop: 0, paddingBottom: 0 }}>
        <EmptyState
          onSubmit={(u) => submit.mutate(u)}
          pending={submit.isPending}
          error={submit.isError ? (submit.error as Error).message : undefined}
        />
      </div>
    );
  }

  // Hero is the newest summarized video (only on first page, only when listing).
  const heroIdx =
    onFirstPage && videos ? videos.findIndex((v) => v.summary && v.status !== "failed") : -1;
  const heroVideo = heroIdx >= 0 ? videos![heroIdx] : null;
  const restVideos = heroVideo
    ? videos!.filter((_, i) => i !== heroIdx)
    : videos;

  return (
    <div className="vim-page">
      {/* Editorial header */}
      {onFirstPage && (
        <div style={{ marginBottom: 28 }}>
          <div className="vim-kicker" style={{ marginBottom: 10 }}>
            Your reading list · {totalForIntro} video{totalForIntro === 1 ? "" : "s"}
          </div>
          <h1 className="vim-h1-page">
            Everything you've{" "}
            <em
              style={{
                color: "var(--vim-accent-ink)",
                fontStyle: "italic",
                fontWeight: 400,
              }}
            >
              queued
            </em>{" "}
            to watch,
            <br />
            turned into something to read.
          </h1>
        </div>
      )}

      {isSearching && (
        <div style={{ marginBottom: 28 }}>
          <div className="vim-kicker" style={{ marginBottom: 10 }}>
            Search · {searchResult.data?.results.length ?? 0} result
            {(searchResult.data?.results.length ?? 0) === 1 ? "" : "s"} for "{query}"
          </div>
          <h1 className="vim-h1-page" style={{ fontSize: 36 }}>
            <em style={{ fontStyle: "italic", color: "var(--vim-accent-ink)" }}>
              {query}
            </em>
          </h1>
        </div>
      )}

      {/* Dual input row */}
      <div className="vim-grid-input-row" style={{ marginBottom: 36 }}>
        <form onSubmit={handleAddSubmit} style={{ position: "relative" }}>
          <span style={{ position: "absolute", left: 16, top: 14, lineHeight: 0 }}>
            <PlayIcon />
          </span>
          <input
            className="vim-input"
            type="text"
            value={youtubeUrl}
            onChange={(e) => setYoutubeUrl(e.target.value)}
            placeholder="Paste a YouTube URL to summarize…"
            style={{ paddingLeft: 44, paddingRight: 110 }}
          />
          <button
            type="submit"
            disabled={submit.isPending || !youtubeUrl.trim()}
            className="vim-btn primary"
            style={{ position: "absolute", right: 6, top: 6, padding: "7px 14px" }}
          >
            {submit.isPending ? "Adding…" : "Add"}
          </button>
        </form>

        <form onSubmit={handleSearchSubmit} style={{ position: "relative" }}>
          <span style={{ position: "absolute", left: 14, top: 14, lineHeight: 0 }}>
            <SearchIcon />
          </span>
          <input
            className="vim-input"
            type="text"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Search across all summaries…"
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
      </div>

      {/* Status messages */}
      {submit.isSuccess && (
        <div
          style={{
            marginBottom: 16,
            padding: "10px 14px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-ok) 8%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-ok) 24%, transparent)",
            color: "var(--vim-ok)",
            fontSize: 13,
          }}
        >
          Video submitted for processing. It will appear shortly.
        </div>
      )}
      {submit.isError && (
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
          {(submit.error as Error).message}
        </div>
      )}

      {/* Failed banner */}
      {!isSearching && failedCount > 0 && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 16,
            padding: "12px 16px",
            marginBottom: 16,
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-err) 10%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-err) 28%, transparent)",
          }}
        >
          <span style={{ color: "var(--vim-err)", fontSize: 13 }}>
            {failedCount} video{failedCount !== 1 ? "s" : ""} failed
          </span>
          <button
            onClick={() => retryAll.mutate()}
            disabled={retryAll.isPending}
            className="vim-btn ghost"
            style={{ padding: "6px 12px", fontSize: 12 }}
          >
            {retryAll.isPending ? "Retrying…" : "Retry all"}
          </button>
        </div>
      )}
      {retryAll.isSuccess && (
        <div
          style={{
            marginBottom: 16,
            padding: "10px 14px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-ok) 8%, transparent)",
            color: "var(--vim-ok)",
            fontSize: 13,
          }}
        >
          {retryAll.data.retried} video{retryAll.data.retried !== 1 ? "s" : ""} queued for retry.
        </div>
      )}

      {/* No-captions banner */}
      {!isSearching && noCaptionsCount > 0 && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 16,
            padding: "12px 16px",
            marginBottom: 16,
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-warn) 8%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-warn) 22%, transparent)",
          }}
        >
          <span style={{ color: "var(--vim-warn)", fontSize: 13 }}>
            {noCaptionsCount} video{noCaptionsCount !== 1 ? "s" : ""} with no captions
          </span>
          <button
            onClick={() => transcribeAll.mutate()}
            disabled={transcribeAll.isPending}
            className="vim-btn ghost"
            style={{ padding: "6px 12px", fontSize: 12 }}
          >
            {transcribeAll.isPending ? "Transcribing…" : "Transcribe all with Voxtral"}
          </button>
        </div>
      )}
      {transcribeAll.isSuccess && (
        <div
          style={{
            marginBottom: 16,
            padding: "10px 14px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-ok) 8%, transparent)",
            color: "var(--vim-ok)",
            fontSize: 13,
          }}
        >
          {transcribeAll.data.transcribing} video
          {transcribeAll.data.transcribing !== 1 ? "s" : ""} queued for Voxtral transcription.
        </div>
      )}

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

      {searchResult.data?.warnings?.map((w, i) => (
        <div
          key={i}
          style={{
            marginBottom: 16,
            padding: "10px 14px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-warn) 8%, transparent)",
            color: "var(--vim-warn)",
            fontSize: 13,
          }}
        >
          {w}
        </div>
      ))}

      {/* Hero card */}
      {heroVideo && (
        <Link
          to={`/video/${heroVideo.id}`}
          style={{ display: "block", color: "inherit", textDecoration: "none", marginBottom: 40 }}
        >
          <article className="vim-card">
            <div className="vim-kicker" style={{ marginBottom: 14 }}>
              — Latest summary · {formatDate(heroVideo.created_at)}
            </div>
            <div className="vim-grid-hero">
              <div className="vim-thumb vim-thumb-hero">
                <img
                  src={`https://img.youtube.com/vi/${heroVideo.youtube_id}/mqdefault.jpg`}
                  alt=""
                />
                {heroVideo.duration_seconds ? (
                  <span className="dur">{formatDuration(heroVideo.duration_seconds)}</span>
                ) : null}
                <div className="play">
                  <svg width="14" height="14" viewBox="0 0 14 14" fill="#fff">
                    <path d="M3 1.5v11L13 7z" />
                  </svg>
                </div>
              </div>
              <div>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--vim-ink-3)",
                    marginBottom: 8,
                    display: "flex",
                    alignItems: "center",
                    flexWrap: "wrap",
                  }}
                >
                  {heroVideo.channel && (
                    <span style={{ color: "var(--vim-ink-2)" }}>{heroVideo.channel}</span>
                  )}
                  {heroVideo.duration_seconds ? (
                    <>
                      <span className="vim-dot" />
                      <span>{formatDuration(heroVideo.duration_seconds)}</span>
                    </>
                  ) : null}
                  {heroVideo.language && (
                    <>
                      <span className="vim-dot" />
                      <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5 }}>
                        {heroVideo.language.toUpperCase()}
                      </span>
                    </>
                  )}
                </div>
                <h2 className="vim-h2-hero">
                  {heroVideo.title || heroVideo.youtube_id}
                </h2>
                <p
                  style={{
                    fontSize: 15,
                    lineHeight: 1.6,
                    color: "var(--vim-ink-2)",
                    margin: "0 0 18px",
                    maxWidth: 580,
                    display: "-webkit-box",
                    WebkitLineClamp: 4,
                    WebkitBoxOrient: "vertical",
                    overflow: "hidden",
                  }}
                >
                  {stripMarkdown(heroVideo.summary ?? "")}
                </p>
                <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                  {(heroVideo.metadata?.topics ?? []).slice(0, 5).map((t) => (
                    <span key={t} className="vim-tag">
                      {t}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          </article>
        </Link>
      )}

      {heroVideo && <hr className="vim-hr" style={{ marginBottom: 8 }} />}

      {/* List */}
      {isLoading ? (
        <LoadingSkeleton count={3} />
      ) : (
        <div>
          {restVideos && restVideos.length === 0 && !heroVideo && (
            <p
              style={{
                color: "var(--vim-ink-3)",
                fontSize: 14,
                padding: "48px 0",
                textAlign: "center",
              }}
            >
              {isSearching ? `No results found for "${query}"` : "No videos yet"}
            </p>
          )}
          {restVideos?.map((v, i) => (
            <VideoCard
              key={v.id}
              id={v.id}
              youtubeId={v.youtube_id}
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
              index={!isSearching ? total - offset - (heroVideo ? 1 : 0) - i : undefined}
              isLast={i === (restVideos.length - 1)}
            />
          ))}

          {/* Pagination */}
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
