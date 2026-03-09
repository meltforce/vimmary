import { Link } from "react-router-dom";
import { formatDuration } from "../utils.ts";

interface Props {
  id: string;
  youtubeId: string;
  title: string;
  channel: string;
  durationSeconds?: number;
  summary?: string;
  topics?: string[];
  score?: number;
  matchType?: string;
}

export default function VideoCard({
  id,
  youtubeId,
  title,
  channel,
  durationSeconds,
  summary,
  topics,
  score,
  matchType,
}: Props) {
  const thumbnail = `https://img.youtube.com/vi/${youtubeId}/mqdefault.jpg`;

  return (
    <Link
      to={`/video/${id}`}
      className="block bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden hover:border-zinc-400 dark:hover:border-zinc-600 transition-colors"
    >
      <div className="flex gap-4 p-4">
        <img
          src={thumbnail}
          alt=""
          className="w-40 h-[90px] object-cover rounded shrink-0 bg-zinc-200 dark:bg-zinc-800"
        />
        <div className="flex-1 min-w-0">
          <h3 className="font-medium text-zinc-900 dark:text-zinc-100 truncate">
            {title}
          </h3>
          <div className="flex items-center gap-2 mt-1 text-sm text-zinc-500 dark:text-zinc-400">
            <span>{channel}</span>
            {durationSeconds ? (
              <>
                <span>·</span>
                <span>{formatDuration(durationSeconds)}</span>
              </>
            ) : null}
            {score !== undefined && (
              <>
                <span>·</span>
                <span className="text-cyan-600 dark:text-cyan-400">
                  {score.toFixed(3)}
                </span>
              </>
            )}
            {matchType && (
              <span
                className={`px-1.5 py-0.5 rounded text-xs ${
                  matchType === "both"
                    ? "bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400"
                    : matchType === "semantic"
                      ? "bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400"
                      : "bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400"
                }`}
              >
                {matchType}
              </span>
            )}
          </div>
          {summary && (
            <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400 line-clamp-2">
              {summary}
            </p>
          )}
          {topics && topics.length > 0 && (
            <div className="flex gap-1.5 mt-2 flex-wrap">
              {topics.slice(0, 5).map((topic) => (
                <span
                  key={topic}
                  className="px-2 py-0.5 bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded text-xs"
                >
                  {topic}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>
    </Link>
  );
}
