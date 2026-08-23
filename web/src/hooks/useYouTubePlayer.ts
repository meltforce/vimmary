import { useCallback, useEffect, useRef, useState } from "react";
import { loadYouTubeAPI, type YTPlayer } from "../youtube-iframe.ts";

/** Wraps a YouTube IFrame player: mounts it into `containerRef`, polls the
 * position four times a second while playing, and reports embed failures.
 * StrictMode double-mounts effects, so creation is guarded by a cancelled
 * flag and every path destroys the player it created. */
export function useYouTubePlayer(youtubeId: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<YTPlayer | null>(null);
  const [ready, setReady] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [embedError, setEmbedError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let player: YTPlayer | null = null;

    loadYouTubeAPI()
      .then((yt) => {
        if (cancelled || !containerRef.current) return;
        // The API replaces the given element with its iframe, so the player
        // gets a child of the container rather than the container itself —
        // unmounting then only has to empty the container.
        const host = document.createElement("div");
        containerRef.current.appendChild(host);
        player = new yt.Player(host, {
          videoId: youtubeId,
          playerVars: { playsinline: 1, rel: 0 },
          events: {
            onReady: () => {
              if (!cancelled) setReady(true);
            },
            onStateChange: (event) => {
              if (cancelled) return;
              setPlaying(event.data === yt.PlayerState.PLAYING);
              setCurrentTime(event.target.getCurrentTime());
            },
            onError: () => {
              // Codes 2/5/100 are broken or removed videos, 101/150 mean the
              // creator disabled embedding. All degrade the same way here.
              if (!cancelled) setEmbedError(true);
            },
          },
        });
        playerRef.current = player;
      })
      .catch(() => {
        if (!cancelled) setEmbedError(true);
      });

    return () => {
      cancelled = true;
      playerRef.current = null;
      player?.destroy();
      if (containerRef.current) containerRef.current.replaceChildren();
      setReady(false);
      setPlaying(false);
      setEmbedError(false);
    };
  }, [youtubeId]);

  useEffect(() => {
    if (!playing) return;
    const interval = setInterval(() => {
      const player = playerRef.current;
      if (player) setCurrentTime(player.getCurrentTime());
    }, 250);
    return () => clearInterval(interval);
  }, [playing]);

  const seekTo = useCallback((seconds: number) => {
    playerRef.current?.seekTo(seconds, true);
    setCurrentTime(seconds);
  }, []);

  return { containerRef, ready, playing, currentTime, embedError, seekTo };
}
