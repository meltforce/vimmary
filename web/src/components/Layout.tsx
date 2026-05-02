import { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import ThemeToggle from "./ThemeToggle.tsx";

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
        <div className="vim-topbar-inner">
          <div className="vim-topbar-left">
            <NavLink to="/" className="vim-brand-mark flex items-baseline" style={{ gap: 10 }}>
              <span>
                vimma<span className="r">r</span>y
              </span>
              <span
                className="vim-kicker vim-brand-tag-mobile-hide"
                style={{ fontSize: 10.5 }}
              >
                youtube · read
              </span>
            </NavLink>
            <nav className="flex items-center" style={{ gap: 4 }}>
              {navItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === "/"}
                  className="transition-colors"
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
          <div className="vim-topbar-right">
            <ThemeToggle />
            <span className="vim-kbd vim-brand-tag-mobile-hide">⌘ K</span>
          </div>
        </div>
      </header>
      <main className="flex-1 w-full">{children}</main>
    </div>
  );
}
