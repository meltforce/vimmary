export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

export function videoToMarkdown(video: {
  title: string;
  channel: string;
  created_at: string;
  summary?: string;
  metadata?: { key_points?: string[]; action_items?: string[]; topics?: string[] };
}): string {
  const lines: string[] = [`# ${video.title}`, ""];
  lines.push(`Channel: ${video.channel}`);
  lines.push(`Date: ${new Date(video.created_at).toLocaleDateString()}`);
  lines.push("");

  if (video.summary) {
    lines.push("## Summary", "", video.summary, "");
  }

  if (video.metadata?.key_points?.length) {
    lines.push("## Key Points", "");
    for (const kp of video.metadata.key_points) {
      lines.push(`- ${kp}`);
    }
    lines.push("");
  }

  if (video.metadata?.action_items?.length) {
    lines.push("## Action Items", "");
    for (const ai of video.metadata.action_items) {
      lines.push(`- ${ai}`);
    }
    lines.push("");
  }

  if (video.metadata?.topics?.length) {
    lines.push("## Topics", "");
    lines.push(video.metadata.topics.join(", "), "");
  }

  return lines.join("\n");
}
