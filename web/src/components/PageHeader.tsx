import type { ReactNode } from "react";

interface Props {
  /** The kicker line above the title — a date, a section, a meta line. */
  kicker: ReactNode;
  title: ReactNode;
  /** Range controls, onward links, destructive actions. */
  actions?: ReactNode;
}

/**
 * Kicker line, then the h1, with the actions right-aligned on the same
 * baseline. The sizes live in .page-head; only the composition is here.
 */
export default function PageHeader({ kicker, title, actions }: Props) {
  return (
    <div className="page-head">
      <div className="min-w-0">
        <div className="kick">{kicker}</div>
        <h1>{title}</h1>
      </div>
      {actions ? <div className="page-head-actions">{actions}</div> : null}
    </div>
  );
}
