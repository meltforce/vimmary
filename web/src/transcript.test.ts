import { describe, expect, it } from "vitest";
import { findActiveIndex, formatTimestamp, searchSegments } from "./transcript.ts";
import type { TranscriptSegment } from "./api.ts";

const segments: TranscriptSegment[] = [
  { s: 0, d: 2.5, t: "Hello and welcome" },
  { s: 2.5, d: 3, t: "to this Video about Go" },
  { s: 5.5, d: 4, t: "concurrency patterns" },
  { s: 9.5, d: 2, t: "and channels in go" },
];

describe("findActiveIndex", () => {
  it("is -1 before the first segment", () => {
    expect(findActiveIndex(segments, -1)).toBe(-1);
  });

  it("matches a start boundary exactly", () => {
    expect(findActiveIndex(segments, 2.5)).toBe(1);
  });

  it("stays on the running segment between starts", () => {
    expect(findActiveIndex(segments, 4)).toBe(1);
  });

  it("sticks to the last segment past the end", () => {
    expect(findActiveIndex(segments, 100)).toBe(3);
  });

  it("is -1 for an empty transcript", () => {
    expect(findActiveIndex([], 5)).toBe(-1);
  });
});

describe("searchSegments", () => {
  it("matches case-insensitively", () => {
    expect(searchSegments(segments, "GO")).toEqual([1, 3]);
  });

  it("returns nothing for no match", () => {
    expect(searchSegments(segments, "rust")).toEqual([]);
  });

  it("treats a blank query as no query", () => {
    expect(searchSegments(segments, "   ")).toEqual([]);
  });
});

describe("formatTimestamp", () => {
  it("renders m:ss under an hour", () => {
    expect(formatTimestamp(59)).toBe("0:59");
    expect(formatTimestamp(61)).toBe("1:01");
  });

  it("renders h:mm:ss above an hour", () => {
    expect(formatTimestamp(3600 + 2 * 60 + 3)).toBe("1:02:03");
  });

  it("floors fractional seconds and clamps negatives", () => {
    expect(formatTimestamp(12.9)).toBe("0:12");
    expect(formatTimestamp(-3)).toBe("0:00");
  });
});
