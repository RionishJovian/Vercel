import { useI18n } from "../../i18n";
import { BackLink, Note, Pills } from "../../components/ui";
import type { Me } from "../../lib/types";
import { Accounts } from "./Accounts";
import { Renewals } from "./Renewals";
import { Addons } from "./Addons";
import { Pricing } from "./Pricing";
import { Businesses } from "./Businesses";
import { PlatformIncidents } from "./Incidents";

const TABS = ["accounts", "renewals", "addons", "pricing", "businesses", "incidents"] as const;
type Tab = (typeof TABS)[number];

export function Platform({ me, sub }: { me: Me; sub: string }) {
  const { t } = useI18n();
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (me.user?.user_role !== "DEVELOPER") return <>{back}<Note kind="warn">{t("pf.denied")}</Note></>;
  const tab: Tab = (TABS as readonly string[]).includes(sub) ? (sub as Tab) : "accounts";
  return (
    <>
      {back}<h1>{t("title.platform")}</h1>
      <Pills items={TABS} current={tab} href={(k) => "#/platform/" + k} label={(k) => t("pt." + k)} />
      {tab === "accounts" ? <Accounts /> : tab === "renewals" ? <Renewals /> : tab === "addons" ? <Addons me={me} /> : tab === "pricing" ? <Pricing /> : tab === "businesses" ? <Businesses me={me} /> : <PlatformIncidents />}
    </>
  );
}
