import { useI18n } from "../../i18n";
import type { HubStatus } from "./hubStatus";

/** The label comes from the Magic Hub handshake. "Unknown" means the lookup was unavailable, not that the router is down. */
export function HubChip({ status }: { status?: HubStatus }) {
  const { t } = useI18n();
  const state = status?.state;
  return <span className={"chip " + (state === "up" ? "ok" : state === "down" ? "bad" : "")}>{state === "up" ? t("c.online") : state === "down" ? t("c.offline") : (status ? t("r.noStatus") : t("r.checking"))}</span>;
}

