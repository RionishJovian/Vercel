import { useSyncExternalStore } from "react";
import { useI18n } from "../i18n";

const subscribe = (cb: () => void) => {
  window.addEventListener("online", cb);
  window.addEventListener("offline", cb);
  return () => { window.removeEventListener("online", cb); window.removeEventListener("offline", cb); };
};
const isOnline = () => navigator.onLine !== false;

/**
 * Says plainly when this device has lost its connection, so figures already on screen are not read as current.
 * The live region is always present, so screen readers announce the change when it happens.
 * navigator.onLine only reports that no network is available; "online" does not prove MegaMagic is reachable,
 * so nothing is shown in that case.
 */
export function OfflineNotice() {
  const { t } = useI18n();
  const online = useSyncExternalStore(subscribe, isOnline, () => true);
  return (
    <div className="offline-slot" role="status" aria-live="polite">
      {online ? null : <p className="note warn small">{t("c.deviceOffline")}</p>}
    </div>
  );
}
