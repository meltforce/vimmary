// Types matching Go structs

export interface VideoMetadata {
  topics?: string[];
  key_points?: string[];
  action_items?: string[];
}

export type ContentSource = "youtube" | "podcast";

export interface Video {
  id: string;
  user_id: number;
  karakeep_bookmark_id?: string;
  youtube_id: string;
  source: ContentSource;
  external_id: string;
  source_url?: string;
  source_feed_id?: string;
  thumbnail_url?: string;
  published_at?: string;
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
  source: ContentSource;
  source_url?: string;
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
  total_duration_seconds: number;
  by_status: Record<string, number>;
  by_source: Record<string, number>;
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

// The server defaults `source` to "youtube", so a call without it stays
// video-only. Pass "podcast" or "all" to widen it.
export function listVideos(opts?: {
  channel?: string;
  /** Matches the stored channel value verbatim — what the facet controls send,
   * where `channel` is an ILIKE partial match. */
  channelExact?: string;
  language?: string;
  topic?: string;
  status?: string;
  source?: ContentSource | "all";
  limit?: number;
  offset?: number;
}): Promise<ListResponse> {
  const params = new URLSearchParams();
  if (opts?.channel) params.set("channel", opts.channel);
  if (opts?.channelExact) params.set("channel_exact", opts.channelExact);
  if (opts?.language) params.set("language", opts.language);
  if (opts?.topic) params.set("topic", opts.topic);
  if (opts?.status) params.set("status", opts.status);
  if (opts?.source) params.set("source", opts.source);
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.offset) params.set("offset", String(opts.offset));
  const qs = params.toString();
  return fetchJSON(`/api/v1/videos${qs ? `?${qs}` : ""}`);
}

export function getVideo(id: string): Promise<Video> {
  return fetchJSON(`/api/v1/videos/${id}`);
}

/** One timed caption line. Compact keys because a long video carries
 * thousands: s = start seconds, d = duration seconds, t = text. */
export interface TranscriptSegment {
  s: number;
  d: number;
  t: string;
}

export interface SegmentsResponse {
  /** False for podcast rows, Voxtral-transcribed rows and videos InnerTube
   * has no captions for — the plain transcript is all there is. */
  available: boolean;
  segments: TranscriptSegment[];
}

/** The server fetches and stores segments on first use, so this call can take
 * an InnerTube round-trip for videos summarized before segments existed. */
export function getVideoSegments(id: string): Promise<SegmentsResponse> {
  return fetchJSON(`/api/v1/videos/${id}/segments`);
}

export function searchVideos(
  query: string,
  limit?: number,
  source?: ContentSource | "all"
): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query });
  if (limit) params.set("limit", String(limit));
  if (source) params.set("source", source);
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

export function retryAllFailed(): Promise<{ retried: number }> {
  return fetchJSON("/api/v1/videos/retry-all", { method: "POST" });
}

export function transcribeVideo(
  id: string
): Promise<{ status: string }> {
  return fetchJSON(`/api/v1/videos/${id}/transcribe`, { method: "POST" });
}

export function transcribeAllNoCaptions(): Promise<{ transcribing: number }> {
  return fetchJSON("/api/v1/videos/transcribe-all", { method: "POST" });
}

export interface TopicConsolidation {
  before: number;
  after: number;
  merged: number;
  updated_videos: number;
  mapping: Record<string, string>;
}

/** Asks the configured LLM to merge near-duplicate topic tags across the
 * whole library and applies the mapping. One model call; synchronous. */
export function consolidateTopics(): Promise<TopicConsolidation> {
  return fetchJSON("/api/v1/videos/consolidate-topics", { method: "POST" });
}

export interface ProvidersInfo {
  providers: string[];
  default: string;
  selected_provider: string;
  selected_model: string;
}

export interface ModelInfo {
  id: string;
  display_name: string;
  provider: string;
}

export interface ModelsResponse {
  models: ModelInfo[];
  selected_provider: string;
  selected_model: string;
}

export function fetchProviders(): Promise<ProvidersInfo> {
  return fetchJSON("/api/v1/config/providers");
}

