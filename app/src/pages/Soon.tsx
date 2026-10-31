import { useI18n } from "../i18n";

/** Pages not yet moved over open in the previous app so nothing is unreachable while the rebuild continues. */
export function Soon({ legacy }: { legacy: string }) {
  const { t } = useI18n();
  return (
    <section className="glass pad stack">
      <h1>{t("soon.title")}</h1>
      <p className="soft">{t("soon.body")}</p>
      <a className="btn" href={"/app.html#" + legacy}>{t("soon.open")}</a>
    </section>
  );
}
