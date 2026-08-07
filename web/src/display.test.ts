import { describe, expect, it } from "vitest";
import { excerpt } from "./display.ts";

/**
 * One case per shape a stored summary actually opens with. All three bodies are
 * taken from real summaries — the scaffolding is what the feed row must not
 * show, and it differs by provider and by detail level.
 */
describe("excerpt", () => {
  it("drops an ATX heading that restates the title", () => {
    const summary = [
      "## Summary of ONCE: Run multi dockerized web apps on single server",
      "",
      "ONCE is a platform designed to simplify the deployment and management of",
      "multiple Dockerized web applications on a single server.",
      "",
      "## Key points",
      "",
      "- Ownership over subscription",
    ].join("\n");

    expect(excerpt(summary, "ONCE: Run multi dockerized web apps on single server")).toBe(
      "ONCE is a platform designed to simplify the deployment and management of multiple Dockerized web applications on a single server.",
    );
  });

  it("drops a plain leading line that restates the title", () => {
    const summary = [
      "Chapter-by-Chapter Summary of Mozilla Wants You Back.",
      "",
      "Mozilla's pivot toward **AI** has drawn sustained criticism from the",
      "community that carried `Firefox` through its lean years.",
    ].join("\n");

    expect(excerpt(summary, "Mozilla Wants You Back")).toBe(
      "Mozilla's pivot toward AI has drawn sustained criticism from the community that carried Firefox through its lean years.",
    );
  });

  it("keeps a summary that starts straight into prose", () => {
    const summary =
      "A grill-brush company tries to manufacture entirely inside the United\nStates and discovers which parts of the supply chain no longer exist there.";

    expect(excerpt(summary, "The Puzzle of the All-American BBQ Scrubber")).toBe(
      "A grill-brush company tries to manufacture entirely inside the United States and discovers which parts of the supply chain no longer exist there.",
    );
  });

  it("steps over a fenced block and resolves link syntax to its text", () => {
    const summary = [
      "```json",
      "{ \"topics\": [\"docker\"] }",
      "```",
      "",
      "The talk closes on [the ONCE licence](https://once.com), which the speaker",
      "contrasts with per-seat pricing.",
    ].join("\n");

    expect(excerpt(summary, "Anything")).toBe(
      "The talk closes on the ONCE licence, which the speaker contrasts with per-seat pricing.",
    );
  });

  it("returns an empty string when nothing but scaffolding is left", () => {
    expect(excerpt("## Summary of X", "X")).toBe("");
    expect(excerpt(undefined, "X")).toBe("");
    expect(excerpt("", "X")).toBe("");
  });
});
