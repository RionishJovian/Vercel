import { createContext, use, useCallback, useMemo, useState, type ReactNode } from "react";

export type Theme = "night" | "day";
interface ThemeCtx { theme: Theme; setTheme: (t: Theme) => void }
const Ctx = createContext<ThemeCtx | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, set] = useState<Theme>(() => (document.documentElement.dataset.theme === "day" ? "day" : "night"));
  const setTheme = useCallback((t: Theme) => {
    set(t); document.documentElement.dataset.theme = t;
    try { localStorage.setItem("mm_theme", t); } catch { /* ignore */ }
    document.querySelector('meta[name="theme-color"]')?.setAttribute("content", t === "day" ? "#e4efe9" : "#04120f");
  }, []);
  const value = useMemo(() => ({ theme, setTheme }), [theme, setTheme]);
  return <Ctx value={value}>{children}</Ctx>;
}
export function useTheme(): ThemeCtx {
  const v = use(Ctx);
  if (!v) throw new Error("ThemeProvider missing");
  return v;
}
