import { afterEach, describe, expect, it, vi } from "vitest";
import { canSignOut, signOut, SIGNED_OUT_URL } from "./signout.ts";

describe("canSignOut", () => {
  it.each([
    ["password session", { enabled: true, method: "password" as const }, true],
    ["OIDC session", { enabled: true, method: "oidc" as const }, true],
    // WhoIs signs the request in again on the next call: nothing to end.
    ["Tailscale WhoIs", { enabled: true, method: "tailscale" as const }, false],
    ["personal access token", { enabled: true, method: "token" as const }, false],
    ["no auth: block", { enabled: false, method: "" as const }, false],
  ])("%s → %s", (_, auth, want) => {
    expect(canSignOut(auth)).toBe(want);
  });
});

describe("signOut", () => {
  afterEach(() => vi.unstubAllGlobals());

  function stub(status: number) {
    const fetch = vi.fn(async () => new Response(status === 204 ? null : "{}", { status }));
    const assign = vi.fn();
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("window", { location: { assign, pathname: "/" } });
    return { fetch, assign };
  }

  it("ends the session and leaves for the login page without auto sign-in", async () => {
    const { fetch, assign } = stub(204);
    await signOut();
    expect(fetch).toHaveBeenCalledWith("/auth/logout", { method: "POST" });
    expect(assign).toHaveBeenCalledWith(SIGNED_OUT_URL);
    expect(SIGNED_OUT_URL).toBe("/login?signed_out=1");
  });

  it("stays on the page when the server refuses", async () => {
    const { assign } = stub(500);
    await expect(signOut()).rejects.toThrow();
    expect(assign).not.toHaveBeenCalled();
  });
});
