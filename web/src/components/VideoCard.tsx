import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { retryVideo } from "../api.ts";
import { formatDuration } from "../utils.ts";

interface Props {
  id: string;
  youtubeId: string;
  title: string;
  channel: string;
  durationSeconds?: number;
  summary?: string;
  topics?: string[];
  status?: string;
  errorMessage?: string;
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
  status,
  errorMessage,
  score,
  matchType,
}: Props) {
  const queryClient = useQueryClient();
  const retry = useMutation({
    mutationFn: () => retryVideo(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
    },
  });

  const thumbnail = `https://img.youtube.com/vi/${youtubeId}/mqdefault.jpg`;
  const isFailed = status === "failed";
  const isProcessing = status === "processing" || status === "pending";

  const card = (
    <div
      className={`bg-white dark:bg-zinc-900 border rounded-lg overflow-hidden transition-colors ${
        isFailed
          ? "border-red-300 dark:border-red-900/50"
          : "border-zinc-200 dark:border-zinc-800 hover:border-zinc-400 dark:hover:border-zinc-600"
      }`}
    >
      <div className="flex gap-4 p-4">
        <img
          src={thumbnail}
          alt=""
          className="w-40 h-[90px] object-cover rounded shrink-0 bg-zinc-200 dark:bg-zinc-800"
        />
        <div className="flex-1 min-w-0">
          <h3 className="font-medium text-zinc-900 dark:text-zinc-100 truncate">
            {title || youtubeId}
          </h3>
          <div className="flex items-center gap-2 mt-1 text-sm text-zinc-500 dark:text-zinc-400">
            {channel && <span>{channel}</span>}
            {durationSeconds ? (
              <>
                <span>·</span>
                <span>{formatDuration(durationSeconds)}</span>
              </>
            ) : null}
            {isFailed && (
              <span className="px-1.5 py-0.5 rounded text-xs bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400">
                failed
              </span>
            )}
            {isProcessing && (
              <span className="px-1.5 py-0.5 rounded text-xs bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400">
                processing
              </span>
            )}
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
          {isFailed && errorMessage && (
            <p className="mt-1 text-xs text-red-600 dark:text-red-400 truncate">
              {errorMessage}
            </p>
          )}
          {isFailed && (
            <button
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                retry.mutate();
              }}
              disabled={retry.isPending}
              className="mt-2 px-3 py-1 text-xs bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded hover:bg-red-200 dark:hover:bg-red-900/50 transition-colors disabled:opacity-50"
            >
              {retry.isPending ? "Retrying..." : "Retry"}
            </button>
          )}
          {!isFailed && summary && (
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
    </div>
  );

  // Don't link failed/processing videos to detail page
  if (isFailed || isProcessing) return card;

  return (
    <Link to={`/video/${id}`} className="block">
      {card}
    </Link>
  );
}
