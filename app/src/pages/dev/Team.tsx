import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { Page } from "./Paged";

interface User { user_id: string; full_name?: string; email?: string; user_role?: string; is_active?: boolean }
const ROLES = ["OWNER", "ADMIN", "VIEWER"];
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

function Member({ u, onChanged, onError }: { u: User; onChanged: () => void; onError: (m: string) => void }) {
  const { t } = useI18n();
  const [role, setRole] = useState(u.user_role ?? ""), [busy, setBusy] = useState(false);
  const managed = ROLES.includes(u.user_role ?? "");
  const run = async (fn: () => Promise<unknown>) => { setBusy(true); try { await fn(); onChanged(); } catch (e) { if (isUnauthorized(e)) throw e; onError(errText(e)); } setBusy(false); };
  return (
    <div className="glass pad stack">
      <div className="row"><b>{u.full_name}</b><span className={"chip " + (u.is_active ? "ok" : "")}>{u.is_active ? t("dev.tm.active") : t("dev.tm.disabled")}</span></div>
      <p className="soft small">{u.email}</p>
      {managed ? (
        <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
          <select className="input" style={{ width: "auto" }} aria-label={t("dev.tm.role")} value={role} onChange={(e) => setRole(e.target.value)}>{ROLES.map((r) => <option key={r}>{r}</option>)}</select>
          <button className="pill" type="button" disabled={busy || role === u.user_role} onClick={() => void run(() => api.put(`/team/${encodeURIComponent(u.user_id)}`, { user_role: role }))}>{t("pf.save")}</button>
          <button className="pill danger" type="button" disabled={busy || !u.is_active} onClick={() => void run(() => api.del(`/team/${encodeURIComponent(u.user_id)}`))}>{t("dev.tm.disable")}</button>
        </div>
      ) : <span className="soft small">{u.user_role} · {t("dev.tm.platform")}</span>}
    </div>
  );
}

export function Team() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ team?: User[] }>("/team", fetcher, { shouldRetryOnError: false });
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  if (isLoading) return <Page title={t("dev.tm.title")}><Skeleton h={200} /></Page>;
  if (error instanceof ApiError && error.status === 403) return <Page title={t("dev.tm.title")}><Note kind="warn">{t("dev.tm.denied")}</Note></Page>;
  if (error) return <Page title={t("dev.tm.title")}><Note kind="bad">{t("c.loadFailed")}</Note></Page>;
  const team = asArray<User>(data?.team);
  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    setBusy(true); setMsg("");
    try { await api.post("/team", { full_name: String(f.get("name")), email: String(f.get("email")), password: String(f.get("pass")), user_role: String(f.get("role")) }); form.reset(); void mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); }
    setBusy(false);
  }
  return (
    <Page title={t("dev.tm.title")} lead={t("dev.tm.lead")}>
      {msg && <Note kind="bad">{msg}</Note>}
      {!team.length ? <Note>{t("dev.tm.none")}</Note> : <div className="stack">{team.map((u) => <Member key={u.user_id} u={u} onChanged={() => void mutate()} onError={setMsg} />)}</div>}
      <form className="glass pad stack" onSubmit={add}>
        <h2>{t("dev.tm.add")}</h2>
        <Field label={t("pf.name")}><input className="input" name="name" required /></Field>
        <Field label={t("s.email")}><input className="input" name="email" type="email" required /></Field>
        <Field label={t("dev.tm.password")}><input className="input" name="pass" type="password" autoComplete="new-password" minLength={8} required /></Field>
        <Field label={t("dev.tm.role")}><select className="input" name="role" defaultValue="ADMIN">{ROLES.map((r) => <option key={r}>{r}</option>)}</select></Field>
        <button className="btn" type="submit" disabled={busy}>{t("dev.tm.addBtn")}</button>
      </form>
    </Page>
  );
}
