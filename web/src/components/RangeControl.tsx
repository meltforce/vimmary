import { useEffect, useId, useState, type ReactNode } from "react";
import { useIsDesktop } from "../hooks/useMediaQuery.ts";

interface Props<T extends string> {
  options: readonly T[];
  value: T;
  onChange: (next: T) => void;
  /** Label for the sheet header and the collapsed chip's group. */
  label: string;
  /** Long form shown in the mobile sheet; falls back to the option itself. */
  longLabel?: Record<string, string>;
  /** Radio group name; must be unique when two controls share a page. */
  name?: string;
  /** Shown inside the mobile sheet under the options. */
  note?: ReactNode;
}

/**
 * A real radio group, not buttons. Below the phone breakpoint a segmented
 * control does not fit, so it collapses to one chip that opens a sheet.
 */
export default function RangeControl<T extends string>({
  options,
  value,
  onChange,
  label,
  longLabel,
  name,
  note,
}: Props<T>) {
  const isDesktop = useIsDesktop();
  const generatedName = useId();
  const groupName = name ?? generatedName;
  const [sheetOpen, setSheetOpen] = useState(false);

  // The sheet has no reason to stay open once the layout switches to desktop.
  useEffect(() => {
    if (isDesktop) setSheetOpen(false);
  }, [isDesktop]);

  const long = (opt: string) => longLabel?.[opt] ?? opt;

  if (isDesktop) {
    return (
      <span className="seg">
        {options.map((opt) => (
          <label key={opt} className="seg-opt">
            <input
              type="radio"
              name={groupName}
              value={opt}
              checked={value === opt}
              onChange={() => onChange(opt)}
            />
            {opt}
          </label>
        ))}
      </span>
    );
  }

  return (
    <>
      <button
        type="button"
        className="chip"
        aria-pressed="true"
        onClick={() => setSheetOpen(true)}
      >
        {long(value)}
      </button>

      {sheetOpen ? (
        <div className="sheet-backdrop" onClick={() => setSheetOpen(false)}>
          <div className="sheet" onClick={(e) => e.stopPropagation()}>
            <div className="kick page-x" style={{ paddingTop: 14, paddingBottom: 6 }}>
              {label}
            </div>
            {options.map((opt) => (
              <button
                key={opt}
                type="button"
                className="row w-full text-left"
                aria-current={value === opt}
                style={{
                  background: value === opt ? "var(--color-accent-100)" : "transparent",
                  fontWeight: value === opt ? 600 : 400,
                  minHeight: 48,
                }}
                onClick={() => {
                  onChange(opt);
                  setSheetOpen(false);
                }}
              >
                <span className="flex-1" style={{ fontSize: 14 }}>
                  {long(opt)}
                </span>
                <span className="num kick">{opt}</span>
              </button>
            ))}
            {note ? (
              <p
                className="page-x"
                style={{
                  font: "400 12px/1.5 var(--font-body)",
                  color: "var(--color-neutral-700)",
                  padding: "14px 0",
                  margin: 0,
                }}
              >
                {note}
              </p>
            ) : null}
            <div className="page-x" style={{ paddingTop: 6, paddingBottom: 20 }}>
              <button
                type="button"
                className="btn btn-secondary btn-block btn-center"
                onClick={() => setSheetOpen(false)}
              >
                Close
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
