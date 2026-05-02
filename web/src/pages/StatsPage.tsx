import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchStats, listVideos, retryVideo, deleteVideo } from "../api.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

function busiestWeekday(daily: { date: string; count: number }[]): string {
  if (!daily.length) return "—";
  const byDay = new Map<number, number>();
  for (const d of daily) {
    const dt = new Date(d.date);
    byDay.set(dt.getDay(), (byDay.get(dt.getDay()) ?? 0) + d.count);
  }
  let best = -1;
  let bestN = -1;
  for (const [k, n] of byDay) {
    if (n > bestN) {
      bestN = n;
      best = k;
    }
  }
  return ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][best] ?? "—";
}

function shortDate(d: string): string {
  return new Date(d).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export default function StatsPage() {
  const queryClient = useQueryClient();

  const { data: stats, isLoading, error } = useQuery({
    queryKey: ["stats"],
    queryFn: fetchStats,
  });

  const failedCount = stats?.by_status?.failed ?? 0;

  const { data: failedVideos } = useQuery({
    queryKey: ["videos", "failed"],
    queryFn: () => listVideos({ status: "failed", limit: 20 }),
    enabled: failedCount > 0,
  });

  const retry = useMutation({
    mutationFn: (id: string) => retryVideo(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteVideo(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] });
    },
  });

  if (isLoading)
    return (
      <div className="vim-page">
        <LoadingSkeleton count={3} />
      </div>
    );

  if (error)
    return (
      <div className="vim-page">
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

  if (!stats) return null;

  const completed = stats.by_status?.completed ?? 0;
  const completionRate =
    stats.total_count > 0 ? Math.round((completed / stats.total_count) * 100) : 0;
  const totalHours = stats.total_duration_seconds / 3600;
  // Reading a summary takes roughly 15% of the runtime — so ~85% saved vs watching.
  const savedHours = totalHours * 0.85;

  const sumLast30 = stats.daily_activity.reduce((acc, d) => acc + d.count, 0);
  const maxDaily = Math.max(...stats.daily_activity.map((d) => d.count), 1);
  const today = new Date().toISOString().slice(0, 10);

  const topChannelMax = stats.by_channel[0]?.count ?? 1;
  const topTopicMax = stats.top_topics[0]?.count ?? 1;

  return (
    <div className="vim-page">
      <div className="vim-kicker" style={{ marginBottom: 10 }}>
        — Reading habits
      </div>
      <h1 className="vim-h1-stats-settings">Stats</h1>

      {/* Headline grid */}
      <div className="vim-grid-stats-headline" style={{ marginBottom: 30 }}>
        {[
          { n: stats.total_count.toLocaleString(), l: "summaries" },
          {
            n: totalHours >= 1 ? `${totalHours.toFixed(0)} hrs` : `${Math.round(totalHours * 60)} min`,
            l: "watched by vimmary",
          },
          {
            n: savedHours >= 1 ? `${savedHours.toFixed(0)} hrs` : `${Math.round(savedHours * 60)} min`,
            l: "saved vs 1× speed",
          },
          { n: `${completionRate}%`, l: "completion rate" },
        ].map((x) => (
          <div
            key={x.l}
            style={{
              padding: "22px 24px",
              background: "var(--vim-surface)",
              borderRadius: 12,
              border: "1px solid var(--vim-line-soft)",
            }}
          >
            <div
              style={{
                fontFamily: "var(--font-serif)",
                fontSize: 36,
                fontWeight: 400,
                letterSpacing: "-0.02em",
                color: "var(--vim-ink)",
                lineHeight: 1.05,
              }}
            >
              {x.n}
            </div>
            <div style={{ fontSize: 12, color: "var(--vim-ink-3)", marginTop: 6 }}>{x.l}</div>
          </div>
        ))}
      </div>

      {/* Sparkline card */}
      {stats.daily_activity.length > 0 && (
        <div
          style={{
            padding: 28,
            background: "var(--vim-surface)",
            borderRadius: 12,
            border: "1px solid var(--vim-line-soft)",
            marginBottom: 30,
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "baseline",
              justifyContent: "space-between",
              marginBottom: 18,
              gap: 16,
              flexWrap: "wrap",
            }}
          >
            <div>
              <div className="vim-kicker" style={{ marginBottom: 6 }}>
                Last 30 days
              </div>
              <div
                style={{
                  fontFamily: "var(--font-serif)",
                  fontSize: 22,
                  fontWeight: 500,
                  letterSpacing: "-0.01em",
                  color: "var(--vim-ink)",
                }}
              >
                {sumLast30} summar{sumLast30 === 1 ? "y" : "ies"} · busiest{" "}
                {busiestWeekday(stats.daily_activity)}
              </div>
            </div>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--vim-ink-3)" }}>
              {shortDate(stats.daily_activity[0].date)} —{" "}
              {shortDate(stats.daily_activity[stats.daily_activity.length - 1].date)}
            </div>
          </div>
          <div className="vim-spark">
            {stats.daily_activity.map((d) => (
              <div
                key={d.date}
                className={"bar" + (d.date === today ? " cur" : "")}
                style={{ height: `${(d.count / maxDaily) * 100}%` }}
                title={`${d.date}: ${d.count}`}
              />
            ))}
          </div>
        </div>
      )}

      {/* By status */}
      {Object.keys(stats.by_status).length > 0 && (
        <div
          style={{
            padding: 24,
            background: "var(--vim-surface)",
            borderRadius: 12,
            border: "1px solid var(--vim-line-soft)",
            marginBottom: 30,
          }}
        >
          <div className="vim-kicker" style={{ marginBottom: 16 }}>
            — By status
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {Object.entries(stats.by_status)
              .sort(([, a], [, b]) => b - a)
              .map(([status, count]) => (
                <div
                  key={status}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "120px 1fr 38px",
                    alignItems: "center",
                    gap: 12,
                  }}
                >
                  <span
                    style={{
                      fontSize: 13,
                      color: status === "failed" ? "var(--vim-err)" : "var(--vim-ink-2)",
                    }}
                  >
                    {status}
                  </span>
                  <div className={"vim-bar" + (status === "completed" ? " accent" : "")}>
                    <span
                      style={{
                        width: `${(count / stats.total_count) * 100}%`,
                        background:
                          status === "failed" ? "var(--vim-err)" : undefined,
                      }}
                    />
                  </div>
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 12,
                      color: "var(--vim-ink-3)",
                      textAlign: "right",
                    }}
                  >
                    {count}
                  </span>
                </div>
              ))}
          </div>
        </div>
      )}

      {/* Top channels + topics */}
      <div className="vim-grid-stats-2col" style={{ marginBottom: 30 }}>
        {stats.by_channel.length > 0 && (
          <div
            style={{
              padding: 24,
              background: "var(--vim-surface)",
              borderRadius: 12,
              border: "1px solid var(--vim-line-soft)",
            }}
          >
            <div className="vim-kicker" style={{ marginBottom: 16 }}>
              — Top channels
            </div>
            {stats.by_channel.map((cc, i) => (
              <div
                key={cc.channel}
                style={{
                  display: "grid",
                  gridTemplateColumns: "1fr 38px",
                  alignItems: "center",
                  gap: 12,
                  padding: "9px 0",
                  borderBottom:
                    i === stats.by_channel.length - 1
                      ? "none"
                      : "1px solid var(--vim-line-soft)",
                }}
              >
                <div>
                  <div style={{ fontSize: 13.5, marginBottom: 6, color: "var(--vim-ink)" }}>
                    {cc.channel}
                  </div>
                  <div className="vim-bar accent">
                    <span style={{ width: `${(cc.count / topChannelMax) * 100}%` }} />
                  </div>
                </div>
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 12,
                    color: "var(--vim-ink-3)",
                    textAlign: "right",
                  }}
                >
                  {cc.count}
                </span>
              </div>
            ))}
          </div>
        )}
        {stats.top_topics.length > 0 && (
          <div
            style={{
              padding: 24,
              background: "var(--vim-surface)",
              borderRadius: 12,
              border: "1px solid var(--vim-line-soft)",
            }}
          >
            <div className="vim-kicker" style={{ marginBottom: 16 }}>
              — Top topics
            </div>
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                gap: 6,
                alignContent: "flex-start",
              }}
            >
              {stats.top_topics.map((tc) => (
                <span
                  key={tc.topic}
                  className="vim-tag"
                  style={{
                    fontSize: 12 + Math.min(tc.count / Math.max(topTopicMax / 3, 1), 3),
                    padding: "5px 11px",
                  }}
                >
                  {tc.topic}{" "}
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 10,
                      color: "var(--vim-ink-4)",
                      marginLeft: 4,
                    }}
                  >
                    {tc.count}
                  </span>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Failed videos log */}
      {failedVideos && failedVideos.videos.length > 0 && (
        <div
          style={{
            padding: 24,
            background: "var(--vim-surface)",
            borderRadius: 12,
            border: "1px solid color-mix(in oklch, var(--vim-err) 22%, transparent)",
          }}
        >
          <div className="vim-kicker" style={{ marginBottom: 16, color: "var(--vim-err)" }}>
            — Failed videos
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {failedVideos.videos.map((v) => (
              <div
                key={v.id}
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  justifyContent: "space-between",
                  gap: 16,
                }}
              >
                <div style={{ minWidth: 0, flex: 1 }}>
                  <p
                    style={{
                      color: "var(--vim-ink)",
                      fontSize: 13.5,
                      margin: 0,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {v.title || v.youtube_id}
                  </p>
                  <p
                    style={{
                      color: "var(--vim-err)",
                      fontSize: 12,
                      margin: "2px 0 0",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {v.error_message}
                  </p>
                  <p
                    style={{
                      color: "var(--vim-ink-4)",
                      fontFamily: "var(--font-mono)",
                      fontSize: 11,
                      margin: "2px 0 0",
                    }}
                  >
                    {new Date(v.created_at).toLocaleString()}
                  </p>
                </div>
                <div style={{ display: "flex", gap: 6, flexShrink: 0 }}>
                  <button
                    onClick={() => retry.mutate(v.id)}
                    disabled={retry.isPending}
                    className="vim-btn ghost"
                    style={{ padding: "5px 10px", fontSize: 11 }}
                  >
                    Retry
                  </button>
                  <button
                    onClick={() => remove.mutate(v.id)}
                    disabled={remove.isPending}
                    className="vim-btn outline"
                    style={{ padding: "5px 10px", fontSize: 11 }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
