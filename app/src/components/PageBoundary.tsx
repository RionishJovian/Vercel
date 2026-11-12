import { Component, type ReactNode } from "react";
import { useI18n } from "../i18n";
import { isChunkLoadError, requestReload } from "../lib/staleBuild";

function PageFailed() {
  const { t } = useI18n();
  return (
    <section className="glass pad stack" role="alert">
      <h1>{t("c.pageFailed")}</h1>
      <p className="soft">{t("c.pageFailedHelp")}</p>
      <div><button className="btn" type="button" onClick={() => requestReload(t("c.reloadEdits"))}>{t("c.reload")}</button></div>
    </section>
  );
}

/**
 * Keeps one failing page from blanking the whole app. Give it a key that changes with the
 * route: opening another page then starts fresh, and the tab bar stays usable meanwhile.
 */
export class PageBoundary extends Component<{ children: ReactNode; inline?: boolean }, { failed: boolean; error: unknown }> {
  state = { failed: false, error: null as unknown };
  static getDerivedStateFromError(error: unknown) { return { failed: true, error }; }
  render() {
    if (!this.state.failed) return this.props.children;
    if (isChunkLoadError(this.state.error)) return <ChunkFailed />;
    if (this.props.inline) throw this.state.error;
    return <PageFailed />;
  }
}

function ChunkFailed() {
  const { t } = useI18n();
  return <section className="glass pad stack" role="alert">
    <h2>{t("c.partFailed")}</h2>
    <p className="soft">{t("c.partFailedHelp")}</p>
    <div><button className="btn" type="button" onClick={() => requestReload(t("c.reloadEdits"))}>{t("c.reload")}</button></div>
  </section>;
}
