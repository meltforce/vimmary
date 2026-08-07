import { useEffect, useMemo, useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import {
  deleteVideo,
  fetchKarakeepStatus,
  fetchProviders,
  getVideo,
  resummarizeVideo,
  retryVideo,
  transcribeVideo,
  type Video,
} from "../api.ts";
import { formatDuration, formatTokens, videoToMarkdown } from "../utils.ts";
import PageHeader from "../components/PageHeader.tsx";
import ConfirmDialog from "../components/ConfirmDialog.tsx";
import Toast, { useToast } from "../components/Toast.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { usePodcastsEnabled } from "../features.ts";
import { isInFlight, shortDate, statusClass, statusLabel } from "../display.ts";

type Tab = "summary" | "chapters" | "transcript" | "topics";

const TAB_LABEL: Record<Tab, string> = {
  summary: "Summary",
  chapters: "Chapters",
  transcript: "Transcript",
  topics: "Topics",
};

/** `12:04 — Title` splits into a timestamp column and a body; anything else
 *  keeps its position as the column instead. */
function parseChapter(raw: string, index: number): { ts: string; body: string; seconds?: number } {
  const m = raw.match(/^\s*(\d{1,2}:\d{2}(?::\d{2})?)\s*[—–-]?\s*(.+)/s);
  if (!m) return { ts: String(index + 1).padStart(2, "0"), body: raw.trim() };
  const parts = m[1].split(":").map(Number);
  const seconds =
    parts.length === 3 ? parts[0] * 3600 + parts[1] * 60 + parts[2] : parts[0] * 60 + parts[1];
  return { ts: m[1], body: m[2].trim(), seconds };
}

function wordCount(s: string): number {
  return s.trim() ? s.trim().split(/\s+/).length : 0;
}

export default function VideoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const isDesktop = useIsDesktop();
  const podcastsEnabled = usePodcastsEnabled();
  const toast = useToast();

  const [tab, setTab] = useState<Tab>("summary");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [resumOpen, setResumOpen] = useState(false);

  const { data: providers } = useQuery({ queryKey: ["providers"], queryFn: fetchProviders });
  const { data: karakeep } = useQuery({
    queryKey: ["settings", "karakeep"],
    queryFn: fetchKarakeepStatus,
  });

  const { data: video, isLoading, error } = useQuery({
    queryKey: ["video", id],
    queryFn: () => getVideo(id!),
    enabled: !!id,
    refetchInterval: (q) => (q.state.data && isInFlight(q.state.data.status) ? 2000 : false),
  });

  useEffect(() => {
    if (video?.title) document.title = `${video.title} — vimmary`;
    return () => {
      document.title = "vimmary";
    };
  }, [video?.title]);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["video", id] });

  const resummarize = useMutation({
    mutationFn: (opts: { level: string; language?: string; provider?: string }) =>
      resummarizeVideo(id!, opts.level, opts.language, opts.provider),
    onSuccess: (r) => {
      setResumOpen(false);
      refresh();
      toast.show(`Queued for a ${r.level} summary.`);
    },
  });

  const retry = useMutation({
    mutationFn: () => retryVideo(id!),
    onSuccess: () => {
      refresh();
      toast.show("Queued for another attempt.");
    },
  });

  const transcribe = useMutation({
    mutationFn: () => transcribeVideo(id!),
    onSuccess: () => {
      refresh();
      toast.show("Queued for Voxtral transcription.");
    },
  });

  const del = useMutation({
    mutationFn: () => deleteVideo(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
      navigate(video?.source === "podcast" ? "/podcasts" : "/");
    },
  });

  const tabs = useMemo<Tab[]>(() => {
    if (!video) return ["summary"];
    const t: Tab[] = ["summary"];
    if (video.metadata?.key_points?.length) t.push("chapters");
    if (video.transcript) t.push("transcript");
    if (video.metadata?.topics?.length) t.push("topics");
    return t;
  }, [video]);

  if (isLoading) {
    return (
      <>
        <div className="page-head">
          <div>
            <Skel w={190} h={10} />
            <div style={{ marginTop: 10 }}><Skel w={440} h={34} /></div>
          </div>
        </div>
        <div style={{ borderTop: "var(--rule-strong)" }} />
        <div className="page-x" style={{ paddingTop: 24, maxWidth: "68ch" }}>
          {[92, 100, 84, 96, 70].map((w, i) => (
            <div key={i} style={{ marginBottom: 10 }}><Skel w={`${w}%`} h={16} /></div>
          ))}
        </div>
      </>
    );
  }

  if (error) {
    return (
      <div className="empty">
        <div className="kick">Error</div>
        <h3>This summary could not be loaded.</h3>
        <p>{(error as Error).message}</p>
        <Link to="/" className="btn btn-secondary">Back to videos</Link>
      </div>
    );
  }

  if (!video) return null;

  const isPodcast = video.source === "podcast";
  const externalUrl = isPodcast
    ? video.source_url ?? ""
    : `https://youtube.com/watch?v=${video.youtube_id}`;
  const inFlight = isInFlight(video.status);
  const failed = video.status === "failed" || video.status === "no_captions";
  const activeTab = tabs.includes(tab) ? tab : "summary";

  const kicker = [
    podcastsEnabled ? (isPodcast ? "Podcast" : "Video") : null,
    video.channel,
    shortDate(video.created_at),
    video.duration_seconds ? formatDuration(video.duration_seconds) : null,
    video.detail_level,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <PageHeader
        kicker={kicker}
        title={video.title || video.youtube_id}
        actions={
          <>
            {inFlight || failed ? (
              <span className={`status ${statusClass(video.status)}`}>
                {statusLabel(video.status)}
              </span>
            ) : null}
            {externalUrl ? (
              <a
                className="btn btn-secondary"
                style={{ fontSize: 12 }}
                href={externalUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                {isPodcast ? "Open in cast2md" : "Open on YouTube"}
              </a>
            ) : null}
            {!isPodcast && video.karakeep_bookmark_id && karakeep?.base_url ? (
              <a
                className="btn btn-secondary"
                style={{ fontSize: 12 }}
                href={`${karakeep.base_url}/dashboard/preview/${video.karakeep_bookmark_id}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                Open in Karakeep
              </a>
            ) : null}
            {video.summary ? (
              <button
                type="button"
                className="btn btn-secondary"
                style={{ fontSize: 12 }}
                onClick={() => {
                  navigator.clipboard.writeText(videoToMarkdown(video));
                  toast.show("Markdown copied.");
                }}
              >
                Copy Markdown
              </button>
            ) : null}
            <button
              type="button"
              className="btn btn-ghost"
              style={{ fontSize: 12 }}
              onClick={() => setResumOpen(true)}
            >
              Resummarize
            </button>
            <button
              type="button"
              className="btn btn-danger"
              style={{ fontSize: 12 }}
              onClick={() => setConfirmDelete(true)}
            >
              Delete
            </button>
          </>
        }
      />

      {failed ? (
        <FailureState
          video={video}
          onRetry={() => retry.mutate()}
          onTranscribe={() => transcribe.mutate()}
          busy={retry.isPending || transcribe.isPending}
        />
      ) : (
        <>
          <div className="filters">
            {isDesktop ? (
              <span className="seg">
                {tabs.map((t) => (
                  <label key={t} className="seg-opt">
                    <input
                      type="radio"
                      name="detail-tab"
                      checked={activeTab === t}
                      onChange={() => setTab(t)}
                    />
                    {TAB_LABEL[t]}
                  </label>
                ))}
              </span>
            ) : (
              tabs.map((t) => (
                <button
                  key={t}
                  type="button"
                  className="chip"
                  aria-pressed={activeTab === t}
                  onClick={() => setTab(t)}
                >
                  {TAB_LABEL[t]}
                </button>
              ))
            )}
            {video.summary_provider ? (
              <span
                className="spacer mono"
                style={{ color: "var(--color-neutral-600)" }}
              >
                {video.summary_provider}
                {video.summary_model ? ` · ${video.summary_model}` : ""}
                {(video.summary_input_tokens ?? 0) > 0
                  ? ` · ${formatTokens(video.summary_input_tokens!)} in / ${formatTokens(
                      video.summary_output_tokens!,
                    )} out`
                  : ""}
              </span>
            ) : null}
          </div>

          {inFlight ? (
            <div className="banner">
              <AlertIcon />
              <span>
                {isPodcast ? "This episode" : "This video"} is still being processed. The page
                updates itself.
              </span>
            </div>
          ) : null}

          <div className="page-x" style={{ paddingTop: 24, paddingBottom: 40 }}>
            {activeTab === "summary" ? (
              video.summary ? (
                <div className="reader">
                  <ReactMarkdown>{video.summary}</ReactMarkdown>
                  {video.metadata?.action_items?.length ? (
                    <>
                      <h3>Things to try</h3>
                      <ul>
                        {video.metadata.action_items.map((ai, i) => (
                          <li key={i}>
                            <ReactMarkdown>{ai}</ReactMarkdown>
                          </li>
                        ))}
                      </ul>
                    </>
                  ) : null}
                </div>
              ) : (
                <p style={{ color: "var(--color-neutral-600)", fontSize: 13.5 }}>
                  No summary yet.
                </p>
              )
            ) : null}

            {activeTab === "chapters" ? (
              <div style={{ maxWidth: "72ch" }}>
                {video.metadata!.key_points!.map((kp, i) => {
                  const { ts, body, seconds } = parseChapter(kp, i);
                  return (
                    <div key={i} className="cue">
                      {seconds !== undefined && !isPodcast ? (
                        <time>
                          <a
                            href={`https://youtube.com/watch?v=${video.youtube_id}&t=${seconds}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            style={{ color: "inherit" }}
                          >
                            {ts}
                          </a>
                        </time>
                      ) : (
                        <time>{ts}</time>
                      )}
                      <div className="reader" style={{ fontSize: 14.5, lineHeight: 1.55 }}>
                        <ReactMarkdown>{body}</ReactMarkdown>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : null}

            {activeTab === "transcript" ? (
              <>
                <p className="kick" style={{ marginBottom: 10 }}>
                  {wordCount(video.transcript!).toLocaleString()} words
                </p>
                <div
                  className="transcript"
                  style={{
                    padding: "16px 0",
                    fontSize: 13.5,
                    lineHeight: 1.65,
                    color: "var(--color-neutral-800)",
                    whiteSpace: "pre-wrap",
                    maxWidth: "76ch",
                  }}
                >
                  {video.transcript}
                </div>
              </>
            ) : null}

            {activeTab === "topics" ? (
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                {video.metadata!.topics!.map((t) => (
                  <span key={t} className="tag tag-neutral">
                    {t}
                  </span>
                ))}
              </div>
            ) : null}
          </div>
        </>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title={`Delete this ${isPodcast ? "episode" : "video"}?`}
        body="The summary, the transcript and the embedding are removed. The source is untouched, so it can be submitted again."
        confirmLabel="Delete"
        danger
        busy={del.isPending}
        onConfirm={() => del.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />

      {resumOpen ? (
        <ResummarizeDialog
          video={video}
          providers={providers?.providers ?? []}
          defaultProvider={providers?.default ?? ""}
          busy={resummarize.isPending}
          error={resummarize.error as Error | null}
          onSubmit={(opts) => resummarize.mutate(opts)}
          onCancel={() => setResumOpen(false)}
        />
      ) : null}

      <Toast message={toast.message} onDismiss={toast.dismiss} />
    </>
  );
}

/* The reader area is replaced rather than pushed down: on a failure the error
   is the content. It states the message verbatim — a summarised failure is a
   failure that has to be looked up in the logs. */
function FailureState({
  video,
  onRetry,
  onTranscribe,
  busy,
}: {
  video: Video;
  onRetry: () => void;
  onTranscribe: () => void;
  busy: boolean;
}) {
  const noCaptions = video.status === "no_captions";
  return (
    <div className="empty">
      <div className="kick">{noCaptions ? "No captions" : "Failed"}</div>
      <h3>{noCaptions ? "YouTube has no transcript for this one." : "This summary did not finish."}</h3>
      <p>
        {video.error_message ||
          (noCaptions
            ? "Voxtral can transcribe the audio instead. It costs a transcription call and takes about as long as the video."
            : "No message was recorded.")}
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <button type="button" className="btn btn-primary" disabled={busy} onClick={onRetry}>
          Try again
        </button>
        {video.source === "youtube" ? (
          <button type="button" className="btn btn-secondary" disabled={busy} onClick={onTranscribe}>
            Transcribe with Voxtral
          </button>
        ) : null}
      </div>
    </div>
  );
}

function ResummarizeDialog({
  video,
  providers,
  defaultProvider,
  busy,
  error,
  onSubmit,
  onCancel,
}: {
  video: Video;
  providers: string[];
  defaultProvider: string;
  busy: boolean;
  error: Error | null;
  onSubmit: (opts: { level: string; language?: string; provider?: string }) => void;
  onCancel: () => void;
}) {
  const [level, setLevel] = useState(video.detail_level === "deep" ? "deep" : "medium");
  const [language, setLanguage] = useState("");
  const [provider, setProvider] = useState("");

  return (
    <div className="dialog-backdrop" onClick={onCancel}>
      <div className="dialog" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
        <h2 className="dialog-title">Summarize again</h2>
        <p className="dialog-body">
          The transcript is reused; only the summary is regenerated. A deep summary costs more
          tokens and takes longer.
        </p>

        <div className="field" style={{ marginBottom: 14 }}>
          <label htmlFor="resum-level">Detail</label>
          <select
            id="resum-level"
            className="select"
            value={level}
            onChange={(e) => setLevel(e.target.value)}
          >
            <option value="medium">medium</option>
            <option value="deep">deep</option>
          </select>
        </div>

        <div className="field" style={{ marginBottom: 14 }}>
          <label htmlFor="resum-lang">Language</label>
          <select
            id="resum-lang"
            className="select"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
          >
            <option value="">Auto ({video.language || "unknown"})</option>
            <option value="en">English</option>
            <option value="de">German</option>
            <option value="fr">French</option>
            <option value="es">Spanish</option>
          </select>
        </div>

        {providers.length > 1 ? (
          <div className="field" style={{ marginBottom: 14 }}>
            <label htmlFor="resum-provider">Provider</label>
            <select
              id="resum-provider"
              className="select"
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
            >
              <option value="">Default ({defaultProvider})</option>
              {providers.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
        ) : null}

        {error ? <p className="field-error">{error.message}</p> : null}

        <div className="dialog-actions" style={{ marginTop: 20 }}>
          <button
            type="button"
            className="btn btn-primary"
            disabled={busy}
            onClick={() =>
              onSubmit({
                level,
                language: language || undefined,
                provider: provider || undefined,
              })
            }
          >
            {busy ? "Queueing…" : "Summarize"}
          </button>
          <button type="button" className="btn btn-secondary" onClick={onCancel}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
