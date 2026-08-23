// Loader and minimal typings for YouTube's IFrame Player API.
//
// The script tag is the app's only external runtime dependency (see
// DECISIONS.md): a client that cannot reach youtube.com cannot play the video
// either way, and a load failure degrades to the plain transcript. The types
// are hand-written for the handful of members the player hook uses —
// @types/youtube would add a dependency for a fraction of its surface.

export interface YTPlayer {
  seekTo(seconds: number, allowSeekAhead: boolean): void;
  getCurrentTime(): number;
  getPlayerState(): number;
  destroy(): void;
}

export interface YTPlayerEvent {
  target: YTPlayer;
  data: number;
}

interface YTPlayerConstructor {
  new (
    element: HTMLElement,
    options: {
      videoId: string;
      playerVars?: Record<string, string | number>;
      events?: {
        onReady?: (event: YTPlayerEvent) => void;
        onStateChange?: (event: YTPlayerEvent) => void;
        onError?: (event: YTPlayerEvent) => void;
      };
    },
  ): YTPlayer;
}

interface YTNamespace {
  Player: YTPlayerConstructor;
  PlayerState: { PLAYING: number };
}

declare global {
  interface Window {
    YT?: YTNamespace;
    onYouTubeIframeAPIReady?: () => void;
  }
}

let loading: Promise<YTNamespace> | null = null;

/** Loads the IFrame API once per page and resolves with the YT namespace.
 * Subsequent calls share the same promise. */
export function loadYouTubeAPI(): Promise<YTNamespace> {
  if (loading) return loading;
  loading = new Promise<YTNamespace>((resolve, reject) => {
    if (window.YT?.Player) {
      resolve(window.YT);
      return;
    }
    window.onYouTubeIframeAPIReady = () => {
      if (window.YT) resolve(window.YT);
    };
    const script = document.createElement("script");
    script.src = "https://www.youtube.com/iframe_api";
    script.onerror = () => {
      loading = null;
      reject(new Error("YouTube player failed to load"));
    };
    document.head.appendChild(script);
  });
  return loading;
}
