import { useEffect, useMemo, useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import {
  deleteVideo,
  fetchKarakeepStatus,
  fetchProviders,
  getVideo,
  getVideoSegments,
  resummarizeVideo,
  retryVideo,
  transcribeVideo,
  type Video,
} from "../api.ts";
import TranscriptPlayer from "../components/TranscriptPlayer.tsx";
import { formatDuration, formatTokens, videoToMarkdown } from "../utils.ts";
import PageHeader from "../components/PageHeader.tsx";
import ConfirmDialog from "../components/ConfirmDialog.tsx";
import Toast, { useToast } from "../components/Toast.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon } from "../components/icons.tsx";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";
import { usePodcastsEnabled } from "../features.ts";
import { isInFlight, shortDate, statusClass, statusLabel } from "../display.ts";

type Tab = "summary" | "chapters" | "transcript";

/* The transcript tab is labelled "Watch": the tab and the rail's player button
   are one destination, and calling it Transcript made them read as two. A
   podcast has no video to watch, so it keeps the literal name — the timed
   Listen player is designed but not shippable (no timed segments from cast2md,
   no audio source). */
function tabLabel(tab: Tab, isPodcast: boolean): string {
  if (tab === "summary") return "Summary";
  if (tab === "chapters") return "Chapters";
  return isPodcast ? "Transcript" : "Watch";
}

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

/* The rail lists chapters as one clamped line each, so it renders plain text
   rather than markdown — the emphasis markers the summaries carry would show
   as literal asterisks there. The Chapters tab still renders them. */
function stripMarkdown(s: string): string {
  return s.replace(/\*\*(.+?)\*\*/g, "$1").replace(/\*(.+?)\*/g, "$1").replace(/`(.+?)`/g, "$1");
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
  // Seconds a chapter click wants the player to jump to once it is mounted.
  const [seekTarget, setSeekTarget] = useState<number | null>(null);

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

  // The timed transcript loads only when the player can use it: opening the
  // tab on a pre-000014 video triggers the server's one-time InnerTube fetch,
  // so this is not free the first time and is never polled.
  const segmentsQuery = useQuery({
    queryKey: ["video-segments", id],
    queryFn: () => getVideoSegments(id!),
    enabled:
      !!id &&
      tab === "transcript" &&
      video?.source === "youtube" &&
      video?.status === "completed" &&
      !!video?.transcript,
    staleTime: Infinity,
    retry: false,
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
    return t;
  }, [video]);

  if (isLoading) {
    return (
      <div className="detail-page">
        <div className="page-head">
          <div>
            <Skel w={190} h={10} />
            <div style={{ marginTop: 10 }}><Skel w={440} h={34} /></div>
          </div>
        </div>
        <div style={{ borderTop: "var(--rule-strong)" }} />
        <div>
          {[92, 100, 84, 96, 70].map((w, i) => (
            <div key={i} style={{ marginBottom: 10 }}><Skel w={`${w}%`} h={16} /></div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="detail-page">
        <div className="empty">
          <div className="kick">Error</div>
          <h3>This summary could not be loaded.</h3>
          <p>{(error as Error).message}</p>
          <Link to="/" className="btn btn-secondary">Back to videos</Link>
        </div>
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

  const meta = [
    podcastsEnabled ? (isPodcast ? "Podcast" : "Video") : null,
    shortDate(video.created_at),
    video.duration_seconds ? formatDuration(video.duration_seconds) : null,
    video.detail_level,
  ]
    .filter(Boolean)
    .join(" · ");

  /* Round for a video channel, rounded-square for a podcast show — the app's
     media-type marker. A podcast row's thumbnail is the show cover; a video
     row's is the video still, which is not a channel avatar, so the channel
     initial stands in there. */
  const kicker = (
    <span className="detail-kicker">
      <span className={`avatar${isPodcast ? " is-show" : ""}`} aria-hidden>
        {isPodcast && video.thumbnail_url ? (
          <img src={video.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" />
        ) : (
          (video.channel || "?").slice(0, 1).toUpperCase()
        )}
      </span>
      <span className="channel">{video.channel}</span>
      <span>{meta}</span>
    </span>
  );

  const showPlayerCard =
    !isPodcast && !failed && video.status === "completed" && !!video.transcript;
  const chapters = video.metadata?.key_points ?? [];
  const topics = video.metadata?.topics ?? [];
  const hasRail = showPlayerCard || chapters.length > 0 || topics.length > 0;

  return (
    <div className="detail-page">
      <div className="detail-content">
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
                    {tabLabel(t, isPodcast)}
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
                  {tabLabel(t, isPodcast)}
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

          <div>
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
                          {/* A plain click jumps to the in-page player; a
                              modified click keeps the link's YouTube target. */}
                          <a
                            href={`https://youtube.com/watch?v=${video.youtube_id}&t=${seconds}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            style={{ color: "inherit" }}
                            onClick={(e) => {
                              if (e.metaKey || e.ctrlKey || e.shiftKey) return;
                              e.preventDefault();
                              setSeekTarget(seconds);
                              setTab("transcript");
                            }}
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
              segmentsQuery.data?.available ? (
                <TranscriptPlayer
                  youtubeId={video.youtube_id}
                  segments={segmentsQuery.data.segments}
                  seekTarget={seekTarget}
                  onSeekHandled={() => setSeekTarget(null)}
                />
              ) : (
                <>
                  <p className="kick" style={{ marginBottom: 10 }}>
                    {wordCount(video.transcript!).toLocaleString()} words
                    {segmentsQuery.isLoading ? " · loading timed transcript…" : ""}
                  </p>
                  {segmentsQuery.isError ? (
                    <p style={{ fontSize: 13, color: "var(--color-neutral-600)", marginBottom: 10 }}>
                      The timed transcript could not be fetched.{" "}
                      <button
                        type="button"
                        className="btn btn-ghost"
                        style={{ fontSize: 12 }}
                        onClick={() => segmentsQuery.refetch()}
                      >
                        Try again
                      </button>
                    </p>
                  ) : null}
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
              )
            ) : null}
          </div>
        </>
      )}
      </div>

      {/* The rail is where playback, chapters and topics live, so the reading
          column stays prose. Below 768px it is one column and this sits after
          the summary — the player teaser included, deliberately: on a phone the
          summary is what the reader came for. */}
      {hasRail && !failed ? (
        <aside className="detail-rail">
          {/* Hidden while the player is open: the card's only job is to lead
              there. */}
          {showPlayerCard && activeTab !== "transcript" ? (
            <section className="card-ink">
              <div className="kick">Watch</div>
              <p style={{ margin: "6px 0 12px" }}>
                Video and transcript side by side, the current line following along.
              </p>
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => setTab("transcript")}
              >
                Open player
              </button>
            </section>
          ) : null}

          {chapters.length > 0 ? (
            <section>
              <div className="kick">Chapters</div>
              <div style={{ marginTop: 8 }}>
                {chapters.map((kp, i) => {
                  const { ts, body, seconds } = parseChapter(kp, i);
                  const seekable = seconds !== undefined && !isPodcast && !!video.transcript;
                  return (
                    <button
                      key={i}
                      type="button"
                      className="rail-chapter"
                      disabled={!seekable}
                      onClick={() => {
                        if (!seekable) return;
                        setSeekTarget(seconds!);
                        setTab("transcript");
                      }}
                    >
                      <time>{ts}</time>
                      <span>{stripMarkdown(body)}</span>
                    </button>
                  );
                })}
              </div>
            </section>
          ) : null}

          {topics.length > 0 ? (
            <section>
              <div className="kick">Topics</div>
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginTop: 10 }}>
                {topics.map((t) => (
                  <Link key={t} to={`/?topic=${encodeURIComponent(t)}`} className="chip chip-topic">
                    {t}
                  </Link>
                ))}
              </div>
            </section>
          ) : null}
        </aside>
      ) : null}

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
    </div>
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
