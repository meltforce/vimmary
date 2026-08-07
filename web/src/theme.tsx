import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type ThemePreference = "auto" | "light" | "dark";

const STORAGE_KEY = "vimmary.theme";

function readStored(): ThemePreference {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === "light" || raw === "dark" || raw === "auto") return raw;
  } catch {
    // localStorage is unavailable in private windows on some browsers.
  }
  return "auto";
}

interface ThemeContextValue {
  theme: ThemePreference;
  setTheme: (next: ThemePreference) => void;
}

const ThemeContext = createContext<ThemeContextValue>({
  theme: "auto",
  setTheme: () => {},
});

/**
 * Sets data-theme on <html>. The token sheet re-points every colour variable
 * under [data-theme="dark"], and under [data-theme="auto"] inside a
 * prefers-color-scheme block, so nothing else has to know which ground it is
 * on. The same attribute is applied before first paint by the inline script in
 * index.html; this provider only keeps it in step with the preference.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemePreference>(readStored);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  const setTheme = useCallback((next: ThemePreference) => {
    setThemeState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // The preference stays for this session only.
    }
  }, []);

  const value = useMemo(() => ({ theme, setTheme }), [theme, setTheme]);

  return <ThemeContext value={value}>{children}</ThemeContext>;
}

export function useTheme() {
  return useContext(ThemeContext);
}
