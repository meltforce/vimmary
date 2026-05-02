import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteVideo, retryVideo, transcribeVideo } from "../api.ts";
import { formatDuration, stripMarkdown } from "../utils.ts";

interface Props {
  id: string;
  youtubeId: string;
  title: string;
  channel: string;
  durationSeconds?: number;
  summary?: string;
  topics?: string[];
  status?: string;
  errorMessage?: string;
  score?: number;
  matchType?: string;
  index?: number;
  isLast?: boolean;
  createdAt?: string;
}

function formatDate(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export default function VideoCard({
  id,
  youtubeId,
  title,
  channel,
  durationSeconds,
  summary,
  topics,
  status,
  errorMessage,
  score,
  matchType,
  index,
  isLast,
  createdAt,
}: Props) {
  const queryClient = useQueryClient();
  const [confirmDelete, setConfirmDelete] = useState(false);

  const retry = useMutation({
    mutationFn: () => retryVideo(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["videos"] }),
  });
  const transcribe = useMutation({
    mutationFn: () => transcribeVideo(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["videos"] }),
  });
  const deleteMut = useMutation({
    mutationFn: () => deleteVideo(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["videos"] }),
  });

  const thumbnail = `https://img.youtube.com/vi/${youtubeId}/mqdefault.jpg`;
  const isFailed = status === "failed";
  const isNoCaptions = status === "no_captions";
  const isProcessing = status === "processing" || status === "pending";
  const isLinked = !isFailed && !isNoCaptions;

  const stop = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const inner = (
    <div
      className="vim-card vim-grid-list-row"
      style={{
        padding: "22px 0",
        borderBottom: isLast ? "none" : "1px solid var(--vim-line-soft)",
      }}
    >
      <div className="vim-thumb vim-thumb-list-row" style={{ width: 176, height: 99 }}>
        <img src={thumbnail} alt="" />
        {durationSeconds ? <span className="dur">{formatDuration(durationSeconds)}</span> : null}
        {isLinked && (
          <div className="play">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="#fff">
              <path d="M3 1.5v11L13 7z" />
            </svg>
          </div>
        )}
      </div>

      <div style={{ minWidth: 0 }}>
        <div
          style={{
            fontSize: 11.5,
            color: "var(--vim-ink-3)",
            marginBottom: 6,
            display: "flex",
            alignItems: "center",
            flexWrap: "wrap",
          }}
        >
          {channel && <span>{channel}</span>}
          {createdAt && (
            <>
              <span className="vim-dot" />
              <span>{formatDate(createdAt)}</span>
            </>
          )}
          {isFailed && (
            <>
              <span className="vim-dot" />
              <span className="vim-status fail">failed</span>
            </>
          )}
          {isNoCaptions && (
            <>
              <span className="vim-dot" />
              <span className="vim-status fail">no captions</span>
            </>
          )}
          {isProcessing && (
            <>
              <span className="vim-dot" />
              <span className="vim-status proc">
                <span className="pulse" />
                {status === "pending" ? "queued" : "transcribing"}
              </span>
            </>
          )}
          {score !== undefined && (
            <>
              <span className="vim-dot" />
              <span className="vim-mono" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--vim-accent-ink)" }}>
                {score.toFixed(3)}
              </span>
            </>
          )}
          {matchType && (
            <>
              <span className="vim-dot" />
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 10.5, letterSpacing: "0.06em", color: "var(--vim-ink-3)", textTransform: "uppercase" }}>
                {matchType}
              </span>
            </>
          )}
        </div>

        <h3
          style={{
            fontFamily: "var(--font-serif)",
            fontSize: 20,
            fontWeight: 500,
            margin: "0 0 8px",
            lineHeight: 1.22,
            letterSpacing: "-0.015em",
            color: "var(--vim-ink)",
          }}
        >
          {title || youtubeId}
        </h3>

        {isFailed && errorMessage && (
          <p style={{ fontSize: 12.5, color: "var(--vim-err)", margin: "0 0 10px" }}>
            {errorMessage}
          </p>
        )}
        {isNoCaptions && (
          <p style={{ fontSize: 12.5, color: "var(--vim-ink-3)", margin: "0 0 10px" }}>
            No captions available on YouTube.
          </p>
        )}
        {!isFailed && !isNoCaptions && summary && (
          <p
            style={{
              fontSize: 13.5,
              lineHeight: 1.55,
              color: "var(--vim-ink-2)",
              margin: "0 0 10px",
              display: "-webkit-box",
              WebkitLineClamp: 2,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {stripMarkdown(summary)}
          </p>
        )}

        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", alignItems: "center" }}>
          {topics?.slice(0, 4).map((t) => (
            <span key={t} className="vim-tag bare" style={{ fontSize: 11 }}>
              #{t}
            </span>
          ))}
          {isFailed && (
            <button
              onClick={(e) => {
                stop(e);
                retry.mutate();
              }}
              disabled={retry.isPending}
              className="vim-btn ghost"
              style={{ padding: "4px 10px", fontSize: 11 }}
            >
              {retry.isPending ? "Retrying…" : "Retry"}
            </button>
          )}
          {isNoCaptions && (
            <button
              onClick={(e) => {
                stop(e);
                transcribe.mutate();
              }}
              disabled={transcribe.isPending}
              className="vim-btn ghost"
              style={{ padding: "4px 10px", fontSize: 11 }}
            >
              {transcribe.isPending ? "Transcribing…" : "Transcribe with Voxtral"}
            </button>
          )}
        </div>
      </div>

      <div
        style={{
          display: "flex",
          flexDirection: "column",
          alignItems: "flex-end",
          gap: 8,
          paddingTop: 4,
        }}
      >
        {index !== undefined && (
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 10.5,
              color: "var(--vim-ink-4)",
              textAlign: "right",
              letterSpacing: "0.08em",
              whiteSpace: "nowrap",
            }}
          >
            №&nbsp;{String(index).padStart(3, "0")}
          </span>
        )}
        {confirmDelete ? (
          <div
            className="flex items-center"
            style={{ gap: 6 }}
            onClick={stop}
          >
            <button
              onClick={() => deleteMut.mutate()}
              disabled={deleteMut.isPending}
              className="vim-btn primary"
              style={{ padding: "4px 10px", fontSize: 11 }}
            >
              {deleteMut.isPending ? "…" : "Yes"}
            </button>
            <button
              onClick={() => setConfirmDelete(false)}
              className="vim-btn ghost"
              style={{ padding: "4px 10px", fontSize: 11 }}
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={(e) => {
              stop(e);
              setConfirmDelete(true);
            }}
            className="vim-btn ghost"
            style={{
              padding: "4px 10px",
              fontSize: 11,
              color: "var(--vim-ink-4)",
              opacity: 0.6,
            }}
            title="Delete video"
          >
            Delete
          </button>
        )}
      </div>
    </div>
  );

  if (!isLinked) return inner;

  return (
    <Link to={`/video/${id}`} style={{ display: "block", color: "inherit", textDecoration: "none" }}>
      {inner}
    </Link>
  );
}
