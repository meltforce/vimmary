import { useState, ReactNode } from "react";

/**
 * The three building blocks every settings section is made of. They carry the
 * page's layout — the two-column grid, the card, the divider between rows — so
 * that a section file contains its own concern and nothing about how the page
 * looks.
 */

export function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
      className="vim-btn ghost"
      style={{ padding: "6px 12px", fontSize: 12 }}
    >
      {copied ? "Copied ✓" : label}
    </button>
  );
}

export function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <section style={{ marginBottom: 40 }}>
      <div className="vim-grid-settings">
        <div>
          <h3
            style={{
              fontFamily: "var(--font-serif)",
              fontSize: 20,
              fontWeight: 500,
              margin: "0 0 6px",
              letterSpacing: "-0.01em",
              color: "var(--vim-ink)",
            }}
          >
            {title}
          </h3>
          <p
            style={{
              fontSize: 12.5,
              color: "var(--vim-ink-3)",
              margin: 0,
              lineHeight: 1.5,
            }}
          >
            {subtitle}
          </p>
        </div>
        <div
          style={{
            background: "var(--vim-surface)",
            borderRadius: 12,
            border: "1px solid var(--vim-line-soft)",
            padding: "4px 20px",
          }}
        >
          {children}
        </div>
      </div>
    </section>
  );
}

export function Row({
  label,
  value,
  mono = false,
  truncate = true,
  isLast = false,
  children,
}: {
  label: string;
  value?: ReactNode;
  mono?: boolean;
  truncate?: boolean;
  isLast?: boolean;
  children?: ReactNode;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "16px 0",
        borderBottom: isLast ? "none" : "1px solid var(--vim-line-soft)",
        gap: 16,
      }}
    >
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
          {label}
        </div>
        {value && (
          <div
            style={{
              fontFamily: mono ? "var(--font-mono)" : undefined,
              fontSize: mono ? 12.5 : 14,
              color: "var(--vim-ink)",
              overflow: truncate ? "hidden" : undefined,
              textOverflow: truncate ? "ellipsis" : undefined,
              whiteSpace: truncate ? "nowrap" : undefined,
            }}
          >
            {value}
          </div>
        )}
      </div>
      {children && <div style={{ flexShrink: 0 }}>{children}</div>}
    </div>
  );
}

/**
 * SectionError is what a section renders when its own query fails. Each section
 * owns its queries, so one failing backend takes out one card rather than the
 * page — before the split all four page-level queries shared a single
 * isLoading/errorObj conjunction, and any one of them failing replaced the whole
 * Settings page with an error box.
 */
export function SectionError({ error }: { error: Error }) {
  return (
    <div style={{ padding: "16px 0", fontSize: 13, color: "var(--vim-err)" }}>
      {error.message}
    </div>
  );
}

export function SectionLoading({ what }: { what: string }) {
  return (
    <div style={{ padding: "16px 0", fontSize: 13, color: "var(--vim-ink-3)" }}>
      Loading {what}…
    </div>
  );
}
