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
  summary_provider?: string;
  summary_model?: string;
  summary_input_tokens?: number;
  summary_output_tokens?: number;
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
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<ListResponse> {
  const params = new URLSearchParams();
  if (opts?.channel) params.set("channel", opts.channel);
  if (opts?.language) params.set("language", opts.language);
  if (opts?.topic) params.set("topic", opts.topic);
  if (opts?.status) params.set("status", opts.status);
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

export async function deleteVideo(id: string): Promise<void> {
  const res = await fetch(`/api/v1/videos/${id}`, { method: "DELETE" });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
}

export function resummarizeVideo(
  id: string,
  level: string,
  language?: string,
  provider?: string
): Promise<{ message: string; level: string }> {
  const payload: Record<string, string> = { level };
  if (language) payload.language = language;
  if (provider) payload.provider = provider;
  return fetchJSON(`/api/v1/videos/${id}/resummarize`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function submitVideo(
  url: string
): Promise<{ status: string; youtube_id: string }> {
  return fetchJSON("/api/v1/videos", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

export function retryVideo(
  id: string
): Promise<{ status: string }> {
  return fetchJSON(`/api/v1/videos/${id}/retry`, { method: "POST" });
}

export interface ProvidersInfo {
  providers: string[];
  default: string;
  models: Record<string, string>;
}

export interface ModelInfo {
  id: string;
  display_name: string;
}

export interface ModelsResponse {
  models: ModelInfo[];
  selected: string;
}

export function fetchProviders(): Promise<ProvidersInfo> {
  return fetchJSON("/api/v1/config/providers");
}

export function fetchStats(): Promise<VideoStats> {
  return fetchJSON("/api/v1/stats");
}

// Settings API

export interface WebhookInfo {
  token: string;
}

export interface KarakeepStatus {
  configured: boolean;
  base_url: string;
}

export interface ImportResult {
  total: number;
  imported: number;
  skipped: number;
}

export function fetchWebhookInfo(): Promise<WebhookInfo> {
  return fetchJSON("/api/v1/settings/webhook");
}

export function fetchKarakeepStatus(): Promise<KarakeepStatus> {
  return fetchJSON("/api/v1/settings/karakeep");
}

export function importKarakeepBookmarks(): Promise<ImportResult> {
  return fetchJSON("/api/v1/settings/karakeep/import", { method: "POST" });
}

export function setKarakeepAPIKey(
  apiKey: string
): Promise<{ status: string }> {
  return fetchJSON("/api/v1/settings/karakeep", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ api_key: apiKey }),
  });
}

export interface SummaryPromptsInfo {
  medium: string;
  deep: string;
  default_medium: string;
  default_deep: string;
}

export function fetchSummaryPrompts(): Promise<SummaryPromptsInfo> {
  return fetchJSON("/api/v1/settings/prompts");
}

export function setSummaryPrompt(
  level: string,
  prompt: string
): Promise<{ status: string }> {
  return fetchJSON("/api/v1/settings/prompts", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ level, prompt }),
  });
}

export function fetchModels(provider: string): Promise<ModelsResponse> {
  return fetchJSON(`/api/v1/config/models?provider=${encodeURIComponent(provider)}`);
}

export function fetchModelPreferences(): Promise<Record<string, string>> {
  return fetchJSON("/api/v1/settings/models");
}

export function setModel(
  provider: string,
  model: string
): Promise<{ status: string }> {
  return fetchJSON("/api/v1/settings/model", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, model }),
  });
}
