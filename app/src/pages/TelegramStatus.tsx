import { useI18n } from "../i18n";

export interface BotRuntime {
  scope?: "platform" | "business";
  receiver?: string;
  token?: string;
  approver_configured?: boolean;
  last_delivery?: string;
  last_delivery_at?: string;
  last_poll_at?: string;
  detail?: string;
}

/** Runtime evidence is separate from the owner's saved enable preference. */
export function TelegramStatus({ runtime }: { runtime?: BotRuntime }) {
  const { t } = useI18n();
  const receiver = runtime?.receiver ?? "unavailable";
  const label = (kind: "receiver" | "token" | "delivery", value?: string) => t(`tg.${kind}.${value ?? (kind === "delivery" ? "not_tested" : "unknown")}`);
  return (
    <section className="stack small" aria-label={t("tg.status")} aria-live="polite">
      <dl className="stack">
        <div><dt>{t("tg.token")}</dt><dd>{label("token", runtime?.token)}</dd></div>
        <div><dt>{t("tg.receiver")}</dt><dd>{label("receiver", receiver)}</dd></div>
        <div><dt>{t("tg.approver")}</dt><dd>{runtime?.approver_configured ? t("tg.configured") : t("tg.notConfigured")}</dd></div>
        <div><dt>{t("tg.lastPoll")}</dt><dd className="num">{runtime?.last_poll_at ? new Date(runtime.last_poll_at).toLocaleString() : "—"}</dd></div>
        <div><dt>{t("tg.delivery")}</dt><dd className="num">{label("delivery", runtime?.last_delivery)}{runtime?.last_delivery_at ? ` · ${new Date(runtime.last_delivery_at).toLocaleString()}` : ""}</dd></div>
      </dl>
      {runtime?.detail && <p role="alert">{runtime.detail}</p>}
      <p className="soft">{t("tg.evidenceNote")}</p>
    </section>
  );
}