/** Which optional integrations this deployment has configured. */
export interface Features {
  podcasts: boolean;
  cast2md_url: string;
  // True for the primary user — the first Tailscale login, which the server
  // also treats as the owner that tagged devices resolve to. Gates the
  // service-wide LLM settings.
  is_admin: boolean;
}

export function fetchFeatures(): Promise<Features> {
  return fetchJSON("/api/v1/config/features");
}

/** The library's navigable dimensions: channels and LLM topics of completed
 * rows, with counts. */
export interface VideoFacets {
  channels: ChannelCount[];
  topics: TopicCount[];
}

export function fetchVideoFacets(source?: ContentSource | "all"): Promise<VideoFacets> {
  const params = new URLSearchParams();
  if (source) params.set("source", source);
  const qs = params.toString();
  return fetchJSON(`/api/v1/videos/facets${qs ? `?${qs}` : ""}`);
}

export function fetchStats(source?: ContentSource | "all"): Promise<VideoStats> {
  const params = new URLSearchParams();
  if (source) params.set("source", source);
  const qs = params.toString();
  return fetchJSON(`/api/v1/stats${qs ? `?${qs}` : ""}`);
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

export interface FeedInfo {
  token: string;
  urls: {
    videos: string;
    podcasts: string;
    all: string;
  };
}

export function fetchFeedInfo(): Promise<FeedInfo> {
  return fetchJSON("/api/v1/settings/feed");
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

export interface LLMSettings {
  mistral_configured: boolean;
  anthropic_configured: boolean;
  provider: string;
}

/** Every field is optional: omitting one leaves it unchanged, sending an empty
 * string clears it. That is how the Anthropic key is removed again. */
export interface LLMSettingsUpdate {
  mistral_api_key?: string;
  anthropic_api_key?: string;
  provider?: string;
}

export function fetchLLMSettings(): Promise<LLMSettings> {
  return fetchJSON("/api/v1/settings/llm");
}

export function updateLLMSettings(
  update: LLMSettingsUpdate
): Promise<LLMSettings> {
  return fetchJSON("/api/v1/settings/llm", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(update),
  });
}

export interface SummaryPromptsInfo {
  source: ContentSource;
  medium: string;
  deep: string;
  default_medium: string;
  default_deep: string;
}

export function fetchSummaryPrompts(
  source: ContentSource = "youtube"
): Promise<SummaryPromptsInfo> {
  return fetchJSON(`/api/v1/settings/prompts?source=${source}`);
}

export function setSummaryPrompt(
  source: ContentSource,
  level: string,
  prompt: string
): Promise<{ status: string }> {
  return fetchJSON("/api/v1/settings/prompts", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source, level, prompt }),
  });
}

// Podcasts

export interface PodcastFeed {
  feed_id: string;
  title: string;
  image_url?: string;
  episode_count: number;
  /** Episodes cast2md has a transcript for — what "Summarize all" would take. */
  completed_count: number;
  /** Episodes cast2md could still transcribe — what "Transcribe all" would queue. */
  transcribable_count: number;
  subscribed: boolean;
  detail_level: string;
  initial_backfill: number;
  initialized: boolean;
  summarized_count: number;
  last_polled_at?: string;
  last_error?: string;
}

export interface BatchResult {
  queued: number;
  skipped: number;
  message?: string;
}

export interface PodcastFeedsResponse {
  count: number;
  feeds: PodcastFeed[];
  cast2md_url: string;
}

export interface EpisodePreview {
  episode_id: number;
  feed_id: string;
  feed_title: string;
  title: string;
  description?: string;
  duration_seconds?: number;
  published_at?: string;
  status: string;
  image_url?: string;
  source_url: string;
  existing_id?: string;
  existing_status?: string;
}

export function listPodcastFeeds(): Promise<PodcastFeedsResponse> {
  return fetchJSON("/api/v1/podcasts/feeds");
}

