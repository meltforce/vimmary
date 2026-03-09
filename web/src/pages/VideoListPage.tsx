import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { listVideos, searchVideos, submitVideo } from "../api.ts";
import VideoCard from "../components/VideoCard.tsx";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

const PAGE_SIZE = 20;

export default function VideoListPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get("q") || "";
  const [input, setInput] = useState(query);
  const [youtubeUrl, setYoutubeUrl] = useState("");
  const page = parseInt(searchParams.get("page") || "1", 10);
  const offset = (page - 1) * PAGE_SIZE;

  const searchResult = useQuery({
    queryKey: ["search", query],
    queryFn: () => searchVideos(query),
    enabled: query.length > 0,
  });

  const listResult = useQuery({
    queryKey: ["videos", offset],
    queryFn: () => listVideos({ limit: PAGE_SIZE, offset }),
    enabled: query.length === 0,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.videos.some(v => v.status === "pending" || v.status === "processing")) {
        return 3000;
      }
      return false;
    },
  });

  const submit = useMutation({
    mutationFn: (url: string) => submitVideo(url),
    onSuccess: () => {
      setYoutubeUrl("");
      queryClient.invalidateQueries({ queryKey: ["videos"] });
    },
  });

  const isSearching = query.length > 0;
  const isLoading = isSearching ? searchResult.isLoading : listResult.isLoading;
  const error = isSearching ? searchResult.error : listResult.error;

  function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = input.trim();
    if (trimmed) {
      setSearchParams({ q: trimmed });
    } else {
      setSearchParams({});
    }
  }

  function handleClear() {
    setInput("");
    setSearchParams({});
  }

  return (
    <div className="space-y-6">
      <form onSubmit={handleSearch} className="flex gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Search videos..."
          className="flex-1 px-3 py-2 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-cyan-500/50"
        />
        <button
          type="submit"
          className="px-4 py-2 bg-cyan-600 text-white rounded-lg text-sm hover:bg-cyan-500 transition-colors"
        >
          Search
        </button>
        {query && (
          <button
            type="button"
            onClick={handleClear}
            className="px-3 py-2 bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded-lg text-sm hover:bg-zinc-300 dark:hover:bg-zinc-700 transition-colors"
          >
            Clear
          </button>
        )}
      </form>

      {/* Add YouTube video */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const trimmed = youtubeUrl.trim();
          if (trimmed) submit.mutate(trimmed);
        }}
        className="flex gap-2"
      >
        <input
          type="text"
          value={youtubeUrl}
          onChange={(e) => setYoutubeUrl(e.target.value)}
          placeholder="Add YouTube URL..."
          className="flex-1 px-3 py-2 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-emerald-500/50"
        />
        <button
          type="submit"
          disabled={submit.isPending || !youtubeUrl.trim()}
          className="px-4 py-2 bg-emerald-600 text-white rounded-lg text-sm hover:bg-emerald-500 transition-colors disabled:opacity-50"
        >
          {submit.isPending ? "Adding..." : "Add"}
        </button>
      </form>
      {submit.isSuccess && (
        <div className="text-emerald-600 dark:text-emerald-400 text-sm bg-emerald-50 dark:bg-emerald-950/30 border border-emerald-200 dark:border-emerald-900/50 rounded-lg p-3">
          Video submitted for processing. It will appear shortly.
        </div>
      )}
      {submit.isError && (
        <div className="text-red-600 dark:text-red-400 text-sm bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
          {(submit.error as Error).message}
        </div>
      )}

      {error && (
        <div className="text-red-600 dark:text-red-400 text-sm bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
          {(error as Error).message}
        </div>
      )}

      {searchResult.data?.warnings?.map((w, i) => (
        <div
          key={i}
          className="text-amber-600 dark:text-amber-400 text-sm bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900/50 rounded-lg p-3"
        >
          {w}
        </div>
      ))}

      {isLoading ? (
        <LoadingSkeleton count={3} />
      ) : isSearching ? (
        <div className="space-y-3">
          {searchResult.data && searchResult.data.results.length === 0 && (
            <p className="text-zinc-500 text-sm py-8 text-center">
              No results found for "{query}"
            </p>
          )}
          {searchResult.data?.results.map((m) => (
            <VideoCard
              key={m.id}
              id={m.id}
              youtubeId={m.youtube_id}
              title={m.title}
              channel={m.channel}
              summary={m.summary}
              topics={m.metadata?.topics}
              score={m.score}
              matchType={m.match_type}
            />
          ))}
        </div>
      ) : (
        <div className="space-y-3">
          {listResult.data && listResult.data.videos.length === 0 && (
            <p className="text-zinc-500 text-sm py-8 text-center">
              No videos yet
            </p>
          )}
          {listResult.data?.videos.map((v) => (
            <VideoCard
              key={v.id}
              id={v.id}
              youtubeId={v.youtube_id}
              title={v.title}
              channel={v.channel}
              durationSeconds={v.duration_seconds}
              summary={v.summary}
              topics={v.metadata?.topics}
              status={v.status}
              errorMessage={v.error_message}
            />
          ))}

          {/* Pagination */}
          {listResult.data && listResult.data.total > PAGE_SIZE && (
            <div className="flex items-center justify-center gap-4 pt-4">
              <button
                disabled={page <= 1}
                onClick={() =>
                  setSearchParams({ page: String(page - 1) })
                }
                className="px-3 py-1.5 text-sm bg-zinc-200 dark:bg-zinc-800 rounded-md disabled:opacity-30 hover:bg-zinc-300 dark:hover:bg-zinc-700 transition-colors"
              >
                Previous
              </button>
              <span className="text-sm text-zinc-500">
                Page {page} of{" "}
                {Math.ceil(listResult.data.total / PAGE_SIZE)}
              </span>
              <button
                disabled={offset + PAGE_SIZE >= listResult.data.total}
                onClick={() =>
                  setSearchParams({ page: String(page + 1) })
                }
                className="px-3 py-1.5 text-sm bg-zinc-200 dark:bg-zinc-800 rounded-md disabled:opacity-30 hover:bg-zinc-300 dark:hover:bg-zinc-700 transition-colors"
              >
                Next
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
