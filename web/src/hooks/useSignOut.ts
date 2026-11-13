import { useMutation } from "@tanstack/react-query";
import { signOut } from "../signout.ts";

/** The sign-out action of the nav and of Settings → Account. */
export function useSignOut() {
  return useMutation({ mutationFn: signOut });
}
