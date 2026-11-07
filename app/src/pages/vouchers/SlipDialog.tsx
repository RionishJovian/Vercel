import { useEffect, useRef } from "react";
import { useI18n } from "../../i18n";
import { printCodes, type Layout, type PrintVoucher } from "./printCodes";
import { Preview } from "./Print";

/** One code's slip, on screen with its QR code. Print sends this same slip to paper, so the two cannot disagree. */
export function SlipDialog({ voucher, layout, firstUse, onClose }: { voucher: PrintVoucher; layout: Layout; firstUse: string; onClose: () => void }) {
  const { t } = useI18n();
  const box = useRef<HTMLDialogElement>(null);
  const slip: Layout = { ...layout, show_qr: true };
  useEffect(() => {
    const d = box.current;
    if (d && !d.open) { if (d.showModal) d.showModal(); else d.setAttribute("open", ""); }
  }, []);
  return (
    <dialog ref={box} className="slip-dialog glass" aria-label={t("vt.slip")} onClose={onClose} onCancel={onClose} onClick={(e) => { if (e.target === box.current) onClose(); }}>
      <div className="slip-body">
        <Preview voucher={voucher} layout={slip} firstUse={firstUse} />
        <div className="slip-actions">
          <button className="btn" type="button" onClick={() => printCodes([voucher], slip, firstUse)}>{t("vt.slipPrint")}</button>
          <button className="btn quiet" type="button" onClick={onClose}>{t("vt.slipClose")}</button>
        </div>
      </div>
    </dialog>
  );
}
