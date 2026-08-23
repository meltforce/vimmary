import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  dismissAllInbox,
  dismissInboxItem,
  listChannels,
  listInbox,
  summarizeInboxItem,
  type InboxItem,
} from "../api.ts";
import PageHeader from "../components/PageHeader.tsx";
import ConfirmDialog from "../components/ConfirmDialog.tsx";
import Toast, { useToast } from "../components/Toast.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { shortDate } from "../display.ts";

export default function InboxPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [params, setParams] = useSearchParams();
  const [confirmDismissAll, setConfirmDismissAll] = useState(false);

  // The channel filter lives in the URL so reload and back keep it.
  const channelParam = parseInt(params.get("channel") ?? "", 10);
  const subscriptionId = Number.isNaN(channelParam) ? 0 : channelParam;

  useEffect(() => {
    document.title = "Inbox — vimmary";
    return () => {
      document.title = "vimmary";
    };
  }, []);

  const channels = useQuery({ queryKey: ["channels"], queryFn: listChannels });
  const inbox = useQuery({
    queryKey: ["inbox", subscriptionId],
    queryFn: () => listInbox({ subscriptionId: subscriptionId || undefined, limit: 100 }),
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["inbox"] });
    queryClient.invalidateQueries({ queryKey: ["channels"] });
  };

  const summarize = useMutation({
    mutationFn: (item: InboxItem) => summarizeInboxItem(item.id),
    onSuccess: (video, item) => {
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      toast.show(`Queued: ${item.title || video.youtube_id}`);
    },
  });

  const watch = useMutation({
    mutationFn: (item: InboxItem) => summarizeInboxItem(item.id),
    onSuccess: (video) => {
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      navigate(`/video/${video.id}`);
    },
  });

  const dismiss = useMutation({
    mutationFn: (item: InboxItem) => dismissInboxItem(item.id),
    onSuccess: invalidate,
  });

  const dismissAll = useMutation({
    mutationFn: dismissAllInbox,
    onSuccess: (r) => {
      setConfirmDismissAll(false);
      invalidate();
      toast.show(`Dismissed ${r.dismissed} ${r.dismissed === 1 ? "video" : "videos"}.`);
    },
  });

  const busyId = summarize.isPending
    ? summarize.variables?.id
    : watch.isPending
      ? watch.variables?.id
      : dismiss.isPending
        ? dismiss.variables?.id
        : undefined;
  const actionError = (summarize.error ?? watch.error ?? dismiss.error ?? dismissAll.error) as
    | Error
    | null;

  const total = inbox.data?.total ?? 0;
  const followedChannels = channels.data?.channels ?? [];
  const chipChannels = followedChannels.filter((c) => c.new_count > 0);
  const allNewCount = followedChannels.reduce((sum, c) => sum + c.new_count, 0);

  return (
    // feed-page, not detail-page: the inbox is a list screen like the two
    // libraries, and homelab.css centers the nav into the reading column only
    // while a .feed-page is on the page — a detail-page root here left the nav
    // at the viewport edge, jumping on every switch from Videos or Podcasts.
    <div className="feed-page">
      <PageHeader
        kicker="New from followed channels"
        title="Inbox"
        actions={
          total > 0 ? (
            <button
              type="button"
              className="btn btn-secondary"
              style={{ fontSize: 12 }}
              onClick={() => setConfirmDismissAll(true)}
            >
              Dismiss all
            </button>
          ) : undefined
        }
      />

      {chipChannels.length > 1 ? (
        <div className="filters" style={{ flexWrap: "wrap" }}>
          <button
            type="button"
            className="chip"
            aria-pressed={subscriptionId === 0}
            onClick={() => setParams({}, { replace: true })}
          >
            All ({allNewCount})
          </button>
          {chipChannels.map((c) => (
            <button
              key={c.id}
              type="button"
              className="chip"
              aria-pressed={subscriptionId === c.id}
              onClick={() => setParams({ channel: String(c.id) }, { replace: true })}
            >
              {c.title} ({c.new_count})
            </button>
          ))}
        </div>
      ) : null}

      <div className="page-x detail-content">
        {actionError ? <p className="field-error">{actionError.message}</p> : null}

        {inbox.isLoading ? (
          <div>
            {[0, 1, 2].map((i) => (
              <div key={i} style={{ marginBottom: 14 }}>
                <Skel w="100%" h={68} />
              </div>
            ))}
          </div>
        ) : null}

        {inbox.error ? (
          <div className="empty">
            <div className="kick">Error</div>
            <h3>The inbox could not be loaded.</h3>
            <p>{(inbox.error as Error).message}</p>
          </div>
        ) : null}

        {inbox.data && inbox.data.items.length === 0 ? (
          <div className="empty">
            <div className="kick">Nothing to triage</div>
            <h3>
              {followedChannels.length === 0
                ? "You are not following any channels yet."
                : "No new videos from your channels."}
            </h3>
            {followedChannels.length === 0 ? (
              <p>
                Follow a channel under <Link to="/settings">Settings → Channels</Link> and its new
                videos land here.
              </p>
            ) : null}
          </div>
        ) : null}

        {inbox.data?.items.map((item) => (
          <div key={item.id} className="inbox-row">
            <img
              className="inbox-art"
              src={`https://i.ytimg.com/vi/${item.youtube_id}/hqdefault.jpg`}
              alt=""
              loading="lazy"
            />
            <div className="inbox-body">
              <div className="inbox-title">{item.title || item.youtube_id}</div>
              <div className="inbox-meta">
                {[item.channel_title, item.published_at ? shortDate(item.published_at) : null]
                  .filter(Boolean)
                  .join(" · ")}
              </div>
              <div className="inbox-actions">
                <button
                  type="button"
                  className="btn btn-primary"
                  style={{ fontSize: 12 }}
                  disabled={busyId === item.id}
                  title="Summarize and open — the player is on the detail page"
                  onClick={() => watch.mutate(item)}
                >
                  Watch
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  style={{ fontSize: 12 }}
                  disabled={busyId === item.id}
                  onClick={() => summarize.mutate(item)}
                >
                  Summarize
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  style={{ fontSize: 12 }}
                  disabled={busyId === item.id}
                  onClick={() => dismiss.mutate(item)}
                >
                  Dismiss
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      <ConfirmDialog
        open={confirmDismissAll}
        title="Dismiss everything?"
        body={`${total} ${total === 1 ? "video leaves" : "videos leave"} the inbox. Nothing is deleted from the channels — a dismissed video just stops asking.`}
        confirmLabel="Dismiss all"
        busy={dismissAll.isPending}
        onConfirm={() => dismissAll.mutate()}
        onCancel={() => setConfirmDismissAll(false)}
      />

      <Toast message={toast.message} onDismiss={toast.dismiss} />
    </div>
  );
}
