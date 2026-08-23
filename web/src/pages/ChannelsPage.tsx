import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addChannel,
  deleteChannel,
  fetchVideoFacets,
  importChannels,
  listChannels,
  listPodcastFeeds,
  setChannelEnabled,
  type ChannelSubscription,
  type PodcastFeed,
} from "../api.ts";
import ConfirmDialog from "../components/ConfirmDialog.tsx";
import PageHeader from "../components/PageHeader.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { usePodcastsEnabled } from "../features.ts";
import { useInboxCount } from "../hooks/useInboxCount.ts";

/**
 * Channels and shows as two card shelves — round avatars for video channels,
 * square covers for podcast shows, which is the app's media-type marker
 * everywhere.
 *
 * The screen was Settings → Channels until the Shelf redesign promoted it to
 * the nav: following a channel is browsing, not configuration, and the shelf
 * only reads as a shelf at full page width.
 */

function Artwork({ src, name, show }: { src?: string; name: string; show?: boolean }) {
  return (
    <span className={`avatar is-xl${show ? " is-show" : ""}`} aria-hidden>
      {src ? (
        // no-referrer: yt3.googleusercontent answers a CORB-blocked non-image
        // when the request carries a foreign referrer (observed 2026-08-23).
        <img src={src} alt="" loading="lazy" referrerPolicy="no-referrer" />
      ) : (
        name.slice(0, 1).toUpperCase()
      )}
    </span>
  );
}

function ChannelCard({ channel }: { channel: ChannelSubscription }) {
  const queryClient = useQueryClient();
  const [confirmDelete, setConfirmDelete] = useState(false);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["channels"] });
    queryClient.invalidateQueries({ queryKey: ["inbox"] });
  };

  const toggle = useMutation({
    mutationFn: (enabled: boolean) => setChannelEnabled(channel.id, enabled),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: () => deleteChannel(channel.id),
    onSuccess: () => {
      setConfirmDelete(false);
      invalidate();
    },
  });

  const rowError = (toggle.error ?? remove.error) as Error | undefined;

  return (
    <div className="shelf-card">
      {channel.new_count > 0 ? <span className="badge-new">{channel.new_count} new</span> : null}
      {/* The card opens the channel's rows in the library; managing the
          subscription is the small print underneath. */}
      <Link
        to={`/?channel=${encodeURIComponent(channel.title)}`}
        style={{ color: "inherit", display: "contents" }}
      >
        <Artwork src={channel.thumbnail_url} name={channel.title} />
        <span className="name">{channel.title}</span>
      </Link>
      <span className="meta">{channel.enabled ? "following" : "paused"}</span>
      {channel.last_error ? (
        <span className="meta" style={{ color: "var(--color-accent)" }}>
          {channel.last_error}
        </span>
      ) : null}
      <div style={{ display: "flex", gap: 4, marginTop: 12 }}>
        <button
          type="button"
          className="btn btn-ghost"
          style={{ fontSize: 12 }}
          disabled={toggle.isPending}
          onClick={() => toggle.mutate(!channel.enabled)}
        >
          {channel.enabled ? "Pause" : "Resume"}
        </button>
        <button
          type="button"
          className="btn btn-ghost"
          style={{ fontSize: 12 }}
          disabled={remove.isPending}
          onClick={() => setConfirmDelete(true)}
        >
          Unfollow
        </button>
      </div>
      {rowError ? <p className="field-error">{rowError.message}</p> : null}

      <ConfirmDialog
        open={confirmDelete}
        title={`Unfollow ${channel.title}?`}
        body="Its inbox items go with it. Videos already summarized stay in the library; re-following imports the channel's current videos again."
        confirmLabel="Unfollow"
        danger
        busy={remove.isPending}
        onConfirm={() => remove.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  );
}

function ShowCard({ feed }: { feed: PodcastFeed }) {
  return (
    <Link to={`/podcasts?channel=${encodeURIComponent(feed.title)}`} className="shelf-card" style={{ color: "inherit" }}>
      <Artwork src={feed.image_url} name={feed.title} show />
      <span className="name">{feed.title}</span>
      <span className="meta">
        {feed.summarized_count} of {feed.episode_count} summarized
      </span>
    </Link>
  );
}

/** Channels that produced library rows without being followed — Karakeep
 * one-shots. They are the majority by count and none of them is navigable as a
 * subscription, so they fold into one card until asked for. */
function OneShotFold({ names }: { names: string[] }) {
  const [open, setOpen] = useState(false);

  if (names.length === 0) return null;
  if (!open) {
    return (
      <button type="button" className="shelf-add" onClick={() => setOpen(true)}>
        <span className="name">+{names.length}</span>
        <span className="meta">one-shot channels from Karakeep</span>
      </button>
    );
  }
  return (
    <>
      {names.map((name) => (
        <Link
          key={name}
          to={`/?channel=${encodeURIComponent(name)}`}
          className="shelf-card is-fold"
          style={{ color: "inherit" }}
        >
          <Artwork name={name} />
          <span className="name">{name}</span>
          <span className="meta">not followed</span>
        </Link>
      ))}
    </>
  );
}

