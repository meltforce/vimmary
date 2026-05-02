import { ReactNode } from "react";
import { NavLink } from "react-router-dom";

const navItems = [
  { to: "/", label: "Videos" },
  { to: "/stats", label: "Stats" },
  { to: "/settings", label: "Settings" },
];

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col">
      <header
        className="sticky top-0 z-10 backdrop-blur-sm"
        style={{
          background:
            "linear-gradient(to bottom, var(--vim-bg), color-mix(in oklch, var(--vim-bg) 92%, transparent))",
          borderBottom: "1px solid var(--vim-line-soft)",
        }}
      >
        <div
          className="flex items-center justify-between"
          style={{ padding: "18px 40px" }}
        >
          <div className="flex items-center" style={{ gap: 36 }}>
            <NavLink to="/" className="vim-brand-mark flex items-baseline" style={{ gap: 10 }}>
              <span>
                vimma<span className="r">r</span>y
              </span>
              <span className="vim-kicker" style={{ fontSize: 10.5 }}>
                youtube · read
              </span>
            </NavLink>
            <nav className="flex items-center" style={{ gap: 4 }}>
              {navItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === "/"}
                  className={({ isActive }) =>
                    `transition-colors ${isActive ? "vim-nav-active" : "vim-nav-link"}`
                  }
                  style={({ isActive }) => ({
                    padding: "8px 14px",
                    borderRadius: 999,
                    fontSize: 13,
                    color: isActive ? "var(--vim-ink)" : "var(--vim-ink-3)",
                    background: isActive ? "var(--vim-surface-2)" : "transparent",
                  })}
                >
                  {item.label}
                </NavLink>
              ))}
            </nav>
          </div>
          <div className="flex items-center" style={{ gap: 10 }}>
            <span className="vim-kbd">⌘ K</span>
          </div>
        </div>
      </header>
      <main className="flex-1 w-full">{children}</main>
    </div>
  );
}
