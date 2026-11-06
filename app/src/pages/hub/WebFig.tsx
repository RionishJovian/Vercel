import { useState } from "react";
import useSWR from "swr";
import { Note } from "../../components/ui";
import { useI18n } from "../../i18n";
import { api, ApiError, fetcher } from "../../lib/api";
import { errorText } from "../../lib/errors";

interface Status {
  access: "off" | "locked" | "open";
  ready?: boolean;
  plan?: string[];
  version?: string;
  update_required?: boolean;
}

type Action = "allow" | "remove" | "open";

export function WebFig({ id }: { id: string }) {
  const { t } = useI18n();
  const path = `/routers/${encodeURIComponent(id)}/webfig`;
  const { data, error, mutate, isValidating } = useSWR<Status>(path, fetcher, { shouldRetryOnError: false, revalidateOnFocus: false });
  const [review, setReview] = useState<"allow" | "remove" | null>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");

  async function run(action: Action) {
    // Reserve a tab within the click so the browser treats this as user initiated.
    const tab = action === "open" ? window.open("about:blank", "_blank") : null;
    if (action === "open" && !tab) {
      setFailure(t("wf.popup"));
      return;
    }
    if (tab) tab.opener = null;
    setFailure("");
    setBusy(true);
    try {
      if (tab) {
        const { url } = await api.post<{ url: string }>(path + "/session");
        const destination = new URL(url);
        if (destination.protocol !== "https:") throw new Error(t("wf.failed"));
        tab.location.replace(destination.href);
      } else {
        const updated = action === "allow"
          ? await api.post<Status>(path + "/access", { confirm: "ALLOW_WEBFIG" })
          : await api.del<Status>(path + "/access");
        await mutate(updated, { revalidate: false });
        setReview(null);
      }
    } catch (err) {
      tab?.close();
      setFailure(errorText(err, t("wf.failed")));
    } finally {
      setBusy(false);
    }
  }

  if (data?.access === "off") return null;
  if (error) {
    // Older API releases do not register the route. Preserve explicit router errors.
    if (error instanceof ApiError && error.status === 404 && !error.data) return null;
    return <Note kind="warn">{errorText(error, t("wf.failed"))} <button className="pill" type="button" disabled={isValidating} onClick={() => void mutate()}>{t("wf.retry")}</button></Note>;
  }
  if (!data) return null;
  if (data.access === "locked") return <Note>{t("wf.locked")}</Note>;
  return (
    <section className="stack" aria-label={t("wf.title")}>
      <h3>{t("wf.title")}</h3>
      {data.update_required && <Note kind="warn">{t("wf.update")} {data.version}</Note>}
      <p className="small soft">{t(data.ready ? "wf.ready" : "wf.setup")}</p>
      {!review && <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        {data.ready ? <>
          <button className="pill" type="button" disabled={busy || data.update_required} onClick={() => void run("open")}>{t("wf.open")}</button>
          <button className="pill" type="button" disabled={busy} onClick={() => { setFailure(""); setReview("remove"); }}>{t("wf.remove")}</button>
        </> : <button className="pill" type="button" disabled={busy || data.update_required} onClick={() => { setFailure(""); setReview("allow"); }}>{t("wf.review")}</button>}
      </div>}
      {review && <div className="note warn stack">
        <b>{t(review === "allow" ? "wf.review" : "wf.remove")}</b>
        {review === "allow" && !!data.plan?.length && <ul>{data.plan.map((step) => <li key={step}>{step}</li>)}</ul>}
        {review === "remove" && <p>{t("wf.removeHelp")}</p>}
        <div className="row" style={{ justifyContent: "flex-start" }}>
          <button className="pill" type="button" disabled={busy} onClick={() => { setReview(null); setFailure(""); }}>{t("c.cancel")}</button>
          <button className="btn" type="button" disabled={busy} onClick={() => void run(review)}>{t(review === "allow" ? "wf.allow" : "wf.remove")}</button>
        </div>
      </div>}
      {failure && <Note kind="bad">{failure}</Note>}
    </section>
  );
}
