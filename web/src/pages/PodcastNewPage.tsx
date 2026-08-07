import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getEpisodePreview, submitEpisode } from "../api.ts";
import PageHeader from "../components/PageHeader.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { AlertIcon } from "../components/icons.tsx";
import { formatDuration } from "../utils.ts";

/**
 * The target of the "Summarize in vimmary" button in cast2md. It takes an
 * episode ID, shows what it is about to summarize, and redirects to the summary
 * once one exists — including immediately, when the episode has been summarized
 * before.
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
      <div className="empty">
        <div className="kick">New summary</div>
        <h3>No episode was given.</h3>
        <p>This page is opened from cast2md's “Summarize in vimmary” button.</p>
        <Link to="/podcasts" className="btn btn-secondary">Back to podcasts</Link>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="page-head">
        <div>
          <Skel w={150} h={10} />
          <div style={{ marginTop: 10 }}><Skel w={420} h={34} /></div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="empty">
        <div className="kick">New summary</div>
        <h3>This episode could not be read.</h3>
        <p>{(error as Error).message}</p>
        <Link to="/podcasts" className="btn btn-secondary">Back to podcasts</Link>
      </div>
    );
  }

  if (!preview) return null;

  const notReady = preview.status !== "completed";
  const kicker = [
    "New summary",
    preview.feed_title,
    preview.duration_seconds ? formatDuration(preview.duration_seconds) : null,
    preview.published_at ? preview.published_at.slice(0, 10) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <PageHeader
        kicker={kicker}
        title={preview.title}
        actions={
          <a
            className="btn btn-secondary"
            style={{ fontSize: 12 }}
            href={preview.source_url}
            target="_blank"
            rel="noopener noreferrer"
          >
            Open in cast2md
          </a>
        }
      />

      {notReady ? (
        <div className="banner">
          <AlertIcon />
          <span>
            cast2md has not finished transcribing this episode (status: {preview.status}). Come
            back once it is done.
          </span>
        </div>
      ) : null}

      <div style={{ borderTop: "var(--rule-strong)" }} />

      <div className="page-x" style={{ paddingTop: 22, paddingBottom: 32, maxWidth: "68ch" }}>
        {!notReady ? (
          <>
            <div className="field" style={{ maxWidth: 360 }}>
              <label htmlFor="podcast-level">Detail level</label>
              <select
                id="podcast-level"
                className="select"
                value={level}
                onChange={(e) => setLevel(e.target.value)}
              >
                <option value="">Default</option>
                <option value="medium">medium — a few paragraphs</option>
                <option value="deep">deep — segment by segment</option>
              </select>
              <p className="field-hint">
                A deep summary costs more tokens and takes longer. A three-hour episode is worth
                the difference; a twenty-minute one usually is not.
              </p>
            </div>

            <button
              type="button"
              className="btn btn-primary"
              style={{ marginTop: 16 }}
              disabled={submit.isPending}
              onClick={() => submit.mutate()}
            >
              {submit.isPending ? "Starting…" : "Summarize this episode"}
            </button>
            {submit.isError ? (
              <p className="field-error">{(submit.error as Error).message}</p>
            ) : null}
          </>
        ) : null}

        {preview.description ? (
          <section style={{ marginTop: notReady ? 0 : 36 }}>
            <div className="kick" style={{ marginBottom: 10 }}>From the feed</div>
            <p
              style={{
                font: "400 14.5px/1.6 var(--font-body)",
                color: "var(--color-neutral-800)",
                margin: 0,
                whiteSpace: "pre-wrap",
              }}
            >
              {preview.description}
            </p>
          </section>
        ) : null}
      </div>
    </>
  );
}
