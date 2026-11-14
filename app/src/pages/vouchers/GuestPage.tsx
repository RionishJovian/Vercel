import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized, request } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note, Section, SectionHead } from "../../components/ui";
import { speedOk, speedValue } from "./planSpeed";
import { Skeleton } from "../../components/Skeleton";
import { GuestDeploy } from "./GuestDeploy";
import { PORTAL_TEMPLATES, effectiveTemplate, isPremiumTemplate, type TemplateGroup } from "./portalTemplates";

interface Plan { key: string; label: string; minutes: number; amount: number; data_mb: number; speed_down_mbps?: number; speed_up_mbps?: number }
interface Config {
  business_name?: string; welcome_text?: string; terms_text?: string; primary_hex?: string; glass_tint_hex?: string;
  bank_name?: string; bank_holder?: string; bank_account?: string; mmqr_url?: string; payment_instructions?: string;
  guest_grace_minutes?: number; guest_grace_data_mb?: number; guest_plans?: Plan[];
  voucher_login_enabled?: boolean; mobile_payment_enabled?: boolean; consent_required?: boolean; consent_text?: string; template_id?: string;
}
const HEX6 = /^#[0-9a-fA-F]{6}$/;
const hexOr = (v: unknown, fallback: string) => (typeof v === "string" && HEX6.test(v.trim()) ? v.trim() : fallback);
// Configs saved before access-method selection existed have no flags: treat as both enabled.
const on = (v: boolean | undefined) => (typeof v === "boolean" ? v : true);
const EMPTY: Config = {};
const blankPlan = (p: Plan) => !(p.key || p.label || p.minutes || p.amount || p.data_mb || p.speed_down_mbps || p.speed_up_mbps);

