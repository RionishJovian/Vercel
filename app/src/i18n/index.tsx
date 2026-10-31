import { createContext, use, useCallback, useMemo, useState, type ReactNode } from "react";
import base from "./dict.json";
import extra from "./extra";

export type Lang = "my" | "en";
const dict: Record<Lang, Record<string, string>> = {
  en: { ...(base as { en: Record<string, string> }).en, ...extra.en },
  my: { ...(base as { my: Record<string, string> }).my, ...extra.my },
};
const KEY = "mm2_lang";
const initial = (): Lang => { try { const s = localStorage.getItem(KEY); if (s === "en" || s === "my") return s; } catch { /* storage blocked */ } return "my"; };

interface I18n { lang: Lang; setLang: (l: Lang) => void; t: (key: string) => string }
const Ctx = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, set] = useState<Lang>(() => { const l = initial(); document.documentElement.lang = l; return l; });
  const setLang = useCallback((l: Lang) => {
    set(l); document.documentElement.lang = l;
    try { localStorage.setItem(KEY, l); } catch { /* ignore */ }
  }, []);
  const value = useMemo<I18n>(() => ({ lang, setLang, t: (k) => dict[lang][k] ?? dict.en[k] ?? k }), [lang, setLang]);
  return <Ctx value={value}>{children}</Ctx>;
}
export function useI18n(): I18n {
  const v = use(Ctx);
  if (!v) throw new Error("I18nProvider missing");
  return v;
}
