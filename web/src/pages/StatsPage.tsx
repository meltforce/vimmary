import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchStats, listVideos, retryAllFailed, type ContentSource, type DailyCount } from "../api.ts";
import { usePodcastsEnabled } from "../features.ts";
import PageHeader from "../components/PageHeader.tsx";
import RangeControl from "../components/RangeControl.tsx";
import Toast, { useToast } from "../components/Toast.tsx";
import { Skel } from "../components/LoadingSkeleton.tsx";
import { shortDate, statusClass, statusLabel } from "../display.ts";

type StatsScope = ContentSource | "all";

const SCOPES: StatsScope[] = ["all", "youtube", "podcast"];
const SCOPE_LABEL: Record<string, string> = {
  all: "Everything",
  youtube: "Videos",
  podcast: "Podcasts",
};

function busiestWeekday(daily: DailyCount[]): string {
  if (!daily.length) return "—";
  const byDay = new Map<number, number>();
  for (const d of daily) {
    const day = new Date(d.date).getDay();
    byDay.set(day, (byDay.get(day) ?? 0) + d.count);
  }
  let best = -1;
  let bestN = -1;
  for (const [k, n] of byDay) {
    if (n > bestN) {
      bestN = n;
      best = k;
    }
  }
  return (
    ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][best] ?? "—"
  );
}

