import { useEffect, useState } from "react";

type Theme = "dark" | "light";

function readTheme(): Theme {
  if (typeof window === "undefined") return "dark";
  const stored = localStorage.getItem("vim-theme");
  if (stored === "dark" || stored === "light") return stored;
  return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

function apply(theme: Theme) {
  const root = document.documentElement;
  root.classList.remove("dark", "light");
  root.classList.add(theme);
}

export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(readTheme);

  useEffect(() => {
    apply(theme);
    localStorage.setItem("vim-theme", theme);
  }, [theme]);

  const next: Theme = theme === "dark" ? "light" : "dark";

  return (
    <button
      onClick={() => setTheme(next)}
      title={`Switch to ${next} mode`}
      aria-label={`Switch to ${next} mode`}
      className="vim-btn ghost"
      style={{
        padding: "6px 8px",
        background: "transparent",
        color: "var(--vim-ink-3)",
        lineHeight: 0,
      }}
    >
      {theme === "dark" ? (
        // Sun icon (light mode toggle target)
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="4.5" />
          <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
        </svg>
      ) : (
        // Moon icon (dark mode toggle target)
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
        </svg>
      )}
    </button>
  );
}
