import { useQuery } from "@tanstack/react-query";
import { fetchStats } from "../api.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

export default function StatsPage() {
  const {
    data: stats,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["stats"],
    queryFn: fetchStats,
  });

  if (isLoading) return <LoadingSkeleton count={2} />;
  if (error) {
    return (
      <div className="text-red-600 dark:text-red-400 text-sm bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
        {(error as Error).message}
      </div>
    );
  }
  if (!stats) return null;

  const maxDaily = Math.max(...stats.daily_activity.map((d) => d.count), 1);

  return (
    <div className="space-y-8">
      {/* Count card */}
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6">
        <p className="text-zinc-500 text-sm">Total videos</p>
        <p className="text-4xl font-bold text-zinc-900 dark:text-zinc-100 mt-1">
          {stats.total_count}
        </p>
      </div>

      {/* By status */}
      {Object.keys(stats.by_status).length > 0 && (
        <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-4">
          <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
            By status
          </h2>
          <div className="space-y-2">
            {Object.entries(stats.by_status)
              .sort(([, a], [, b]) => b - a)
              .map(([status, count]) => (
                <div key={status} className="flex items-center gap-3">
                  <span className="text-zinc-500 dark:text-zinc-400 text-sm w-28 shrink-0">
                    {status}
                  </span>
                  <div className="flex-1 bg-zinc-100 dark:bg-zinc-800 rounded-full h-2">
                    <div
                      className="bg-cyan-500 h-2 rounded-full"
                      style={{
                        width: `${(count / stats.total_count) * 100}%`,
                      }}
                    />
                  </div>
                  <span className="text-zinc-500 text-sm w-10 text-right">
                    {count}
                  </span>
                </div>
              ))}
          </div>
        </div>
      )}

      {/* Channels and Topics side by side */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {stats.by_channel.length > 0 && (
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-3">
            <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
              Top channels
            </h2>
            <div className="space-y-2">
              {stats.by_channel.map((cc) => (
                <div
                  key={cc.channel}
                  className="flex items-center justify-between"
                >
                  <span className="text-cyan-600 dark:text-cyan-400 text-sm">
                    {cc.channel}
                  </span>
                  <span className="text-zinc-500 text-sm">{cc.count}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        {stats.top_topics.length > 0 && (
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-3">
            <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
              Top topics
            </h2>
            <div className="space-y-2">
              {stats.top_topics.map((tc) => (
                <div
                  key={tc.topic}
                  className="flex items-center justify-between"
                >
                  <span className="text-indigo-600 dark:text-indigo-400 text-sm">
                    {tc.topic}
                  </span>
                  <span className="text-zinc-500 text-sm">{tc.count}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* 30-day activity */}
      {stats.daily_activity.length > 0 && (
        <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-4">
          <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
            Last 30 days
          </h2>
          <div className="flex items-end gap-1 h-32">
            {stats.daily_activity.map((d) => (
              <div
                key={d.date}
                className="flex-1 bg-cyan-500/80 rounded-t hover:bg-cyan-400 transition-colors group relative"
                style={{
                  height: `${(d.count / maxDaily) * 100}%`,
                  minHeight: d.count > 0 ? "4px" : "0",
                }}
              >
                <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block bg-zinc-800 text-zinc-300 text-xs px-2 py-1 rounded whitespace-nowrap">
                  {d.date}: {d.count}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
