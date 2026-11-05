import { Component, type ReactNode } from "react";
import { useI18n } from "../i18n";

function PageFailed() {
  const { t } = useI18n();
  return (
    <section className="glass pad stack" role="alert">
      <h1>{t("c.pageFailed")}</h1>
      <p className="soft">{t("c.pageFailedHelp")}</p>
      <div><button className="btn" type="button" onClick={() => location.reload()}>{t("c.reload")}</button></div>
    </section>
  );
}

/**
 * Keeps one failing page from blanking the whole app. Give it a key that changes with the
 * route: opening another page then starts fresh, and the tab bar stays usable meanwhile.
 */
export class PageBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? <PageFailed /> : this.props.children; }
}