export default function ChannelsPage() {
  const queryClient = useQueryClient();
  const podcasts = usePodcastsEnabled();
  const inbox = useInboxCount();
  const [input, setInput] = useState("");
  const [importOpen, setImportOpen] = useState(false);
  const [importCSV, setImportCSV] = useState("");

  const channels = useQuery({ queryKey: ["channels"], queryFn: listChannels });
  const facets = useQuery({ queryKey: ["facets", "youtube"], queryFn: () => fetchVideoFacets("youtube") });
  const feeds = useQuery({
    queryKey: ["podcast-feeds"],
    queryFn: listPodcastFeeds,
    enabled: podcasts,
  });

  const invalidateAll = () => {
    queryClient.invalidateQueries({ queryKey: ["channels"] });
    queryClient.invalidateQueries({ queryKey: ["inbox"] });
  };

  const follow = useMutation({
    mutationFn: () => addChannel(input.trim()),
    onSuccess: () => {
      setInput("");
      invalidateAll();
    },
  });
  const bulkImport = useMutation({
    mutationFn: () => importChannels(importCSV),
    onSuccess: () => {
      setImportCSV("");
      invalidateAll();
    },
  });

  const oneShots = useMemo(() => {
    const followed = new Set((channels.data?.channels ?? []).map((c) => c.title));
    return (facets.data?.channels ?? [])
      .map((f) => f.channel)
      .filter((name) => name && !followed.has(name));
  }, [channels.data, facets.data]);

  const subscribedFeeds = (feeds.data?.feeds ?? []).filter((f) => f.subscribed);

  return (
    <>
      <PageHeader kicker="Library" title="Channels & shows" />

      <div className="feed-page" style={{ display: "flex", flexDirection: "column", gap: 28 }}>
        <form
          className="card"
          onSubmit={(e) => {
            e.preventDefault();
            if (input.trim()) follow.mutate();
          }}
        >
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <input
              className="input"
              style={{ flex: "1 1 260px" }}
              type="text"
              placeholder="@handle or channel URL"
              value={input}
              disabled={follow.isPending}
              onChange={(e) => setInput(e.target.value)}
            />
            <button
              type="submit"
              className="btn btn-primary"
              disabled={follow.isPending || !input.trim()}
            >
              {follow.isPending ? "Resolving…" : "Follow"}
            </button>
          </div>
          {follow.error ? <p className="field-error">{(follow.error as Error).message}</p> : null}
          <p className="field-hint" style={{ marginTop: 8 }}>
            <button
              type="button"
              className="btn btn-ghost"
              style={{ fontSize: 12 }}
              onClick={() => setImportOpen((o) => !o)}
            >
              {importOpen ? "Hide Takeout import" : "Import subscriptions from Google Takeout…"}
            </button>
          </p>
          {importOpen ? (
            <div style={{ marginTop: 8 }}>
              <p
                style={{
                  font: "400 12px/1.5 var(--font-body)",
                  color: "var(--color-text-2)",
                  maxWidth: "62ch",
                  margin: "0 0 8px",
                }}
              >
                Export your YouTube data at takeout.google.com (YouTube → subscriptions), open
                subscriptions.csv and paste its content here. Direct account access is not
                possible — Google closed the subscription and Watch Later APIs to third parties.
              </p>
              <textarea
                className="input"
                style={{ width: "100%", minHeight: 120, fontFamily: "var(--font-mono)", fontSize: 12 }}
                placeholder={"Channel Id,Channel Url,Channel Title\nUC…,https://…,Some Channel"}
                value={importCSV}
                disabled={bulkImport.isPending}
                onChange={(e) => setImportCSV(e.target.value)}
              />
              <div style={{ marginTop: 8 }}>
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={bulkImport.isPending || !importCSV.trim()}
                  onClick={() => bulkImport.mutate()}
                >
                  {bulkImport.isPending ? "Importing…" : "Import"}
                </button>
              </div>
              {bulkImport.isSuccess ? (
                <p className="field-hint">
                  {bulkImport.data.imported} followed · {bulkImport.data.skipped} skipped. The
                  inboxes fill within a few minutes as the channels are polled.
                </p>
              ) : null}
              {bulkImport.error ? (
                <p className="field-error">{(bulkImport.error as Error).message}</p>
              ) : null}
            </div>
          ) : null}
        </form>

        <section>
          <h2 className="kick">Video channels</h2>
          {channels.isLoading ? (
            <Skel w={280} h={16} />
          ) : channels.error ? (
            <p className="field-error">{(channels.error as Error).message}</p>
          ) : (
            <div className="shelf" style={{ marginTop: 14 }}>
              {channels.data?.channels.map((channel) => (
                <ChannelCard key={channel.id} channel={channel} />
              ))}
              <OneShotFold names={oneShots} />
            </div>
          )}
        </section>

        {podcasts && subscribedFeeds.length > 0 ? (
          <section>
            <h2 className="kick">Podcast shows</h2>
            <div className="shelf" style={{ marginTop: 14 }}>
              {subscribedFeeds.map((feed) => (
                <ShowCard key={feed.feed_id} feed={feed} />
              ))}
              <Link to="/settings?tab=podcasts" className="shelf-add" style={{ color: "inherit" }}>
                <span className="name">Subscribe</span>
                <span className="meta">a cast2md feed</span>
              </Link>
            </div>
          </section>
        ) : null}

        {inbox > 0 ? (
          <Link to="/inbox" className="card-ink" style={{ color: "inherit", display: "block" }}>
            <div className="kick">Inbox</div>
            <p style={{ margin: "6px 0 0" }}>
              {inbox} {inbox === 1 ? "video is" : "videos are"} waiting for triage.
            </p>
          </Link>
        ) : null}
      </div>
    </>
  );
}
