/**
 * Placeholder blocks the exact height of the text they stand in for, so
 * nothing reflows when the data arrives. No spinner and no "Loading…": the
 * rules, the table head and the group bands paint immediately and only the
 * values are placeholders.
 */

export function Skel({ w, h = 14 }: { w: number | string; h?: number }) {
  return <span className="skel" style={{ width: w, height: h }} />;
}

/** Rows for the phone list and for any single-column stack of records. */
export default function LoadingSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div style={{ borderTop: "var(--rule-strong)" }}>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="row">
          <div style={{ flex: 1, minWidth: 0 }}>
            <Skel w={`${70 - (i % 3) * 12}%`} />
            <div style={{ marginTop: 6 }}>
              <Skel w={140} h={11} />
            </div>
          </div>
          <Skel w={56} h={18} />
        </div>
      ))}
    </div>
  );
}
