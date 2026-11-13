import { logout, type Features } from "./api.ts";

/**
 * Where sign-out lands. `signed_out` keeps the login page from handing straight
 * over to the OIDC provider, which signs a known browser back in without asking.
 */
export const SIGNED_OUT_URL = "/login?signed_out=1";

/**
 * Only a session can be ended. A request recognised through Tailscale, or an
 * instance without login, is signed in again by WhoIs on the next request, so
 * a sign-out control there would do nothing.
 */
export function canSignOut(auth: { enabled: boolean; method: Features["auth_method"] }): boolean {
  return auth.enabled && (auth.method === "password" || auth.method === "oidc");
}

/** Ends this browser's session and leaves for the login page. */
export async function signOut(): Promise<void> {
  await logout();
  window.location.assign(SIGNED_OUT_URL);
}