export function setPodcastSubscription(
  feedID: string,
  enabled: boolean,
  detailLevel: string,
  initialBackfill?: number
): Promise<unknown> {
  return fetchJSON(`/api/v1/podcasts/feeds/${feedID}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      enabled,
      detail_level: detailLevel,
      // Omitted rather than null, so the server keeps the stored value.
      ...(initialBackfill === undefined ? {} : { initial_backfill: initialBackfill }),
    }),
  });
}

/** Summarize every episode cast2md already has a transcript for. */
export function summarizeAllPodcastFeed(feedID: string): Promise<BatchResult> {
  return fetchJSON(`/api/v1/podcasts/feeds/${feedID}/summarize-all`, { method: "POST" });
}

/** Ask cast2md to download and transcribe the rest of a feed. */
export function transcribeAllPodcastFeed(feedID: string): Promise<BatchResult> {
  return fetchJSON(`/api/v1/podcasts/feeds/${feedID}/transcribe-all`, { method: "POST" });
}

export function backfillPodcastFeed(
  feedID: string,
  limit: number
): Promise<{ queued: number; skipped: number }> {
  return fetchJSON(`/api/v1/podcasts/feeds/${feedID}/backfill`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ limit }),
  });
}

export function getEpisodePreview(episodeID: number): Promise<EpisodePreview> {
  return fetchJSON(`/api/v1/podcasts/episodes/${episodeID}`);
}

export function submitEpisode(
  episodeID: number,
  level?: string
): Promise<Video> {
  return fetchJSON("/api/v1/podcasts/episodes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ episode_id: episodeID, level }),
  });
}

// Channels inbox

export interface ChannelSubscription {
  id: number;
  user_id: number;
  channel_id: string;
  title: string;
  thumbnail_url?: string;
  enabled: boolean;
  last_polled_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
  /** Items still awaiting triage; filled by the list endpoint. */
  new_count: number;
}

export interface InboxItem {
  id: number;
  subscription_id: number;
  user_id: number;
  youtube_id: string;
  title: string;
  published_at?: string;
  state: "new" | "queued" | "dismissed";
  created_at: string;
  updated_at: string;
  channel_title: string;
}

export interface ChannelsResponse {
  count: number;
  channels: ChannelSubscription[];
}

export interface InboxResponse {
  total: number;
  count: number;
  items: InboxItem[];
}

export function listChannels(): Promise<ChannelsResponse> {
  return fetchJSON("/api/v1/channels");
}

/** Accepts a channel URL, an @handle or a bare handle. Subscribing runs the
 * first poll in place, so the inbox is filled when this resolves. */
export function addChannel(url: string): Promise<ChannelSubscription> {
  return fetchJSON("/api/v1/channels", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

/** Follows every channel in a pasted Google Takeout subscriptions.csv. The
 * inboxes fill through a background poll pass, not inside this request. */
export function importChannels(csv: string): Promise<{ imported: number; skipped: number }> {
  return fetchJSON("/api/v1/channels/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ csv }),
  });
}

export function setChannelEnabled(id: number, enabled: boolean): Promise<{ status: string }> {
  return fetchJSON(`/api/v1/channels/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
}

export function deleteChannel(id: number): Promise<{ status: string }> {
  return fetchJSON(`/api/v1/channels/${id}`, { method: "DELETE" });
}

export function listInbox(opts?: {
  subscriptionId?: number;
  limit?: number;
  offset?: number;
}): Promise<InboxResponse> {
  const params = new URLSearchParams();
  if (opts?.subscriptionId) params.set("subscription_id", String(opts.subscriptionId));
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.offset) params.set("offset", String(opts.offset));
  const qs = params.toString();
  return fetchJSON(`/api/v1/inbox${qs ? `?${qs}` : ""}`);
}

/** Sends the video through the normal pipeline and returns its library row —
 * "Watch" navigates to it, "Summarize" stays. One backend action either way. */
export function summarizeInboxItem(id: number): Promise<Video> {
  return fetchJSON(`/api/v1/inbox/${id}/summarize`, { method: "POST" });
}

export function dismissInboxItem(id: number): Promise<{ status: string }> {
  return fetchJSON(`/api/v1/inbox/${id}/dismiss`, { method: "POST" });
}

export function dismissAllInbox(): Promise<{ dismissed: number }> {
  return fetchJSON("/api/v1/inbox/dismiss-all", { method: "POST" });
}

export function fetchModels(): Promise<ModelsResponse> {
  return fetchJSON("/api/v1/config/models");
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
