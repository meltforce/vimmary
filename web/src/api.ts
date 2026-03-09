// Types matching Go structs

export interface VideoMetadata {
  topics?: string[];
  key_points?: string[];
  action_items?: string[];
}

export interface Video {
  id: string;
  user_id: number;
  karakeep_bookmark_id?: string;
  youtube_id: string;
  title: string;
  channel: string;
  duration_seconds?: number;
  language?: string;
  transcript?: string;
  summary?: string;
  detail_level: string;
  metadata: VideoMetadata;
  status: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface HybridMatch {
  id: string;
  youtube_id: string;
  title: string;
  channel: string;
  summary: string;
  metadata: VideoMetadata;
  score: number;
  match_type: "keyword" | "semantic" | "both";
  created_at: string;
}

export interface SearchResponse {
  count: number;
  results: HybridMatch[];
  warnings?: string[];
}

export interface ListResponse {
  total: number;
  count: number;
  videos: Video[];
}

export interface ChannelCount {
  channel: string;
  count: number;
}

export interface TopicCount {
  topic: string;
  count: number;
}

export interface DailyCount {
  date: string;
  count: number;
}

export interface VideoStats {
  total_count: number;
  by_status: Record<string, number>;
  by_channel: ChannelCount[];
  top_topics: TopicCount[];
  daily_activity: DailyCount[];
}

// API functions

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  return res.json();
}

export function listVideos(opts?: {
  channel?: string;
  language?: string;
  topic?: string;
  limit?: number;
  offset?: number;
}): Promise<ListResponse> {
  const params = new URLSearchParams();
  if (opts?.channel) params.set("channel", opts.channel);
  if (opts?.language) params.set("language", opts.language);
  if (opts?.topic) params.set("topic", opts.topic);
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.offset) params.set("offset", String(opts.offset));
  const qs = params.toString();
  return fetchJSON(`/api/v1/videos${qs ? `?${qs}` : ""}`);
}

export function getVideo(id: string): Promise<Video> {
  return fetchJSON(`/api/v1/videos/${id}`);
}

export function searchVideos(
  query: string,
  limit?: number
): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query });
  if (limit) params.set("limit", String(limit));
  return fetchJSON(`/api/v1/search?${params}`);
}

export function resummarizeVideo(
  id: string,
  level: string
): Promise<{ message: string; level: string }> {
  return fetchJSON(`/api/v1/videos/${id}/resummarize`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ level }),
  });
}

export function fetchStats(): Promise<VideoStats> {
  return fetchJSON("/api/v1/stats");
}
