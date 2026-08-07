import { useEffect, useState, type ReactNode } from "react";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";

export interface ToastMessage {
  /** Changing the id restarts the dismiss timer for a repeated message. */
  id: number;
  text: ReactNode;
  action?: { label: string; onClick: () => void };
}

const DISMISS_MS = 6_000;

/**
 * Bottom left, above the tab bar. One at a time, never a stack: two overlapping
 * confirmations say less than the last one alone.
 */
export default function Toast({
  message,
  onDismiss,
}: {
  message: ToastMessage | null;
  onDismiss: () => void;
}) {
  const isDesktop = useIsDesktop();

  useEffect(() => {
    if (!message) return;
    const t = setTimeout(onDismiss, DISMISS_MS);
    return () => clearTimeout(t);
  }, [message, onDismiss]);

  if (!message) return null;

  return (
    <div
      role="status"
      className="toast"
      style={{
        position: "fixed",
        left: "var(--page-x)",
        // Clears the tab bar on the phone; sits on the page edge on desktop.
        bottom: isDesktop ? 24 : "calc(96px + env(safe-area-inset-bottom))",
        zIndex: 25,
        maxWidth: "calc(100vw - 2 * var(--page-x))",
      }}
    >
      <span style={{ minWidth: 0 }}>{message.text}</span>
      {message.action ? (
        <button
          type="button"
          className="btn btn-ghost"
          onClick={() => {
            message.action?.onClick();
            onDismiss();
          }}
        >
          {message.action.label}
        </button>
      ) : null}
    </div>
  );
}

/** Holds the single visible toast and hands out a `show(text, action?)`. */
export function useToast() {
  const [message, setMessage] = useState<ToastMessage | null>(null);
  return {
    message,
    dismiss: () => setMessage(null),
    show: (text: ReactNode, action?: ToastMessage["action"]) =>
      setMessage({ id: Date.now(), text, action }),
  };
}
