import { useEffect, useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../lib/api";
import { useI18n, type Lang } from "../i18n";
import { BackLink, Field, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Profile { full_name: string; email: string; user_role: string; language?: string; has_avatar?: boolean; has_cover?: boolean }
interface Telegram { enabled?: boolean; target_chat_id?: number; approver_user_id?: number; bot_token?: string }
const errText = (e: unknown, f: string) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error && e.message && !e.message.startsWith("http-") ? e.message : f);
const initials = (s?: string) => (s ?? "?").trim().split(/\s+/).slice(0, 2).map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";

async function assetUrl(kind: "avatar" | "cover"): Promise<string> {
  const r = await fetch(`/api/v1/me/profile/${kind}`, { cache: "no-store" });
  if (r.status === 404) return "";
  if (!r.ok) throw new Error("load");
  return URL.createObjectURL(await r.blob());
}
async function putAsset(kind: "avatar" | "cover", file: File) {
  const form = new FormData(); form.append("file", file);
  const r = await fetch(`/api/v1/me/profile/${kind}`, { method: "POST", headers: { "X-MM-CSRF": "1" }, body: form });
  const d = (await r.json().catch(() => ({}))) as { error?: string };
  if (!r.ok) throw new Error(d.error || "save");
}

function Photo({ kind, has, bump }: { kind: "avatar" | "cover"; has?: boolean; bump: number }) {
  const [url, setUrl] = useState("");
  useEffect(() => {
    let live = true, made = "";
    if (has) assetUrl(kind).then((u) => { if (live) { made = u; setUrl(u); } else if (u) URL.revokeObjectURL(u); }).catch(() => undefined); else setUrl("");
    return () => { live = false; if (made) URL.revokeObjectURL(made); };
  }, [kind, has, bump]);
  return url ? <img className={"pf-" + kind} src={url} alt="" /> : null;
}

export function ProfilePage() {
  const { t, lang, setLang } = useI18n();
  const prof = useSWR<Profile>("/me/profile", fetcher);
  const tg = useSWR<Telegram>("/guest-wifi/telegram-bot", fetcher, { shouldRetryOnError: false });
  const [msg, setMsg] = useState<Record<string, { bad?: boolean; text: string }>>({});
  const [bump, setBump] = useState(0);
  const say = (k: string, text: string, bad = false) => setMsg((m) => ({ ...m, [k]: { text, bad } }));
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (prof.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (prof.error || !prof.data) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const p = prof.data, T = tg.data ?? {};

  async function saveProfile(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), language = String(f.get("lang")) as Lang;
    say("profile", t("pf.saving"));
    try { await api.put("/me/profile", { full_name: String(f.get("name")).trim(), language }); setLang(language); say("profile", t("pf.saved")); void prof.mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; say("profile", errText(err, t("pf.saveFail")), true); }
  }
  async function changePassword(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    say("password", t("pf.changing"));
    try { await api.post("/me/password", { current_password: String(f.get("current")), new_password: String(f.get("next")) }); form.reset(); say("password", t("pf.pwChanged")); }
    catch (err) { if (isUnauthorized(err) && !(err instanceof ApiError && err.status === 401 && err.message !== "unauthorized")) throw err; say("password", errText(err, t("pf.pwFail")), true); }
  }
  async function saveTelegram(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), chat = String(f.get("chat")).trim(), user = String(f.get("user")).trim(), enabled = f.get("enabled") === "on";
    say("tg", t("pf.saving"));
    try {
      if ((chat && (!/^-?[1-9]\d*$/.test(chat) || !Number.isSafeInteger(Number(chat)))) || (user && (!/^[1-9]\d*$/.test(user) || !Number.isSafeInteger(Number(user)))) || (enabled && (!chat || !user))) throw new Error(t("pf.tgInvalid"));
      await api.put("/guest-wifi/telegram-bot", { bot_token: String(f.get("token")).trim(), target_chat_id: chat ? Number(chat) : 0, approver_user_id: user ? Number(user) : 0, enabled });
      say("tg", t("pf.tgSaved")); void tg.mutate();
    } catch (err) { if (isUnauthorized(err)) throw err; say("tg", errText(err, t("pf.tgFail")), true); }
  }
  async function testBot() {
    say("tg", t("pf.tgChecking"));
    try { const r = await api.post<{ ok?: boolean; detail?: string }>("/guest-wifi/telegram-bot/test", {}); say("tg", r.ok ? `${t("pf.tgOk")} ${r.detail ?? ""}` : `${t("pf.tgBad")} ${r.detail ?? ""}`, !r.ok); }
    catch (err) { if (isUnauthorized(err)) throw err; say("tg", errText(err, t("pf.tgFail")), true); }
  }
  async function upload(kind: "avatar" | "cover", file?: File) {
    if (!file) return;
    say("photo", t("pf.uploading"));
    try { await putAsset(kind, file); say("photo", t("pf.uploaded")); setBump((b) => b + 1); void prof.mutate(); } catch (err) { say("photo", errText(err, t("pf.uploadFail")), true); }
  }
  const out = (k: string) => msg[k] ? <Note kind={msg[k].bad ? "bad" : "ok"}>{msg[k].text}</Note> : null;

  return (
    <>
      {back}
      <section className="glass pf-hero">
        <div className="pf-coverbox"><Photo kind="cover" has={p.has_cover} bump={bump} /></div>
        <div className="pf-id">
          <div className="pf-avatarbox"><Photo kind="avatar" has={p.has_avatar} bump={bump} /><span>{initials(p.full_name || p.email)}</span></div>
          <div className="stack" style={{ gap: 2 }}><h1>{p.full_name}</h1><p className="soft small">{p.email}</p><span className="chip">{p.user_role}</span></div>
        </div>
        <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap", padding: "0 16px 16px" }}>
          <label className="pill">{t("pf.avatar")}<input type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={(e) => void upload("avatar", e.target.files?.[0])} /></label>
          <label className="pill">{t("pf.cover")}<input type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={(e) => void upload("cover", e.target.files?.[0])} /></label>
          {p.has_avatar && <button className="pill danger" type="button" onClick={async () => { try { await api.del("/me/profile/avatar"); setBump((b) => b + 1); void prof.mutate(); } catch (err) { if (isUnauthorized(err)) throw err; say("photo", errText(err, t("pf.removeFail")), true); } }}>{t("pf.removeAvatar")}</button>}
        </div>
        {out("photo")}
      </section>

      <form className="glass pad stack" onSubmit={saveProfile} key={p.full_name + lang}>
        <h2>{t("pf.details")}</h2>
        <Field label={t("pf.name")} htmlFor="pf-name"><input className="input" id="pf-name" name="name" maxLength={255} required defaultValue={p.full_name} /></Field>
        <Field label={t("pf.email")} htmlFor="pf-email"><input className="input" id="pf-email" value={p.email} disabled readOnly /></Field>
        <p className="soft small">{t("pf.emailHint")}</p>
        <Field label={t("pf.language")} htmlFor="pf-lang"><select className="input" id="pf-lang" name="lang" defaultValue={lang}><option value="my">မြန်မာ</option><option value="en">English</option></select></Field>
        {out("profile")}<button className="btn" type="submit">{t("pf.save")}</button>
      </form>

      <form className="glass pad stack" onSubmit={changePassword}>
        <h2>{t("pf.password")}</h2><p className="soft small">{t("pf.passwordHint")}</p>
        <Field label={t("pf.current")} htmlFor="pf-cur"><input className="input" id="pf-cur" name="current" type="password" autoComplete="current-password" required /></Field>
        <Field label={t("pf.new")} htmlFor="pf-new"><input className="input" id="pf-new" name="next" type="password" autoComplete="new-password" minLength={8} maxLength={128} required /></Field>
        {out("password")}<button className="btn quiet" type="submit">{t("pf.changePw")}</button>
      </form>

      <form className="glass pad stack" onSubmit={saveTelegram} key={JSON.stringify(T)}>
        <h2>{t("pf.tg")}</h2><p className="soft small">{t("pf.tgHint")}</p>
        <Field label={t("pf.tgToken")} htmlFor="pf-tok"><input className="input" id="pf-tok" name="token" type="password" autoComplete="off" placeholder={T.bot_token ? `${t("pf.tgBound")} ${T.bot_token}` : t("pf.tgPaste")} /></Field>
        <Field label={t("pf.tgChat")} htmlFor="pf-chat"><input className="input" id="pf-chat" name="chat" inputMode="numeric" defaultValue={T.target_chat_id || ""} placeholder="123456789" /></Field>
        <Field label={t("pf.tgUser")} htmlFor="pf-user"><input className="input" id="pf-user" name="user" inputMode="numeric" defaultValue={T.approver_user_id || ""} /></Field>
        <label className="check"><span>{t("pf.tgEnable")}</span><input type="checkbox" name="enabled" defaultChecked={!!T.enabled} /></label>
        {out("tg")}
        <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><button className="btn" type="submit">{t("pf.tgSave")}</button><button className="btn quiet" type="button" onClick={() => void testBot()}>{t("pf.tgTest")}</button></div>
      </form>
    </>
  );
}
