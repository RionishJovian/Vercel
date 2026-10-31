import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { api, ApiError } from "../lib/api";
import { useI18n } from "../i18n";

type Turn = { role: "user" | "assistant"; content: string };

const PROMPTS = ["md.p1", "md.p2", "md.p3"] as const;

/** Floating advice-only assistant. The API is read-only: it can describe the fleet but never changes it. */
export function MagicDude() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [turns, setTurns] = useState<Turn[]>([]);
  const [text, setText] = useState("");
  const [pending, setPending] = useState(false);
  const [note, setNote] = useState("");
  const list = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);

  useEffect(() => { if (open) { list.current?.scrollTo({ top: list.current.scrollHeight }); input.current?.focus({ preventScroll: true }); } }, [open, turns]);
  useEffect(() => {
    if (!open) return;
    const esc = (e: globalThis.KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [open]);

  async function send(q: string) {
    const question = q.trim();
    if (!question || pending) return;
    const history = turns.slice(-6);
    setTurns(h => [...h, { role: "user", content: question }]);
    setText(""); setPending(true); setNote("");
    try {
      const r = await api.post<{ answer?: string }>("/magic-dude/chat", { question, history });
      setTurns(h => [...h, { role: "assistant", content: r.answer || t("md.empty") }]);
    } catch (e) {
      const code = e instanceof ApiError && e.data && typeof e.data === "object" ? (e.data as { code?: string }).code : "";
      const status = e instanceof ApiError ? e.status : 0;
      const key = status === 403 && code === "MAGIC_DUDE_UNLOCK_REQUIRED" ? "md.locked" : status === 429 ? "md.limit" : "md.fail";
      setTurns(h => [...h, { role: "assistant", content: t(key) }]);
      setNote(t(key === "md.fail" ? "md.failNote" : key));
    } finally { setPending(false); }
  }
  const submit = (e: FormEvent) => { e.preventDefault(); void send(text); };
  const keys = (e: KeyboardEvent) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void send(text); } };

  return (
    <>
      <button type="button" className="md-fab" aria-expanded={open} aria-controls="md-panel" onClick={() => setOpen(o => !o)}>
        <img src="/v2/magic-dude.png" alt="" width={28} height={28} /><span>Magic Dude</span>
      </button>
      {open && (
        <section id="md-panel" className="md-panel glass" role="dialog" aria-label="Magic Dude">
          <header className="row">
            <div className="row" style={{ justifyContent: "flex-start" }}>
              <img src="/v2/magic-dude.png" alt="" width={40} height={40} />
              <div><h2>Magic Dude</h2><p className="small soft">{t("md.sub")}</p></div>
            </div>
            <button type="button" className="iconbtn" aria-label={t("md.close")} onClick={() => setOpen(false)}>×</button>
          </header>
          <div className="md-list" ref={list} aria-live="polite">
            {turns.length === 0 && (
              <div className="stack">
                <p className="soft">{t("md.hello")}</p>
                <div className="scroll-x" style={{ flexWrap: "wrap" }}>
                  {PROMPTS.map(k => <button key={k} type="button" className="pill" onClick={() => void send(t(k))}>{t(k)}</button>)}
                </div>
              </div>
            )}
            {turns.map((m, i) => <div key={i} className={"md-msg " + m.role}><b className="small">{m.role === "user" ? t("md.you") : "Magic Dude"}</b><p>{m.content}</p></div>)}
            {pending && <div className="md-msg assistant"><p className="soft">{t("md.thinking")}</p></div>}
          </div>
          <form className="stack" onSubmit={submit}>
            <label className="sr-only" htmlFor="md-q">{t("md.ask")}</label>
            <textarea id="md-q" ref={input} className="input md-q" rows={2} maxLength={4000} value={text} disabled={pending} placeholder={t("md.ask")} onChange={e => setText(e.target.value)} onKeyDown={keys} />
            <div className="row"><span className="small mute">{note || t("md.note")}</span><button className="btn" disabled={pending || !text.trim()}>{t("md.send")}</button></div>
          </form>
        </section>
      )}
    </>
  );
}