export default function StatsPage() {
  const queryClient = useQueryClient();
  const podcastsEnabled = usePodcastsEnabled();
  const toast = useToast();
  const [scope, setScope] = useState<StatsScope>("all");

  // With cast2md off, count videos rather than everything. Podcast rows can
  // still exist from an earlier configuration, and counting them here while the
  // video list hides them would make the two pages disagree.
  const effectiveScope: StatsScope = podcastsEnabled ? scope : "youtube";

  const { data: stats, isLoading, error } = useQuery({
    queryKey: ["stats", effectiveScope],
    queryFn: () => fetchStats(effectiveScope),
  });

  const failedCount = stats?.by_status?.failed ?? 0;

  const { data: failed } = useQuery({
    queryKey: ["videos", "failed", effectiveScope],
    queryFn: () => listVideos({ status: "failed", source: effectiveScope, limit: 20 }),
    enabled: failedCount > 0,
  });

  const retryAll = useMutation({
    mutationFn: retryAllFailed,
    onSuccess: (r) => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
      toast.show(`${r.retried} ${r.retried === 1 ? "video" : "videos"} queued for retry.`);
    },
  });

  if (error) {
    return (
      <div className="empty">
        <div className="kick">Error</div>
        <h3>Stats could not be loaded.</h3>
        <p>{(error as Error).message}</p>
      </div>
    );
  }

  const totalHours = (stats?.total_duration_seconds ?? 0) / 3600;
  // Reading a summary takes roughly 15% of the runtime, so ~85% is saved.
  const savedHours = totalHours * 0.85;
  const completed = stats?.by_status?.completed ?? 0;
  const completion = stats && stats.total_count > 0
    ? Math.round((completed / stats.total_count) * 100)
    : 0;

  return (
    <>
      <PageHeader
        kicker="Library"
        title="Stats"
        actions={
          podcastsEnabled ? (
            <RangeControl
              options={SCOPES}
              value={effectiveScope}
              onChange={setScope}
              label="Scope"
              longLabel={SCOPE_LABEL}
              name="stats-scope"
              note="Everything counts videos and podcast episodes together."
            />
          ) : null
        }
      />

      <div className="hero">
        <HeroCell label="Summaries" value={stats?.total_count.toLocaleString()} />
        <HeroCell
          label="Runtime"
          value={stats ? hoursLabel(totalHours) : undefined}
          unit="watched"
        />
        <HeroCell label="Saved" value={stats ? hoursLabel(savedHours) : undefined} unit="vs 1×" />
        <HeroCell label="Completed" value={stats ? `${completion}%` : undefined} />
      </div>

      {isLoading || !stats ? (
        <div className="page-x" style={{ paddingTop: 28 }}>
          <Skel w={280} h={16} />
        </div>
      ) : (
        <>
          <Section
            title="Last 30 days"
            note={
              stats.daily_activity.length
                ? `${stats.daily_activity.reduce((n, d) => n + d.count, 0)} summaries · busiest ${busiestWeekday(stats.daily_activity)}`
                : undefined
            }
          >
            <DailyChart daily={stats.daily_activity} />
          </Section>

          {Object.keys(stats.by_status).length > 0 ? (
            <Section title="By status">
              <table className="table">
                <thead>
                  <tr>
                    <th style={{ width: 160 }}>Status</th>
                    <th>Share</th>
                    <th className="right" style={{ width: 90 }}>Count</th>
                  </tr>
                </thead>
                <tbody>
                  {Object.entries(stats.by_status)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, count]) => (
                      <tr key={status}>
                        <td>
                          <span className={`status ${statusClass(status)}`}>
                            {statusLabel(status)}
                          </span>
                        </td>
                        <td>
                          <Bar
                            fraction={count / Math.max(stats.total_count, 1)}
                            accent={status === "failed"}
                          />
                        </td>
                        <td className="num right">{count}</td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </Section>
          ) : null}

          {stats.by_channel.length > 0 ? (
            <Section title="Top channels">
              <table className="table">
                <thead>
                  <tr>
                    <th style={{ width: "40%" }}>Channel</th>
                    <th>Share</th>
                    <th className="right" style={{ width: 90 }}>Summaries</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.by_channel.map((c) => (
                    <tr key={c.channel}>
                      <td>{c.channel}</td>
                      <td>
                        <Bar fraction={c.count / Math.max(stats.by_channel[0].count, 1)} />
                      </td>
                      <td className="num right">{c.count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          ) : null}

          {stats.top_topics.length > 0 ? (
            <Section title="Top topics">
              <table className="table">
                <thead>
                  <tr>
                    <th style={{ width: "40%" }}>Topic</th>
                    <th>Share</th>
                    <th className="right" style={{ width: 90 }}>Summaries</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.top_topics.map((t) => (
                    <tr key={t.topic}>
                      <td>{t.topic}</td>
                      <td>
                        <Bar fraction={t.count / Math.max(stats.top_topics[0].count, 1)} />
                      </td>
                      <td className="num right">{t.count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          ) : null}

          {failed && failed.videos.length > 0 ? (
            <Section title="Failed">
              <table className="table">
                <thead>
                  <tr>
                    <th>Video</th>
                    <th style={{ width: "38%" }}>Reason</th>
                    <th className="right" style={{ width: 110 }}>Added</th>
                  </tr>
                </thead>
                <tbody>
                  {failed.videos.map((v) => (
                    <tr key={v.id}>
                      <td style={{ fontWeight: 500 }}>
                        <Link
                          to={v.source === "podcast" ? `/podcast/${v.id}` : `/video/${v.id}`}
                          style={{ color: "inherit" }}
                        >
                          {v.title || v.youtube_id}
                        </Link>
                      </td>
                      <td style={{ color: "var(--color-accent-700)", fontSize: 12.5 }}>
                        {v.error_message}
                      </td>
                      <td className="num right" style={{ fontSize: 12.5, color: "var(--color-neutral-600)" }}>
                        {shortDate(v.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          ) : null}
        </>
      )}

      <div className="footer">
        {failedCount > 0 ? (
          <button
            type="button"
            className="btn btn-ghost"
            style={{ fontSize: 12.5 }}
            disabled={retryAll.isPending}
            onClick={() => retryAll.mutate()}
          >
            Retry all {failedCount} failed →
          </button>
        ) : null}
        <span className="spacer note">
          {podcastsEnabled ? SCOPE_LABEL[effectiveScope] : "Videos"}
        </span>
      </div>

      <Toast message={toast.message} onDismiss={toast.dismiss} />
    </>
  );
}

function hoursLabel(h: number): string {
  return h >= 1 ? `${h.toFixed(0)} h` : `${Math.round(h * 60)} min`;
}

function HeroCell({ label, value, unit }: { label: string; value?: string; unit?: string }) {
  return (
    <div>
      <div className="kick">{label}</div>
      <div className="value">
        {value ?? <Skel w={92} h={44} />}
        {/* The value's -0.035em tracking is inherited and eats the space in a
            15px word, so the unit resets it. */}
        {value && unit ? (
          <span className="unit" style={{ letterSpacing: "normal" }}>
            {unit}
          </span>
        ) : null}
      </div>
    </div>
  );
}

/* Section head, then the content. No card, no frame: a 2px rule and a kicker
   carry the break. */
function Section({
  title,
  note,
  children,
}: {
  title: string;
  note?: string;
  children: React.ReactNode;
}) {
  return (
    <section>
      <div
        className="page-x flex items-baseline gap-4"
        style={{ paddingTop: 26, paddingBottom: 12, borderTop: "var(--rule-strong)" }}
      >
        <h2 style={{ fontSize: 22 }}>{title}</h2>
        {note ? (
          <span style={{ font: "400 12.5px var(--font-body)", color: "var(--color-neutral-600)" }}>
            {note}
          </span>
        ) : null}
      </div>
      {children}
    </section>
  );
}

/** Square, track in neutral-300, fill in ink — the accent marks failure only. */
function Bar({ fraction, accent }: { fraction: number; accent?: boolean }) {
  return (
    <span
      style={{
        display: "block",
        height: 10,
        background: "var(--color-neutral-300)",
        maxWidth: 320,
      }}
    >
      <span
        style={{
          display: "block",
          height: "100%",
          width: `${Math.max(0, Math.min(1, fraction)) * 100}%`,
          background: accent ? "var(--color-accent)" : "var(--color-text)",
        }}
      />
    </span>
  );
}

/* No frame and no legend: a 2px zero line, 1px grid, .kick axis labels. Bars are
   ink; the current day is the accent, because that is the series the page is
   about. */
function DailyChart({ daily }: { daily: DailyCount[] }) {
  if (!daily.length) {
    return (
      <p className="page-x" style={{ color: "var(--color-neutral-600)", fontSize: 13 }}>
        Nothing summarized in the last 30 days.
      </p>
    );
  }

  const max = Math.max(...daily.map((d) => d.count), 1);
  const today = new Date().toISOString().slice(0, 10);

  return (
    <div className="page-x" style={{ paddingBottom: 8 }}>
      <div
        style={{
          display: "flex",
          alignItems: "flex-end",
          gap: 3,
          height: 120,
          borderBottom: "var(--rule-strong)",
          // One grid line at the midpoint; a full grid says nothing a 30-bar
          // series does not already say.
          backgroundImage:
            "linear-gradient(to bottom, transparent 50%, var(--color-neutral-300) 50%, var(--color-neutral-300) calc(50% + 1px), transparent calc(50% + 1px))",
        }}
      >
        {daily.map((d) => (
          <div
            key={d.date}
            title={`${d.date}: ${d.count}`}
            style={{
              flex: 1,
              height: `${(d.count / max) * 100}%`,
              minHeight: d.count > 0 ? 2 : 0,
              background: d.date === today ? "var(--color-accent)" : "var(--color-text)",
            }}
          />
        ))}
      </div>
      <div className="flex" style={{ justifyContent: "space-between", paddingTop: 6 }}>
        <span className="kick">{shortDate(daily[0].date)}</span>
        <span className="kick">{max} max/day</span>
        <span className="kick">{shortDate(daily[daily.length - 1].date)}</span>
      </div>
    </div>
  );
}