interface Draft {
  name: string; welcome: string; terms: string; primary: string; tint: string;
  bankName: string; bankHolder: string; bankAcc: string; mmqr: string; instructions: string;
  graceMin: string; graceData: string; voucher: boolean; payment: boolean; plans: Plan[]; template: string;
}
const fromConfig = (c: Config): Draft => ({
  name: c.business_name ?? "", welcome: c.welcome_text ?? "", terms: c.terms_text ?? "", primary: hexOr(c.primary_hex, "#38bdf8"), tint: hexOr(c.glass_tint_hex, "#121620"),
  bankName: c.bank_name ?? "", bankHolder: c.bank_holder ?? "", bankAcc: c.bank_account ?? "", mmqr: c.mmqr_url ?? "", instructions: c.payment_instructions ?? "",
  graceMin: String(c.guest_grace_minutes ?? 10), graceData: String(c.guest_grace_data_mb ?? 50), voucher: on(c.voucher_login_enabled), payment: on(c.mobile_payment_enabled), template: c.template_id ?? "",
  plans: asArray<Plan>(c.guest_plans).map((p) => ({ key: p.key ?? "", label: p.label ?? "", minutes: p.minutes ?? 0, amount: p.amount ?? 0, data_mb: p.data_mb ?? 0, speed_down_mbps: p.speed_down_mbps ?? 0, speed_up_mbps: p.speed_up_mbps ?? 0 })),
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

/** Body for POST /portal/router-preview: the same renderer that writes the router's login.html. */
function routerPreviewBody(d: Draft, c: Config) {
  const p = previewConfig(d, c);
  return {
    business_name: p.business_name, welcome_text: p.welcome_text, terms_text: p.terms_text, primary_hex: d.primary, glass_tint_hex: d.tint,
    consent_required: !!c.consent_required, consent_text: c.consent_text ?? "", mobile_payment_enabled: d.payment, template_id: d.template,
    plans: p.guest_plans.slice(0, 4).map((x) => ({ label: x.label, minutes: x.minutes, amount: x.amount })),
  };
}

export function GuestPage() {
  const { t, lang } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ config?: Config } & Config>("/portal/config", fetcher, { shouldRetryOnError: false });
  const cfg: Config = useMemo(() => data?.config ?? data ?? EMPTY, [data]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [msg, setMsg] = useState<ReactNode>(null);
  const [busy, setBusy] = useState(false);
  const hosted = useRef<HTMLIFrameElement>(null);
  const [tab, setTab] = useState<"login" | "pay">("login");
  const [loginHtml, setLoginHtml] = useState<string | null>(null), [loginFailed, setLoginFailed] = useState(false);
  const access = useSWR<{ exempt?: boolean; unlocked?: string[] }>("/portal/template-access", fetcher, { shouldRetryOnError: false });
  const initial = useRef(false);

  useEffect(() => { if (!initial.current && (data || (error && (error as { status?: number }).status === 404))) { initial.current = true; setDraft(fromConfig(cfg)); } }, [data, error, cfg]);

  const pushHosted = useCallback(() => {
    if (!draft || !hosted.current?.contentWindow) return;
    hosted.current.contentWindow.postMessage({ type: "mm-portal-preview", start: "pay", config: { ...previewConfig(draft, cfg), payment_instructions: draft.instructions.trim() }, lang }, location.origin);
  }, [draft, cfg, lang]);
  useEffect(() => {
    const onReady = (ev: MessageEvent) => {
      const type = (ev.data as { type?: string } | null)?.type;
      if (ev.origin === location.origin && ev.source === hosted.current?.contentWindow && type === "mm-portal-preview-ready") pushHosted();
      // The login preview is a sandboxed srcdoc frame (origin "null"); its bank transfer link asks to open the pay page.
      if (type === "mm-portal-preview-pay") setTab("pay");
    };
    window.addEventListener("message", onReady);
    return () => window.removeEventListener("message", onReady);
  }, [pushHosted]);

  useEffect(() => { pushHosted(); }, [pushHosted, tab]);

  // The login page preview comes from the same renderer that builds the router's login.html, so it cannot drift.
  const previewBody = draft ? { ...routerPreviewBody(draft, cfg) } : null;
  const previewKey = JSON.stringify(previewBody);
  useEffect(() => {
    if (!previewBody) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      request<{ html?: string }>("POST", "/portal/router-preview", previewBody, false, controller.signal)
        .then((r) => { if (!controller.signal.aborted) { setLoginHtml(r.html ?? ""); setLoginFailed(false); } })
        .catch((e) => { if (isUnauthorized(e)) throw e; if (!controller.signal.aborted) setLoginFailed(true); });
    }, 250);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [previewKey]); // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading || !draft) return error && (error as { status?: number }).status !== 404 ? <Note kind="bad">{t("c.loadFailed")}</Note> : <Skeleton h={420} />;
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => (d ? { ...d, [k]: v } : d));
  const unlocked = access.data?.unlocked ?? PORTAL_TEMPLATES.filter((x) => !isPremiumTemplate(x.id)).map((x) => x.id);
  const locked = isPremiumTemplate(draft.template) && !unlocked.includes(draft.template);
  const pick = (id: string) => { const tpl = PORTAL_TEMPLATES.find((x) => x.id === id); if (tpl) setDraft((d) => (d ? { ...d, template: id, primary: tpl.primary, tint: tpl.tint } : d)); };
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
    if (!draft.plans.filter((p) => !blankPlan(p)).every(speedOk)) { setMsg(<Note kind="bad">{t("g.plan.speedBad")}</Note>); return; }
    setBusy(true);
    // The server merges, so send only what this screen owns. Media (Jellyfin) and consent settings stay untouched.
    const body = {
      business_name: draft.name.trim(), welcome_text: draft.welcome.trim(), terms_text: draft.terms.trim(), primary_hex: draft.primary, glass_tint_hex: draft.tint,
      bank_name: draft.bankName.trim(), bank_holder: draft.bankHolder.trim(), bank_account: draft.bankAcc.trim(), mmqr_url: draft.mmqr.trim(), payment_instructions: draft.instructions.trim(),
      guest_grace_minutes: gm, guest_grace_data_mb: gd, voucher_login_enabled: draft.voucher, mobile_payment_enabled: draft.payment, guest_plans: draft.plans.filter((p) => !blankPlan(p)),
      // Only sent when changed, so saving other settings never touches (or trips the add-on check for) the template.
      ...(draft.template !== (cfg.template_id ?? "") ? { template_id: draft.template } : {}),
    };
    try { await api.put("/portal/config", body); setMsg(<><Note kind="ok">{t("g.saved")}</Note><Note kind="warn">{t("g.deployHint")}</Note></>); void mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(err instanceof ApiError && err.status === 403 ? <Note kind="warn">{t("gt.lockedNote")}</Note> : <Note kind="bad">{t("g.saveFailed")}</Note>); }
    setBusy(false);
  }

  return (
    <div className="gp-layout">
      <section className="glass pad stack gp-preview" aria-labelledby="gp-prev">
        <h2 id="gp-prev">{t("g.guestPortal")}</h2>
        <div className="row" role="tablist" aria-label={t("g.guestPortal")}>
          <button type="button" role="tab" aria-selected={tab === "login"} className={"pill" + (tab === "login" ? " on" : "")} onClick={() => setTab("login")}>{t("gt.tabLogin")}</button>
          <button type="button" role="tab" aria-selected={tab === "pay"} className={"pill" + (tab === "pay" ? " on" : "")} onClick={() => setTab("pay")}>{t("gt.tabPay")}</button>
        </div>
        <div className="phone">
          {tab === "pay"
            ? <iframe className="gp-frame" ref={hosted} src="/portal.html?preview=1" title={t("g.guestPortal")} />
            : loginHtml !== null ? <iframe className="gp-frame" sandbox="allow-scripts" title={t("gt.tabLogin")} srcDoc={loginHtml} /> : loginFailed ? <Note kind="warn">{t("g.previewFailed")}</Note> : <Skeleton h={420} />}
        </div>
        {tab === "login" && loginFailed && loginHtml !== null && <Note kind="warn">{t("g.previewFailed")}</Note>}
        <p className="soft small">{tab === "pay" ? t("g.previewHostedNote") : t("gt.previewNote")}</p>
        {draft.payment && !draft.plans.some((p) => !blankPlan(p)) && <Note kind="warn">{t("g.noPlansWarn")}</Note>}
      </section>

      <div className="stack gp-form">
      <form className="stack" onSubmit={save}>
        <Section title={t("gt.title")} help={t("gt.help")}>
          {(["myanmar", "commercial"] as TemplateGroup[]).map((g) => (
            <fieldset className="tpl-group" key={g}>
              <legend className="soft small">{t("gt.group." + g)}</legend>
              <div className="tpl-grid">
                {PORTAL_TEMPLATES.filter((x) => x.group === g).map((x) => {
                  const premium = isPremiumTemplate(x.id), owned = !premium || unlocked.includes(x.id);
                  return (
                    <button type="button" key={x.id} className={"tpl-card" + (effectiveTemplate(draft.template) === x.id ? " on" : "")} aria-pressed={effectiveTemplate(draft.template) === x.id} onClick={() => pick(x.id)} data-template={x.id}>
                      <span className="tpl-swatch" aria-hidden="true" style={{ background: x.bg, borderColor: x.accent }}><i style={{ background: x.surface }} /><b style={{ background: x.accent }} /><em style={{ background: x.ink }} /></span>
                      <span className="tpl-name">{lang === "my" ? x.nameMy : x.name}</span>
                      {premium && <span className={"chip " + (owned ? "ok" : "warn")}>{owned ? t("gt.unlocked") : t("gt.addon")}</span>}
                    </button>
                  );
                })}
              </div>
            </fieldset>
          ))}
          {locked && <Note kind="warn">{t("gt.lockedNote")}</Note>}
        </Section>
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
        <Section title={t("g.pay")} help={t("g.instrHint") + " " + t("g.graceHint")}>
          {text("bankName", t("g.bank"), { maxLength: 120 })}{text("bankHolder", t("g.holder"), { maxLength: 120 })}{text("bankAcc", t("g.acc"), { maxLength: 60, inputMode: "numeric", className: "input num" })}{text("mmqr", t("g.mmqr"), { maxLength: 500, placeholder: "https://…/mmqr.jpg" })}
          <Field label={t("g.instr")} htmlFor="gp-instr"><textarea className="input" id="gp-instr" maxLength={600} rows={4} value={draft.instructions} onChange={(e) => set("instructions", e.target.value)} style={{ padding: "12px 16px", minHeight: 104 }} /></Field>
          <div className="two">{text("graceMin", t("g.graceMin"), { type: "number", min: 1, max: 1440, inputMode: "numeric", className: "input num" })}{text("graceData", t("g.graceData"), { type: "number", min: 1, max: 10240, inputMode: "numeric", className: "input num" })}</div>
        </Section>
        <section className="stack"><SectionHead title={t("g.plans")} help={t("g.plan.speedHint")} />
          {draft.plans.map((p, i) => (
            <div className="glass pad stack plan-card" key={i}>
              <div className="two"><Field label={t("g.plan.key")}><input className="input mono" value={p.key} onChange={(e) => setPlan(i, { key: e.target.value })} /></Field><Field label={t("g.plan.label")}><input className="input" value={p.label} onChange={(e) => setPlan(i, { label: e.target.value })} /></Field></div>
              <div className="two">
                <Field label={t("g.plan.minutes")}><input className="input num" type="number" min={0} inputMode="numeric" value={p.minutes} onChange={(e) => setPlan(i, { minutes: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
                <Field label={t("g.plan.price")}><input className="input num" type="number" min={0} inputMode="numeric" value={p.amount} onChange={(e) => setPlan(i, { amount: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
              </div>
              <Field label={t("g.plan.data")}><input className="input num" type="number" min={0} inputMode="numeric" value={p.data_mb} onChange={(e) => setPlan(i, { data_mb: parseInt(e.target.value || "0", 10) || 0 })} /></Field>
              <div className="two">
                <Field label={t("g.plan.down")}><input className="input num" type="number" min={0} max={10000} inputMode="numeric" value={p.speed_down_mbps ?? 0} onChange={(e) => setPlan(i, { speed_down_mbps: speedValue(e.target.value) })} /></Field>
                <Field label={t("g.plan.up")}><input className="input num" type="number" min={0} max={10000} inputMode="numeric" value={p.speed_up_mbps ?? 0} onChange={(e) => setPlan(i, { speed_up_mbps: speedValue(e.target.value) })} /></Field>
              </div>
              <button className="pill danger" type="button" onClick={() => set("plans", draft.plans.filter((_, j) => j !== i))}>{t("g.plan.remove")}</button>
            </div>
          ))}
          <button className="pill" type="button" onClick={() => set("plans", [...draft.plans, { key: "", label: "", minutes: 0, amount: 0, data_mb: 0, speed_down_mbps: 0, speed_up_mbps: 0 }])}>{t("g.plan.add")}</button>
        </section>
        {msg}
        {locked ? <a className="btn" href="/app/store">{t("gt.unlock")}</a> : <button className="btn" type="submit" disabled={busy}>{t("g.save")}</button>}
      </form>
      <GuestDeploy />
      </div>
    </div>
  );
}
