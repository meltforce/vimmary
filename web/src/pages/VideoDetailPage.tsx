import { useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import { getVideo, resummarizeVideo, deleteVideo, fetchProviders } from "../api.ts";
import { formatDuration, formatTokens, videoToMarkdown } from "../utils.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

export default function VideoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [showTranscript, setShowTranscript] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [resumLang, setResumLang] = useState("");
  const [resumProvider, setResumProvider] = useState("");

  const { data: providers } = useQuery({
    queryKey: ["providers"],
    queryFn: fetchProviders,
  });

  const { data: video, isLoading, error } = useQuery({
    queryKey: ["video", id],
    queryFn: () => getVideo(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const v = query.state.data;
      if (v && (v.status === "pending" || v.status === "processing")) {
        return 2000;
      }
      return false;
    },
  });

  const resummarize = useMutation({
    mutationFn: (level: string) => resummarizeVideo(id!, level, resumLang || undefined, resumProvider || undefined),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["video", id] });
    },
  });

  const del = useMutation({
    mutationFn: () => deleteVideo(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
      navigate("/");
    },
  });

  if (isLoading) return <LoadingSkeleton count={2} />;
  if (error) {
    return (
      <div className="text-red-600 dark:text-red-400 text-sm bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
        {(error as Error).message}
      </div>
    );
  }
  if (!video) return null;

  const thumbnail = `https://img.youtube.com/vi/${video.youtube_id}/mqdefault.jpg`;
  const youtubeUrl = `https://youtube.com/watch?v=${video.youtube_id}`;
  const isProcessing = video.status === "pending" || video.status === "processing";

  function handleCopy() {
    if (!video) return;
    navigator.clipboard.writeText(videoToMarkdown(video));
  }

  function handleDownload() {
    if (!video) return;
    const md = videoToMarkdown(video);
    const blob = new Blob([md], { type: "text/markdown" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${video.title.replace(/[^a-zA-Z0-9 ]/g, "").trim()}.md`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-6">
      <Link
        to="/"
        className="text-sm text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
      >
        &larr; Back to videos
      </Link>

      {/* Header */}
      <div className="flex gap-4">
        <a href={youtubeUrl} target="_blank" rel="noopener noreferrer">
          <img
            src={thumbnail}
            alt=""
            className="w-48 h-[108px] object-cover rounded shrink-0 bg-zinc-200 dark:bg-zinc-800"
          />
        </a>
        <div className="flex-1 min-w-0">
          <h2 className="text-xl font-semibold text-zinc-900 dark:text-zinc-100">
            {video.title || video.youtube_id}
          </h2>
          <div className="flex items-center gap-2 mt-1 text-sm text-zinc-500 dark:text-zinc-400">
            {video.channel && <span>{video.channel}</span>}
            {video.duration_seconds ? (
              <>
                <span>·</span>
                <span>{formatDuration(video.duration_seconds)}</span>
              </>
            ) : null}
            {video.language && (
              <>
                <span>·</span>
                <span>{video.language}</span>
              </>
            )}
            <span>·</span>
            <span>{new Date(video.created_at).toLocaleDateString()}</span>
          </div>
          <div className="flex items-center gap-2 mt-2">
            <a
              href={youtubeUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="px-3 py-1.5 text-xs bg-red-600 text-white rounded-md hover:bg-red-500 transition-colors"
            >
              Watch on YouTube
            </a>
            <span className="px-2 py-0.5 bg-zinc-100 dark:bg-zinc-800 text-zinc-500 dark:text-zinc-400 rounded text-xs">
              {video.detail_level}
            </span>
            {video.summary_provider && (
              <span className="px-2 py-0.5 bg-violet-100 dark:bg-violet-900/30 text-violet-700 dark:text-violet-400 rounded text-xs">
                {video.summary_provider}
                {video.summary_model && ` · ${video.summary_model}`}
              </span>
            )}
            {(video.summary_input_tokens ?? 0) > 0 && (
              <span className="px-2 py-0.5 bg-zinc-100 dark:bg-zinc-800 text-zinc-500 dark:text-zinc-400 rounded text-xs">
                {formatTokens(video.summary_input_tokens!)} in · {formatTokens(video.summary_output_tokens!)} out
              </span>
            )}
            {isProcessing && (
              <span className="px-2 py-0.5 bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 rounded text-xs">
                {video.status}...
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Processing indicator */}
      {isProcessing && (
        <div className="text-amber-600 dark:text-amber-400 text-sm bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900/50 rounded-lg p-3">
          Video is being processed. This page will update automatically.
        </div>
      )}

      {/* Summary */}
      {video.summary && (
        <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-zinc-900 dark:text-zinc-100">
              Summary
            </h3>
            <div className="flex gap-2">
              <button
                onClick={handleCopy}
                className="px-2.5 py-1 text-xs bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded hover:bg-zinc-200 dark:hover:bg-zinc-700 transition-colors"
              >
                Copy as Markdown
              </button>
              <button
                onClick={handleDownload}
                className="px-2.5 py-1 text-xs bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded hover:bg-zinc-200 dark:hover:bg-zinc-700 transition-colors"
              >
                Download .md
              </button>
            </div>
          </div>
          <div className="prose prose-zinc dark:prose-invert prose-sm max-w-none">
            <ReactMarkdown>{video.summary}</ReactMarkdown>
          </div>
        </div>
      )}

      {/* Key Points + Action Items */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {video.metadata?.key_points && video.metadata.key_points.length > 0 && (
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-3">
            <h3 className="font-medium text-zinc-900 dark:text-zinc-100">
              Key Points
            </h3>
            <ul className="space-y-2">
              {video.metadata.key_points.map((kp, i) => (
                <li
                  key={i}
                  className="text-sm text-zinc-600 dark:text-zinc-400 flex gap-2"
                >
                  <span className="text-cyan-500 shrink-0">-</span>
                  <span className="prose prose-zinc dark:prose-invert prose-sm prose-p:m-0 max-w-none"><ReactMarkdown>{kp}</ReactMarkdown></span>
                </li>
              ))}
            </ul>
          </div>
        )}
        {video.metadata?.action_items && video.metadata.action_items.length > 0 && (
          <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-3">
            <h3 className="font-medium text-zinc-900 dark:text-zinc-100">
              Action Items
            </h3>
            <ul className="space-y-2">
              {video.metadata.action_items.map((ai, i) => (
                <li
                  key={i}
                  className="text-sm text-zinc-600 dark:text-zinc-400 flex gap-2"
                >
                  <span className="text-emerald-500 shrink-0">-</span>
                  <span className="prose prose-zinc dark:prose-invert prose-sm prose-p:m-0 max-w-none"><ReactMarkdown>{ai}</ReactMarkdown></span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {/* Topics */}
      {video.metadata?.topics && video.metadata.topics.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          {video.metadata.topics.map((topic) => (
            <span
              key={topic}
              className="px-2.5 py-1 bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400 rounded-md text-sm"
            >
              {topic}
            </span>
          ))}
        </div>
      )}

      {/* Resummarize */}
      <div className="flex items-center gap-3 flex-wrap">
        <span className="text-sm text-zinc-500">Resummarize:</span>
        {providers && providers.providers.length > 1 && (
          <select
            value={resumProvider}
            onChange={(e) => setResumProvider(e.target.value)}
            className="px-2 py-1.5 text-sm bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded-md border-none focus:ring-2 focus:ring-cyan-500/50"
          >
            <option value="">Default ({providers.default})</option>
            {providers.providers.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        )}
        <select
          value={resumLang}
          onChange={(e) => setResumLang(e.target.value)}
          className="px-2 py-1.5 text-sm bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded-md border-none focus:ring-2 focus:ring-cyan-500/50"
        >
          <option value="">Auto ({video.language || "?"})</option>
          <option value="de">Deutsch</option>
          <option value="en">English</option>
          <option value="fr">Français</option>
          <option value="es">Español</option>
        </select>
        {["medium", "deep"].map((level) => (
          <button
            key={level}
            disabled={resummarize.isPending}
            onClick={() => resummarize.mutate(level)}
            className="px-3 py-1.5 text-sm bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded-md hover:bg-zinc-300 dark:hover:bg-zinc-700 transition-colors disabled:opacity-50"
          >
            {level}
          </button>
        ))}
        {resummarize.isPending && (
          <span className="text-sm text-zinc-500">Processing...</span>
        )}
        {resummarize.isSuccess && (
          <span className="text-sm text-emerald-500">Done!</span>
        )}
        {resummarize.isError && (
          <span className="text-sm text-red-500">
            {(resummarize.error as Error).message}
          </span>
        )}
      </div>

      {/* Delete */}
      <div className="flex items-center gap-3">
        {!confirmDelete ? (
          <button
            onClick={() => setConfirmDelete(true)}
            className="px-3 py-1.5 text-sm text-red-600 dark:text-red-400 border border-red-300 dark:border-red-900/50 rounded-md hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
          >
            Delete video
          </button>
        ) : (
          <>
            <span className="text-sm text-red-600 dark:text-red-400">
              Are you sure?
            </span>
            <button
              onClick={() => del.mutate()}
              disabled={del.isPending}
              className="px-3 py-1.5 text-sm bg-red-600 text-white rounded-md hover:bg-red-500 transition-colors disabled:opacity-50"
            >
              {del.isPending ? "Deleting..." : "Yes, delete"}
            </button>
            <button
              onClick={() => setConfirmDelete(false)}
              className="px-3 py-1.5 text-sm text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
            >
              Cancel
            </button>
          </>
        )}
        {del.isError && (
          <span className="text-sm text-red-500">
            {(del.error as Error).message}
          </span>
        )}
      </div>

      {/* Transcript */}
      {video.transcript && (
        <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg">
          <button
            onClick={() => setShowTranscript(!showTranscript)}
            className="w-full px-6 py-4 flex items-center justify-between text-left"
          >
            <h3 className="font-medium text-zinc-900 dark:text-zinc-100">
              Transcript
            </h3>
            <span className="text-zinc-400 text-sm">
              {showTranscript ? "Hide" : "Show"}
            </span>
          </button>
          {showTranscript && (
            <div className="px-6 pb-6">
              <div className="text-sm text-zinc-600 dark:text-zinc-400 whitespace-pre-wrap leading-relaxed max-h-96 overflow-y-auto">
                {video.transcript}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
