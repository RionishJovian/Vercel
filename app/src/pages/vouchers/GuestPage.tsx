import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Plan { key: string; label: string; minutes: number; amount: number; data_mb: number }
interface Config {
  business_name?: string; welcome_text?: string; terms_text?: string; primary_hex?: string; glass_tint_hex?: string;
  bank_name?: string; bank_holder?: string; bank_account?: string; mmqr_url?: string; payment_instructions?: string;
  guest_grace_minutes?: number; guest_grace_data_mb?: number; guest_plans?: Plan[];
  voucher_login_enabled?: boolean; mobile_payment_enabled?: boolean; consent_required?: boolean; consent_text?: string;
}
const HEX6 = /^#[0-9a-fA-F]{6}$/;
const hexOr = (v: unknown, fallback: string) => (typeof v === "string" && HEX6.test(v.trim()) ? v.trim() : fallback);
// Configs saved before access-method selection existed have no flags: treat as both enabled.
const on = (v: boolean | undefined) => (typeof v === "boolean" ? v : true);
const EMPTY: Config = {};
const blankPlan = (p: Plan) => !(p.key || p.label || p.minutes || p.amount || p.data_mb);

interface Draft {
  name: string; welcome: string; terms: string; primary: string; tint: string;
  bankName: string; bankHolder: string; bankAcc: string; mmqr: string; instructions: string;
  graceMin: string; graceData: string; voucher: boolean; payment: boolean; plans: Plan[];
}
const fromConfig = (c: Config): Draft => ({
  name: c.business_name ?? "", welcome: c.welcome_text ?? "", terms: c.terms_text ?? "", primary: hexOr(c.primary_hex, "#ffb547"), tint: hexOr(c.glass_tint_hex, "#7ad0ff"),
  bankName: c.bank_name ?? "", bankHolder: c.bank_holder ?? "", bankAcc: c.bank_account ?? "", mmqr: c.mmqr_url ?? "", instructions: c.payment_instructions ?? "",
  graceMin: String(c.guest_grace_minutes ?? 10), graceData: String(c.guest_grace_data_mb ?? 50), voucher: on(c.voucher_login_enabled), payment: on(c.mobile_payment_enabled),
  plans: asArray<Plan>(c.guest_plans).map((p) => ({ key: p.key ?? "", label: p.label ?? "", minutes: p.minutes ?? 0, amount: p.amount ?? 0, data_mb: p.data_mb ?? 0 })),
});

/** Preview payload. Empty fields get sample values for the preview only; Save never sends these. */
function previewConfig(d: Draft, c: Config) {
  const has = d.plans.some((p) => !blankPlan(p));
  return {
    business_name: d.name.trim() || "Your WiFi", welcome_text: d.welcome.trim() || "Fast, secure internet for your stay.", terms_text: d.terms.trim() || "One plan connects one device.",
    primary_hex: d.primary, glass_tint_hex: d.tint, bank_name: d.bankName.trim(), bank_holder: d.bankHolder.trim(), bank_account: d.bankAcc.trim(), mmqr_url: d.mmqr.trim(),
    guest_grace_minutes: Number(d.graceMin) || 0, guest_grace_data_mb: Number(d.graceData) || 0, voucher_login_enabled: d.voucher, mobile_payment_enabled: d.payment,
    guest_plans: has ? d.plans : [{ key: "preview-hour", label: "1 Hour WiFi", minutes: 60, amount: 1000, data_mb: 1024 }, { key: "preview-day", label: "Day Pass", minutes: 1440, amount: 3500, data_mb: 5120 }],
    consent_required: !!c.consent_required, consent_text: c.consent_text ?? "", preview_sample: !has,
  };
}

function Section({ title, children }: { title: string; children: ReactNode }) { return <section className="glass pad stack"><h2>{title}</h2>{children}</section>; }

