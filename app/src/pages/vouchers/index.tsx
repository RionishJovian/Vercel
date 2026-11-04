import { useI18n } from "../../i18n";
import { PageHead, Pills, WorkspaceTabs } from "../../components/ui";
import { Stock } from "./Stock";
import { Create } from "./Create";
import { Shops } from "./Shops";
import { Print } from "./Print";
import { GuestPage } from "./GuestPage";

const TASKS = ["stock", "create", "shops", "print"] as const;
type Task = (typeof TASKS)[number];

export function Vouchers({ sub }: { sub: string }) {
  const { t } = useI18n();
  const guest = sub === "guest";
  const task: Task = (TASKS as readonly string[]).includes(sub) ? (sub as Task) : "stock";
  return (
    <>
      <PageHead title={t("title.vouchers")} help={guest ? t("g.hint") : t("v.hint")} />
      <WorkspaceTabs items={[{ href: "#/vouchers", label: t("v.ws.vouchers"), current: !guest }, { href: "#/vouchers/guest", label: t("v.ws.guest"), current: guest }]} />
      {guest ? <GuestPage /> : (
        <>
          <Pills items={TASKS} current={task} href={(k) => "#/vouchers/" + k} label={(k) => t("v.t." + k)} />
          {task === "stock" ? <Stock /> : task === "create" ? <Create /> : task === "shops" ? <Shops /> : <Print />}
        </>
      )}
    </>
  );
}
