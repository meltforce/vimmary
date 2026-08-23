import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addChannel,
  deleteChannel,
  importChannels,
  listChannels,
  setChannelEnabled,
  type ChannelSubscription,
} from "../../api.ts";
import ConfirmDialog from "../../components/ConfirmDialog.tsx";
import { Section, SectionError, SectionLoading, Switch } from "./primitives.tsx";

function formatPolled(iso?: string): string {
  if (!iso) return "never polled";
  return `polled ${new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })}`;
}

function ChannelRow({ channel }: { channel: ChannelSubscription }) {
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
    <div className="set-row" style={{ alignItems: "flex-start" }}>
      <div className="kick" style={{ paddingTop: 4 }}>
        <Switch
          checked={channel.enabled}
          disabled={toggle.isPending}
          label={`Poll ${channel.title}`}
          onChange={(enabled) => toggle.mutate(enabled)}
        />
      </div>

      <div className="val">
        <div style={{ fontSize: 14, fontWeight: 500 }}>{channel.title}</div>
        <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-neutral-600)", marginTop: 3 }}>
          {channel.new_count} new · {channel.enabled ? formatPolled(channel.last_polled_at) : "paused"}
        </div>
        {channel.last_error ? (
          <div style={{ font: "400 11.5px var(--font-body)", color: "var(--color-accent-700)", marginTop: 4 }}>
            {channel.last_error}
          </div>
        ) : null}
        <div style={{ marginTop: 8 }}>
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
      </div>

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

export default function ChannelsSection() {
  const queryClient = useQueryClient();
  const [input, setInput] = useState("");
  const [importOpen, setImportOpen] = useState(false);
  const [importCSV, setImportCSV] = useState("");

  const { data, isLoading, error } = useQuery({ queryKey: ["channels"], queryFn: listChannels });

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

  return (
    <Section
      title="Channels"
      subtitle="Which YouTube channels feed the inbox. Following imports the channel's current videos for triage; every new upload lands there on the next poll. Nothing is summarized until you pick it."
    >
      <form
        className="set-row"
        onSubmit={(e) => {
          e.preventDefault();
          if (input.trim()) follow.mutate();
        }}
      >
        <div className="kick" style={{ paddingTop: 10 }}>
          Follow
        </div>
        <div className="val">
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
                  color: "var(--color-neutral-700)",
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
        </div>
      </form>

      {isLoading ? <SectionLoading /> : null}
      {error ? <SectionError error={error as Error} /> : null}
      {data && data.channels.length === 0 ? (
        <p style={{ padding: "15px 0", fontSize: 13.5, color: "var(--color-neutral-700)" }}>
          No channels followed yet.
        </p>
      ) : null}
      {data?.channels.map((channel) => (
        <ChannelRow key={channel.id} channel={channel} />
      ))}
    </Section>
  );
}
