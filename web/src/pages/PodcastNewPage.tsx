import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getEpisodePreview, submitEpisode } from "../api.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";
import { MicIcon } from "../components/SourceBadge.tsx";
import { formatDuration } from "../utils.ts";

/**
 * PodcastNewPage is the target of the "Summarize in vimmary" button in
 * cast2md. It takes an episode ID, shows what it is about to summarize, and
 * redirects to the summary once one exists — including immediately, when the
 * episode has been summarized before.
 */
export default function PodcastNewPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const episodeID = parseInt(searchParams.get("episode") || "", 10);
  const [level, setLevel] = useState("");

  const { data: preview, isLoading, error } = useQuery({
    queryKey: ["episode-preview", episodeID],
    queryFn: () => getEpisodePreview(episodeID),
    enabled: Number.isFinite(episodeID) && episodeID > 0,
  });

  const submit = useMutation({
    mutationFn: () => submitEpisode(episodeID, level || undefined),
    onSuccess: (video) => {
      queryClient.invalidateQueries({ queryKey: ["podcasts"] });
      navigate(`/podcast/${video.id}`);
    },
  });

  // A second click on the cast2md button lands on the existing summary rather
  // than offering to make another one.
  useEffect(() => {
    if (preview?.existing_id) {
      navigate(`/podcast/${preview.existing_id}`, { replace: true });
    }
  }, [preview?.existing_id, navigate]);

  if (!Number.isFinite(episodeID) || episodeID <= 0) {
    return (
      <div className="vim-page-narrow">
        <Banner tone="err">
          No episode was given. This page is opened from cast2md's "Summarize in
          vimmary" button.
        </Banner>
        <Link to="/podcasts" className="vim-btn ghost" style={{ marginTop: 16 }}>
          ← Back to podcasts
        </Link>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="vim-page-narrow">
        <LoadingSkeleton count={2} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="vim-page-narrow">
        <Banner tone="err">{(error as Error).message}</Banner>
        <Link to="/podcasts" className="vim-btn ghost" style={{ marginTop: 16 }}>
          ← Back to podcasts
        </Link>
      </div>
    );
  }

  if (!preview) return null;

  const notReady = preview.status !== "completed";

  return (
    <div className="vim-page-narrow vim-page-detail">
      <Link
        to="/podcasts"
        className="vim-kicker"
        style={{
          display: "inline-block",
          marginBottom: 28,
          color: "var(--vim-ink-3)",
          textDecoration: "none",
        }}
      >
        ← Back to podcasts
      </Link>

      <div className="vim-kicker" style={{ marginBottom: 14 }}>
        — New podcast summary
      </div>
      <h1 className="vim-h1-detail">{preview.title}</h1>

      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 0,
          fontSize: 13,
          color: "var(--vim-ink-3)",
          marginBottom: 28,
          flexWrap: "wrap",
        }}
      >
        {preview.feed_title && (
          <span style={{ color: "var(--vim-ink-2)" }}>{preview.feed_title}</span>
        )}
        {preview.duration_seconds ? (
          <>
            <span className="vim-dot" />
            <span>{formatDuration(preview.duration_seconds)}</span>
          </>
        ) : null}
        {preview.published_at && (
          <>
            <span className="vim-dot" />
            <span>{preview.published_at.slice(0, 10)}</span>
          </>
        )}
      </div>

      <div className="vim-grid-detail-actions" style={{ marginBottom: 36 }}>
        <a
          href={preview.source_url}
          target="_blank"
          rel="noopener noreferrer"
          className="vim-thumb"
          style={{ aspectRatio: "16 / 9", width: "100%", height: "auto" }}
        >
          {preview.image_url ? (
            <img src={preview.image_url} alt="" />
          ) : (
            <div
              style={{
                width: "100%",
                height: "100%",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                background: "var(--vim-surface-2)",
              }}
            >
              <MicIcon size={40} color="var(--vim-ink-4)" />
            </div>
          )}
        </a>

        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {notReady ? (
            <>
              <Banner tone="warn">
                cast2md has not finished transcribing this episode (status:{" "}
                {preview.status}). Come back once it is done.
              </Banner>
              <a
                href={preview.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className="vim-btn ghost"
                style={{ padding: "11px 16px" }}
              >
                Open in cast2md
              </a>
            </>
          ) : (
            <>
              <label
                style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: -2 }}
                htmlFor="podcast-level"
              >
                Detail level
              </label>
              <select
                id="podcast-level"
                value={level}
                onChange={(e) => setLevel(e.target.value)}
                className="vim-input"
                style={{ fontSize: 13 }}
              >
                <option value="">Default</option>
                <option value="medium">medium — a few paragraphs</option>
                <option value="deep">deep — segment by segment</option>
              </select>
              <button
                onClick={() => submit.mutate()}
                disabled={submit.isPending}
                className="vim-btn primary"
                style={{ padding: "13px 16px" }}
              >
                <MicIcon size={13} color="currentColor" />
                {submit.isPending ? "Starting…" : "Summarize"}
              </button>
              <a
                href={preview.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className="vim-btn ghost"
                style={{ padding: "11px 16px" }}
              >
                Open in cast2md
              </a>
            </>
          )}
          {submit.isError && <Banner tone="err">{(submit.error as Error).message}</Banner>}
        </div>
      </div>

      {preview.description && (
        <section style={{ marginBottom: 36 }}>
          <div className="vim-kicker" style={{ marginBottom: 14 }}>
            — From the feed
          </div>
          <p
            style={{
              fontSize: 14.5,
              lineHeight: 1.6,
              color: "var(--vim-ink-2)",
              margin: 0,
              whiteSpace: "pre-wrap",
            }}
          >
            {preview.description}
          </p>
        </section>
      )}
    </div>
  );
}

function Banner({ tone, children }: { tone: "err" | "warn"; children: React.ReactNode }) {
  const color = tone === "err" ? "var(--vim-err)" : "var(--vim-warn)";
  return (
    <div
      style={{
        padding: "12px 16px",
        borderRadius: "var(--vim-radius)",
        background: `color-mix(in oklch, ${color} 10%, transparent)`,
        border: `1px solid color-mix(in oklch, ${color} 26%, transparent)`,
        color,
        fontSize: 13,
        lineHeight: 1.5,
      }}
    >
      {children}
    </div>
  );
}
