import { useI18n } from "../../i18n";
import { Pills, WorkspaceTabs } from "../../components/ui";
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
      <h1>{t("title.vouchers")}</h1>
      <WorkspaceTabs items={[{ href: "#/vouchers", label: t("v.ws.vouchers"), current: !guest }, { href: "#/vouchers/guest", label: t("v.ws.guest"), current: guest }]} />
      {guest ? <GuestPage /> : (
        <>
          <Pills items={TASKS} current={task} href={(k) => "#/vouchers/" + k} label={(k) => t("v.t." + k)} />
          <p className="soft small">{t("v.hint")}</p>
          {task === "stock" ? <Stock /> : task === "create" ? <Create /> : task === "shops" ? <Shops /> : <Print />}
        </>
      )}
    </>
  );
}
