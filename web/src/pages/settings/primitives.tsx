import { useState, type CSSProperties, type ReactNode } from "react";
import { Skel } from "../../components/LoadingSkeleton.tsx";

/**
 * The building blocks every settings section is made of. They carry the tab's
 * layout — the heading block, the label column, the rule between rows — so that
 * a section file contains its own concern and nothing about how it looks.
 */

/** Tokens render in monospace and are truncated rather than wrapped. */
export const MONO: CSSProperties = {
  fontFamily: "var(--font-mono)",
  fontSize: 12.5,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

export function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="btn btn-ghost"
      style={{ fontSize: 12 }}
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
    >
      {copied ? "Copied" : label}
    </button>
  );
}

/** The head of a settings tab: h2, one explanatory sentence, a 2px rule. */
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
      <h2 style={{ fontSize: 22 }}>{title}</h2>
      <p
        style={{
          font: "400 13px/1.55 var(--font-body)",
          color: "var(--color-neutral-700)",
          maxWidth: "62ch",
          margin: "8px 0 0",
        }}
      >
        {subtitle}
      </p>
      <div style={{ borderTop: "var(--rule-strong)", marginTop: 14 }}>{children}</div>
    </section>
  );
}

/**
 * A label column and its value on the same baseline. Below 768px the row stacks
 * — `.set-row` handles that, so nothing here reads the viewport.
 */
export function Row({
  label,
  value,
  mono = false,
  children,
}: {
  label: string;
  value?: ReactNode;
  mono?: boolean;
  children?: ReactNode;
}) {
  return (
    <div className="set-row">
      <div className="kick">{label}</div>
      <div className="val">
        {value !== undefined && value !== null ? (
          <div style={mono ? MONO : { fontSize: 14 }}>{value}</div>
        ) : null}
        {children}
      </div>
    </div>
  );
}

/**
 * What a section renders when its own query fails. Each section owns its
 * queries, so one failing backend takes out one tab rather than the page —
 * before the split all four page-level queries shared a single
 * isLoading/errorObj conjunction, and any one of them failing replaced the whole
 * Settings page with an error box.
 */
export function SectionError({ error }: { error: Error }) {
  return (
    <div
      className="banner"
      style={{ paddingLeft: 0, paddingRight: 0, background: "transparent", borderTop: 0 }}
    >
      <span style={{ color: "var(--color-accent-700)" }}>{error.message}</span>
    </div>
  );
}

export function SectionLoading() {
  return (
    <div className="set-row">
      <div className="kick"><Skel w={90} h={10} /></div>
      <div className="val"><Skel w="60%" h={14} /></div>
    </div>
  );
}

/** 44×26, rust ground when on, and round — a deliberate reversal of the
 *  Modernist rule against the native rounded control: in a system built from
 *  pills and cards the square toggle was the last hard edge left. */
export function Switch({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      className="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span />
    </button>
  );
}

/** Square, 15×15, 2px accent border and accent fill when checked. */
export function Checkbox({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="checkbox"
      className="check"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    />
  );
}
