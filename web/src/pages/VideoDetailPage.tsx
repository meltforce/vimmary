import { useState, useEffect } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import {
  getVideo,
  resummarizeVideo,
  deleteVideo,
  fetchProviders,
  fetchKarakeepStatus,
} from "../api.ts";
import { formatDuration, formatTokens, videoToMarkdown } from "../utils.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

function formatDate(iso?: string): string {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function parseChapter(raw: string, index: number): { ts: string; body: string } {
  const m = raw.match(/^\s*(\d{1,2}:\d{2}(?::\d{2})?)\s*[—–-]?\s*(.+)/s);
  if (m) {
    return { ts: m[1], body: m[2].trim() };
  }
  return { ts: String(index + 1).padStart(2, "0"), body: raw.trim() };
}

function wordCount(s: string): number {
  return s.trim() ? s.trim().split(/\s+/).length : 0;
}

export default function VideoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [showTranscript, setShowTranscript] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [resumLang, setResumLang] = useState("");
  const [resumProvider, setResumProvider] = useState("");
  const [copiedMd, setCopiedMd] = useState(false);

  const { data: providers } = useQuery({
    queryKey: ["providers"],
    queryFn: fetchProviders,
  });

  const { data: karakeepStatus } = useQuery({
    queryKey: ["settings", "karakeep"],
    queryFn: fetchKarakeepStatus,
  });

  const {
    data: video,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["video", id],
    queryFn: () => getVideo(id!),
    enabled: !!id,
    refetchInterval: (q) => {
      const v = q.state.data;
      if (v && (v.status === "pending" || v.status === "processing")) return 2000;
      return false;
    },
  });

  useEffect(() => {
    if (video?.title) document.title = video.title;
    return () => {
      document.title = "Vimmary";
    };
  }, [video?.title]);

  const resummarize = useMutation({
    mutationFn: (level: string) =>
      resummarizeVideo(id!, level, resumLang || undefined, resumProvider || undefined),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["video", id] }),
  });

  const del = useMutation({
    mutationFn: () => deleteVideo(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      navigate("/");
    },
  });

  if (isLoading)
    return (
      <div className="vim-page-narrow">
        <LoadingSkeleton count={2} />
      </div>
    );

  if (error)
    return (
      <div className="vim-page-narrow">
        <div
          style={{
            padding: "12px 16px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-err) 10%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-err) 28%, transparent)",
            color: "var(--vim-err)",
            fontSize: 13,
          }}
        >
          {(error as Error).message}
        </div>
      </div>
    );

  if (!video) return null;

  const thumbnail = `https://img.youtube.com/vi/${video.youtube_id}/mqdefault.jpg`;
  const youtubeUrl = `https://youtube.com/watch?v=${video.youtube_id}`;
  const isProcessing = video.status === "pending" || video.status === "processing";
  const isFailed = video.status === "failed";

  const handleCopy = () => {
    navigator.clipboard.writeText(videoToMarkdown(video));
    setCopiedMd(true);
    setTimeout(() => setCopiedMd(false), 2000);
  };

  const summaryFirstLetter = video.summary?.trim().match(/^[A-Za-zÀ-ÿ]/)?.[0] ?? "";
  const summaryRest = video.summary?.trim().replace(/^[A-Za-zÀ-ÿ]/, "") ?? "";

  return (
    <div className="vim-page-narrow vim-page-detail">
      <Link
        to="/"
        className="vim-kicker"
        style={{
          display: "inline-block",
          marginBottom: 28,
          color: "var(--vim-ink-3)",
          textDecoration: "none",
        }}
      >
        ← Back to videos
      </Link>

      {/* Header */}
      <div className="vim-kicker" style={{ marginBottom: 14 }}>
        Summary · {video.detail_level} · {formatDate(video.created_at)}
      </div>
      <h1 className="vim-h1-detail">{video.title || video.youtube_id}</h1>

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
        {video.channel && <span style={{ color: "var(--vim-ink-2)" }}>{video.channel}</span>}
        {video.duration_seconds ? (
          <>
            <span className="vim-dot" />
            <span>{formatDuration(video.duration_seconds)}</span>
          </>
        ) : null}
        {video.language && (
          <>
            <span className="vim-dot" />
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>
              {video.language.toUpperCase()}
            </span>
          </>
        )}
        {!isProcessing && !isFailed && video.summary && (
          <>
            <span className="vim-dot" />
            <span className="vim-status done">summarized</span>
          </>
        )}
        {isProcessing && (
          <>
            <span className="vim-dot" />
            <span className="vim-status proc">
              <span className="pulse" />
              {video.status === "pending" ? "queued" : "transcribing"}
            </span>
          </>
        )}
        {isFailed && (
          <>
            <span className="vim-dot" />
            <span className="vim-status fail">failed</span>
          </>
        )}
      </div>

      {/* Thumbnail + actions */}
      <div className="vim-grid-detail-actions" style={{ marginBottom: 44 }}>
        <a
          href={youtubeUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="vim-thumb"
          style={{ aspectRatio: "16 / 9", width: "100%", height: "auto" }}
        >
          <img src={thumbnail} alt="" />
          {video.duration_seconds ? (
            <span className="dur">{formatDuration(video.duration_seconds)}</span>
          ) : null}
          <div className="play" style={{ opacity: 1 }}>
            <svg width="14" height="14" viewBox="0 0 14 14" fill="#fff">
              <path d="M3 1.5v11L13 7z" />
            </svg>
          </div>
        </a>

        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <a
            href={youtubeUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="vim-btn primary"
            style={{ padding: "13px 16px" }}
          >
            <svg width="13" height="13" viewBox="0 0 14 14" fill="currentColor">
              <path d="M3 1.5v11L13 7z" />
            </svg>
            Watch on YouTube
          </a>
          {video.karakeep_bookmark_id && karakeepStatus?.base_url && (
            <a
              href={`${karakeepStatus.base_url}/dashboard/preview/${video.karakeep_bookmark_id}`}
              target="_blank"
              rel="noopener noreferrer"
              className="vim-btn ghost"
              style={{ padding: "11px 16px" }}
            >
              Open in Karakeep
            </a>
          )}
          {video.summary && (
            <button
              onClick={handleCopy}
              className="vim-btn outline"
              style={{ padding: "11px 16px" }}
            >
              {copiedMd ? "Copied ✓" : "Copy Markdown"}
            </button>
          )}
          <div
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 10.5,
              color: "var(--vim-ink-4)",
              marginTop: 4,
              letterSpacing: "0.06em",
              lineHeight: 1.5,
            }}
          >
            {video.summary_provider && (
              <>
                {video.summary_provider}
                {video.summary_model && ` · ${video.summary_model}`}
                <br />
              </>
            )}
            {(video.summary_input_tokens ?? 0) > 0 && (
              <>
                {formatTokens(video.summary_input_tokens!)} in ·{" "}
                {formatTokens(video.summary_output_tokens!)} out
              </>
            )}
          </div>
        </div>
      </div>

      {/* Processing indicator */}
      {isProcessing && (
        <div
          style={{
            marginBottom: 32,
            padding: "12px 16px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-warn) 8%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-warn) 22%, transparent)",
            color: "var(--vim-warn)",
            fontSize: 13,
          }}
        >
          Video is being processed. This page will update automatically.
        </div>
      )}

      {isFailed && video.error_message && (
        <div
          style={{
            marginBottom: 32,
            padding: "12px 16px",
            borderRadius: "var(--vim-radius)",
            background: "color-mix(in oklch, var(--vim-err) 10%, transparent)",
            border: "1px solid color-mix(in oklch, var(--vim-err) 28%, transparent)",
            color: "var(--vim-err)",
            fontSize: 13,
          }}
        >
          {video.error_message}
        </div>
      )}

      {/* Drop-cap summary */}
      {video.summary && (
        <section style={{ marginBottom: 48 }}>
          <div className="vim-kicker" style={{ marginBottom: 18 }}>
            — The summary
          </div>
          <div
            style={{
              fontFamily: "var(--font-serif)",
              fontSize: 19,
              lineHeight: 1.55,
              color: "var(--vim-ink)",
              fontWeight: 300,
              maxWidth: 640,
            }}
            className="vim-md"
          >
            {summaryFirstLetter && (
              <span className="vim-dropcap-letter">{summaryFirstLetter}</span>
            )}
            <ReactMarkdown
              components={{
                p: ({ children }) => (
                  <p style={{ margin: 0, marginBottom: "0.7em" }}>{children}</p>
                ),
              }}
            >
              {summaryRest}
            </ReactMarkdown>
          </div>
          <div style={{ clear: "both" }} />
        </section>
      )}

      {/* Chapters */}
      {video.metadata?.key_points && video.metadata.key_points.length > 0 && (
        <section style={{ marginBottom: 48 }}>
          <div
            style={{
              display: "flex",
              alignItems: "baseline",
              justifyContent: "space-between",
              marginBottom: 16,
            }}
          >
            <div className="vim-kicker">
              — Chapters · {video.metadata.key_points.length} key point
              {video.metadata.key_points.length === 1 ? "" : "s"}
            </div>
          </div>
          <div
            style={{
              border: "1px solid var(--vim-line-soft)",
              borderRadius: 12,
              overflow: "hidden",
              background: "var(--vim-surface)",
            }}
          >
            {video.metadata.key_points.map((kp, i) => {
              const { ts, body } = parseChapter(kp, i);
              return (
                <div key={i} className="vim-chapter">
                  <span className="ts">{ts}</span>
                  <span className="body vim-md">
                    <ReactMarkdown>{body}</ReactMarkdown>
                  </span>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {/* Action Items */}
      {video.metadata?.action_items && video.metadata.action_items.length > 0 && (
        <section style={{ marginBottom: 48 }}>
          <div className="vim-kicker" style={{ marginBottom: 16 }}>
            — Things to try
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {video.metadata.action_items.map((ai, i) => (
              <div
                key={i}
                style={{
                  display: "grid",
                  gridTemplateColumns: "28px 1fr",
                  gap: 12,
                  padding: "14px 18px",
                  background: "var(--vim-surface)",
                  border: "1px solid var(--vim-line-soft)",
                  borderRadius: 8,
                }}
              >
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 11,
                    color: "var(--vim-accent-ink)",
                    paddingTop: 3,
                    letterSpacing: "0.04em",
                  }}
                >
                  {String(i + 1).padStart(2, "0")}
                </span>
                <span
                  className="vim-md"
                  style={{ fontSize: 14.5, lineHeight: 1.5, color: "var(--vim-ink)" }}
                >
                  <ReactMarkdown>{ai}</ReactMarkdown>
                </span>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Topics */}
      {video.metadata?.topics && video.metadata.topics.length > 0 && (
        <section style={{ marginBottom: 36 }}>
          <div className="vim-kicker" style={{ marginBottom: 12 }}>
            — Filed under
          </div>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {video.metadata.topics.map((t) => (
              <span key={t} className="vim-tag dot">
                {t}
              </span>
            ))}
          </div>
        </section>
      )}

      {/* Transcript toggle */}
      {video.transcript && (
        <div style={{ marginBottom: 32 }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              padding: "18px 20px",
              border: "1px dashed var(--vim-line)",
              borderRadius: 10,
              color: "var(--vim-ink-3)",
              fontSize: 13,
            }}
          >
            <span>
              Transcript ·{" "}
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>
                {wordCount(video.transcript).toLocaleString()} words
              </span>
            </span>
            <button
              onClick={() => setShowTranscript(!showTranscript)}
              className="vim-btn ghost"
              style={{ padding: "7px 14px" }}
            >
              {showTranscript ? "Hide transcript ↑" : "Show transcript ↓"}
            </button>
          </div>
          {showTranscript && (
            <div
              style={{
                marginTop: 8,
                padding: 20,
                border: "1px solid var(--vim-line-soft)",
                borderRadius: 10,
                background: "var(--vim-surface)",
                fontSize: 13.5,
                lineHeight: 1.65,
                color: "var(--vim-ink-2)",
                whiteSpace: "pre-wrap",
                maxHeight: 480,
                overflowY: "auto",
              }}
            >
              {video.transcript}
            </div>
          )}
        </div>
      )}

      {/* Actions */}
      <div
        style={{
          border: "1px solid var(--vim-line-soft)",
          borderRadius: 12,
          background: "var(--vim-surface)",
          marginBottom: 24,
        }}
      >
        <button
          onClick={() => setShowActions(!showActions)}
          className="vim-btn"
          style={{
            width: "100%",
            justifyContent: "space-between",
            padding: "14px 18px",
            background: "transparent",
            color: "var(--vim-ink-2)",
            fontSize: 13,
          }}
        >
          <span>Actions</span>
          <span style={{ color: "var(--vim-ink-4)", fontFamily: "var(--font-mono)", fontSize: 11 }}>
            {showActions ? "Hide ↑" : "Show ↓"}
          </span>
        </button>
        {showActions && (
          <div style={{ padding: "0 18px 18px", display: "flex", flexDirection: "column", gap: 16 }}>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                flexWrap: "wrap",
                fontSize: 13,
                color: "var(--vim-ink-3)",
              }}
            >
              <span>Resummarize:</span>
              {providers && providers.providers.length > 1 && (
                <select
                  value={resumProvider}
                  onChange={(e) => setResumProvider(e.target.value)}
                  className="vim-input"
                  style={{ width: "auto", padding: "7px 10px", fontSize: 12 }}
                >
                  <option value="">Default ({providers.default})</option>
                  {providers.providers.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              )}
              <select
                value={resumLang}
                onChange={(e) => setResumLang(e.target.value)}
                className="vim-input"
                style={{ width: "auto", padding: "7px 10px", fontSize: 12 }}
              >
                <option value="">Auto ({video.language || "?"})</option>
                <option value="de">Deutsch</option>
                <option value="en">English</option>
                <option value="fr">Français</option>
                <option value="es">Español</option>
              </select>
              {["medium", "deep"].map((level) => (
                <button
                  key={level}
                  disabled={resummarize.isPending}
                  onClick={() => resummarize.mutate(level)}
                  className="vim-btn ghost"
                  style={{ padding: "7px 14px", fontSize: 12 }}
                >
                  {level}
                </button>
              ))}
              {resummarize.isPending && (
                <span style={{ color: "var(--vim-ink-3)" }}>Processing…</span>
              )}
              {resummarize.isSuccess && (
                <span style={{ color: "var(--vim-ok)" }}>Done.</span>
              )}
              {resummarize.isError && (
                <span style={{ color: "var(--vim-err)" }}>
                  {(resummarize.error as Error).message}
                </span>
              )}
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              {!confirmDelete ? (
                <button
                  onClick={() => setConfirmDelete(true)}
                  className="vim-btn outline danger"
                  style={{ padding: "7px 14px", fontSize: 12 }}
                >
                  Delete video
                </button>
              ) : (
                <>
                  <span style={{ color: "var(--vim-err)", fontSize: 13 }}>Are you sure?</span>
                  <button
                    onClick={() => del.mutate()}
                    disabled={del.isPending}
                    className="vim-btn primary"
                    style={{ padding: "7px 14px", fontSize: 12 }}
                  >
                    {del.isPending ? "Deleting…" : "Yes, delete"}
                  </button>
                  <button
                    onClick={() => setConfirmDelete(false)}
                    className="vim-btn ghost"
                    style={{ padding: "7px 14px", fontSize: 12 }}
                  >
                    Cancel
                  </button>
                </>
              )}
              {del.isError && (
                <span style={{ color: "var(--vim-err)", fontSize: 13 }}>
                  {(del.error as Error).message}
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
