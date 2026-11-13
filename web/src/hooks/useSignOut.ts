import { useMutation } from "@tanstack/react-query";
import { logout } from "../api.ts";

/**
 * Ends this browser's session and leaves for the login page. `signed_out`
 * keeps the login page from handing straight over to the OIDC provider, which
 * signs a known browser back in without asking.
 */
export function useSignOut() {
  return useMutation({
    mutationFn: logout,
    onSuccess: () => window.location.assign("/login?signed_out=1"),
  });
}