export function GuestPage() {
  const { t, lang } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ config?: Config } & Config>("/portal/config", fetcher, { shouldRetryOnError: false });
  const cfg: Config = useMemo(() => data?.config ?? data ?? EMPTY, [data]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [msg, setMsg] = useState<ReactNode>(null);
  const [busy, setBusy] = useState(false);
  const [dest, setDest] = useState<"router" | "hosted">("router");
  const [note, setNote] = useState("");
  const [html, setHtml] = useState<string | null>(null);
  const hosted = useRef<HTMLIFrameElement>(null);
  const initial = useRef(false);

  useEffect(() => { if (!initial.current && (data || (error && (error as { status?: number }).status === 404))) { initial.current = true; setDraft(fromConfig(cfg)); } }, [data, error, cfg]);

  const pushHosted = useCallback(() => {
    if (!draft || !hosted.current?.contentWindow) return;
    hosted.current.contentWindow.postMessage({ type: "mm-portal-preview", config: { ...previewConfig(draft, cfg), payment_instructions: draft.instructions.trim() }, lang }, location.origin);
  }, [draft, cfg, lang]);
  useEffect(() => {
    const onReady = (ev: MessageEvent) => { if (ev.origin === location.origin && (ev.data as { type?: string } | null)?.type === "mm-portal-preview-ready") pushHosted(); };
    window.addEventListener("message", onReady);
    return () => window.removeEventListener("message", onReady);
  }, [pushHosted]);

  // The server renders the router voucher page, so the preview is the page guests get.
  useEffect(() => {
    if (!draft) return;
    if (dest === "hosted") { setNote(t("g.previewHostedNote")); pushHosted(); return; }
    let live = true; setNote(t("g.previewLoading"));
    const timer = setTimeout(async () => {
      try { const res = await api.post<{ html?: string }>("/portal/router-preview", previewConfig(draft, cfg)); if (!live) return; if (typeof res.html !== "string") throw new Error("no-html"); setHtml(res.html); setNote(t("g.previewNote")); }
      catch (e) { if (isUnauthorized(e)) { location.reload(); return; } if (live) { setHtml(null); setNote(t("g.previewFailed")); } }
    }, 300);
    return () => { live = false; clearTimeout(timer); };
  }, [draft, dest, cfg, pushHosted, t]);

  if (isLoading || !draft) return error && (error as { status?: number }).status !== 404 ? <Note kind="bad">{t("c.loadFailed")}</Note> : <Skeleton h={420} />;
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => (d ? { ...d, [k]: v } : d));
  const setPlan = (i: number, patch: Partial<Plan>) => set("plans", draft.plans.map((p, j) => (j === i ? { ...p, ...patch } : p)));
  const text = (k: keyof Draft, label: string, extra: Record<string, unknown> = {}) => (
    <Field label={label} htmlFor={"gp-" + k}><input className="input" id={"gp-" + k} value={draft[k] as string} onChange={(e) => set(k, e.target.value as never)} {...extra} /></Field>
  );

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!draft) return;
    if (!draft.voucher && !draft.payment) { setMsg(<Note kind="bad">{t("g.needMethod")}</Note>); return; }
    const gm = Number(draft.graceMin), gd = Number(draft.graceData);
    if (!Number.isInteger(gm) || gm < 1 || gm > 1440 || !Number.isInteger(gd) || gd < 1 || gd > 10240) { setMsg(<Note kind="bad">{t("g.badGrace")}</Note>); return; }
    setBusy(true);
    // The server merges, so send only what this screen owns. Media (Jellyfin) and consent settings stay untouched.
    const body = {
      business_name: draft.name.trim(), welcome_text: draft.welcome.trim(), terms_text: draft.terms.trim(), primary_hex: draft.primary, glass_tint_hex: draft.tint,
      bank_name: draft.bankName.trim(), bank_holder: draft.bankHolder.trim(), bank_account: draft.bankAcc.trim(), mmqr_url: draft.mmqr.trim(), payment_instructions: draft.instructions.trim(),
      guest_grace_minutes: gm, guest_grace_data_mb: gd, voucher_login_enabled: draft.voucher, mobile_payment_enabled: draft.payment, guest_plans: draft.plans.filter((p) => !blankPlan(p)),
    };
    try { await api.put("/portal/config", body); setMsg(<><Note kind="ok">{t("g.saved")}</Note><Note kind="warn">{t("g.deployHint")}</Note></>); void mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(<Note kind="bad">{t("g.saveFailed")}</Note>); }
    setBusy(false);
  }

  return (
    <div className="gp-layout">
      <section className="glass pad stack gp-preview" aria-labelledby="gp-prev">
        <h2 id="gp-prev">{t("g.preview")}</h2>
        <div className="row">
          <div className="seg" role="group" aria-label={t("g.preview")}>
            <button type="button" aria-pressed={dest === "router"} onClick={() => setDest("router")}>{t("g.destRouter")}</button>
            <button type="button" aria-pressed={dest === "hosted"} onClick={() => setDest("hosted")}>{t("g.destHosted")}</button>
          </div>
        </div>
        <div className="phone">
          <iframe className="gp-frame" hidden={dest !== "router"} sandbox="allow-scripts" title={t("g.preview")} srcDoc={html ?? undefined} />
          <iframe className="gp-frame" ref={hosted} hidden={dest !== "hosted"} src="/portal.html?preview=1" title={t("g.previewHosted")} />
        </div>
        <p className="soft small" role="status">{note}</p>
      </section>

      <form className="stack gp-form" onSubmit={save}>
        <p className="soft small">{t("g.hint")}</p>
        <Section title={t("g.brand")}>
          {text("name", t("g.name"), { maxLength: 120 })}{text("welcome", t("g.welcome"), { maxLength: 240 })}{text("terms", t("g.terms"), { maxLength: 500 })}
          <div className="two">
            <Field label={t("g.primary")} htmlFor="gp-primary"><input className="input colour" id="gp-primary" type="color" value={draft.primary} onChange={(e) => set("primary", e.target.value)} /></Field>
            <Field label={t("g.tint")} htmlFor="gp-tint"><input className="input colour" id="gp-tint" type="color" value={draft.tint} onChange={(e) => set("tint", e.target.value)} /></Field>
          </div>
        </Section>
        <Section title={t("g.methods")}>
          <label className="check"><span>{t("g.voucherCode")}</span><input type="checkbox" checked={draft.voucher} onChange={(e) => set("voucher", e.target.checked)} /></label>
          <label className="check"><span>{t("g.mobilePay")}</span><input type="checkbox" checked={draft.payment} onChange={(e) => set("payment", e.target.checked)} /></label>
          <p className="soft small">{t("g.methodsHint")}</p>
        </Section>
        <Section title={t("g.pay")}>
          {text("bankName", t("g.bank"), { maxLength: 120 })}{text("bankHolder", t("g.holder"), { maxLength: 120 })}{text("bankAcc", t("g.acc"), { maxLength: 60, inputMode: "numeric" })}{text("mmqr", t("g.mmqr"), { maxLength: 500, placeholder: "https://…/mmqr.jpg" })}
          <Field label={t("g.instr")} htmlFor="gp-instr"><textarea className="input" id="gp-instr" maxLength={600} rows={4} value={draft.instructions} onChange={(e) => set("instructions", e.target.value)} style={{ padding: "12px 16px", minHeight: 104 }} /></Field>
          <p className="soft small">{t("g.instrHint")}</p>
          <div className="two">{text("graceMin", t("g.graceMin"), { type: "number", min: 1, max: 1440, inputMode: "numeric" })}{text("graceData", t("g.graceData"), { type: "number", min: 1, max: 10240, inputMode: "numeric" })}</div>
          <p className="soft small">{t("g.graceHint")}</p>
        </Section>
        <section className="stack"><h2>{t("g.plans")}</h2>
          {draft.plans.map((p, i) => (
            <div className="glass pad stack" key={i}>
              <div className="two"><Field label={t("g.plan.key")}><input className="input" value={p.key} onChange={(e) => setPlan(i, { key: e.target.value })} /></Field><Field label={t("g.plan.label")}><input className="input" value={p.label} onChange={(e) => setPlan(i, { label: e.target.value })} /></Field></div>
              <div className="two">
                <Field label={t("g.plan.minutes")}><input className="input" type="number" min={0} inputMode="numeric" value={p.minutes} onChange={(e) => setPlan(i, { minutes: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
                <Field label={t("g.plan.price")}><input className="input" type="number" min={0} inputMode="numeric" value={p.amount} onChange={(e) => setPlan(i, { amount: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
              </div>
              <Field label={t("g.plan.data")}><input className="input" type="number" min={0} inputMode="numeric" value={p.data_mb} onChange={(e) => setPlan(i, { data_mb: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
              <button className="pill danger" type="button" onClick={() => set("plans", draft.plans.filter((_, j) => j !== i))}>{t("g.plan.remove")}</button>
            </div>
          ))}
          <button className="pill" type="button" onClick={() => set("plans", [...draft.plans, { key: "", label: "", minutes: 0, amount: 0, data_mb: 0 }])}>{t("g.plan.add")}</button>
        </section>
        {msg}
        <button className="btn" type="submit" disabled={busy}>{t("g.save")}</button>
        <a className="btn quiet" href="/app.html#portal">{t("g.deploy")}</a>
      </form>
    </div>
  );
}
