import { createFileRoute, Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createClientOnlyFn, useServerFn } from "@tanstack/react-start";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import QRCode from "qrcode";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Input } from "@/components/ui/input";
import { copyText } from "@/lib/browser/clipboard";
import { listRouters } from "@/lib/routers.functions";
import { HotspotGuestReadyBanner } from "@/components/HotspotGuestReadyBanner";
import { PlansPanel } from "@/components/PlansPanel";
import { useSelectedSite } from "@/hooks/useSelectedSite";
import { getSnapshot, deleteVoucher, redeemVoucher, kickUser } from "@/lib/mikrotik.functions";
import {
  getPortalSettings,
  issuePlanVouchers,
  listPlans,
  listVoucherCodes,
  scanLegacyRouterVouchers,
  importLegacyRouterVouchers,
  reconcileVoucherLedger,
} from "@/lib/portal.functions";
import { sellableVoucherUsers } from "@/lib/hotspot-voucher-users";
import { voucherState, type VoucherState } from "@/lib/payments/plan-revenue";
import { isVirtualRouter } from "@/lib/test-router";
import { useT } from "@/lib/i18n";
import { toErrorMessage } from "@/lib/error-message";
import { RecordPagination } from "@/components/RecordPagination";
import { PAGE_SIZE } from "@/components/record-pagination.helpers";
import { getMe } from "@/lib/auth.functions";
import { canManageVoucherPrintLayouts, canReconcileVoucherLedger } from "@/lib/app-role";
import { getVoucherPrintLayout } from "@/lib/voucher-print-layout.functions";
import { DEFAULT_VOUCHER_PRINT_LAYOUT, type VoucherPrintLayout } from "@/lib/voucher-print-layout";
import { fmtDateTime } from "@/lib/time";
import type { Database } from "@/integrations/supabase/types";
import { pageSelectionState, updatePageSelection } from "@/components/record-selection.helpers";
import { voucherPlanLabel } from "@/lib/voucher-display";

const printVoucherThermalReceipt = createClientOnlyFn(
  async (voucher: VoucherView, layout: VoucherPrintLayout) => {
    const { printVoucherThermalReceipt: print } = await import("@/lib/voucher-print.client");
    return print(voucher, layout);
  },
);

const printVoucherA4CutSheet = createClientOnlyFn(
  async (
    vouchers: Array<{
      code: string;
      profile: string;
      priceMmk?: number | null;
      expiresAt?: string | null;
    }>,
    title: string,
    options?: {
      showPrice?: boolean;
      showExpiry?: boolean;
      columns?: 2 | 3 | 4 | 5;
      rowsPerPage?: number;
    },
  ) => {
    const { printVoucherA4CutSheet: print } = await import("@/lib/voucher-print.client");
    return print(vouchers, {
      title,
      showPrice: options?.showPrice,
      showExpiry: options?.showExpiry,
      columns: options?.columns,
      rowsPerPage: options?.rowsPerPage,
    });
  },
);

const printVoucherThermalCutStrip = createClientOnlyFn(
  async (
    vouchers: Array<{
      code: string;
      profile: string;
      priceMmk?: number | null;
      expiresAt?: string | null;
    }>,
    layout: VoucherPrintLayout,
  ) => {
    const { printVoucherThermalCutStrip: print } = await import("@/lib/voucher-print.client");
    return print(
      vouchers.map((voucher) => ({
        code: voucher.code,
        profile: voucher.profile,
        priceMmk: voucher.priceMmk ?? undefined,
        expiresAt: voucher.expiresAt,
      })),
      layout,
    );
  },
);


export const Route = createFileRoute("/_authenticated/app/vouchers")({
  head: () => ({
    meta: [
      { title: "Vouchers — MikroTik Hotspot Admin" },
      {
        name: "description",
        content:
          "Create, view, and hand out hotspot voucher codes: bulk generation, printable slips, live status, and revocation.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: VouchersPage,
});

type UserRow = Record<string, string> & { ".id": string };
type ActiveRow = Record<string, string> & { ".id": string };
type VoucherLedgerRow = Database["public"]["Tables"]["voucher_codes"]["Row"];

type Filter = "all" | "unused" | "redeemed" | "used" | "live" | "expired" | "cancelled";
type VoucherReconciliationAction = "keep" | "obsolete" | "investigate";
type LegacyVoucherScan = {
  importable: Array<{ code: string; routerProfile: string; comment: string | null }>;
  alreadySynced: number;
  needsReview: Array<{ code: string; routerProfile: string; reason: string }>;
  excluded: number;
};

interface VoucherView {
  id: string;
  code: string;
  profile: string;
  comment: string;
  bytesIn: number;
  bytesOut: number;
  redeemedAt: string | null;
  isUsed: boolean;
  isLive: boolean;
  state: VoucherState;
  active?: ActiveRow;
  /** RouterOS resource id. Absent when the ledger row is awaiting sync. */
  routerUserId?: string;
  deviceMac?: string | null;
  priceMmk?: number;
  expiresAt?: string | null;
}

type LastBatch = {
  codes: string[];
  planLabel: string;
  priceMmk: number;
  expiresAt: string | null;
  routerId: string;
};

type SheetVoucher = {
  code: string;
  planLabel: string;
  priceMmk?: number | null;
  expiresAt?: string | null;
};

function toNumber(v: unknown): number {
  const n = typeof v === "string" ? Number(v) : (v as number);
  return Number.isFinite(n) ? n : 0;
}
function humanBytes(v: unknown): string {
  const n = toNumber(v);
  if (n <= 0) return "0";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let x = n;
  while (x >= 1024 && i < u.length - 1) {
    x /= 1024;
    i++;
  }
  return `${x.toFixed(x < 10 ? 1 : 0)} ${u[i]}`;
}

function formatHandedOutTime(value: string | null): string {
  return value ? `${fmtDateTime(value)} MMT` : "—";
}

function csvCell(value: unknown): string {
  return `"${String(value ?? "").replaceAll('"', '""')}"`;
}

function VouchersPage() {
  const t = useT();
  const qc = useQueryClient();
  const fetchRouters = useServerFn(listRouters);
  const snapshot = useServerFn(getSnapshot);
  const listPlanRows = useServerFn(listPlans);
  const issue = useServerFn(issuePlanVouchers);
  const reconcileLedger = useServerFn(reconcileVoucherLedger);
  const scanLegacyVouchers = useServerFn(scanLegacyRouterVouchers);
  const importLegacyVouchers = useServerFn(importLegacyRouterVouchers);
  const del = useServerFn(deleteVoucher);
  const redeem = useServerFn(redeemVoucher);
  const listDbCodes = useServerFn(listVoucherCodes);
  const portalSettings = useServerFn(getPortalSettings);
  const fetchMe = useServerFn(getMe);
  const fetchPrintLayout = useServerFn(getVoucherPrintLayout);
  const kick = useServerFn(kickUser);

  const me = useQuery({ queryKey: ["me"], queryFn: () => fetchMe(), staleTime: 5 * 60_000 });
  const canManagePrintLayouts = canManageVoucherPrintLayouts(
    me.data?.roles,
    me.data?.isPlatformAdmin,
  );
  const canReconcileLedger = canReconcileVoucherLedger(me.data?.roles, me.data?.isPlatformAdmin);

  const routers = useQuery({ queryKey: ["routers"], queryFn: () => fetchRouters() });
  const plans = useQuery({ queryKey: ["portal-plans"], queryFn: () => listPlanRows() });
  const { site: selectedSite } = useSelectedSite();
  const siteRouters = useMemo(
    () =>
      (routers.data ?? []).filter(
        (r) => (selectedSite ? r.site_id === selectedSite.id : true) && !isVirtualRouter(r),
      ),
    [routers.data, selectedSite],
  );
  const [routerId, setRouterId] = useState("");
  const chosen =
    (routerId && siteRouters.some((r) => r.id === routerId) ? routerId : null) ||
    siteRouters[0]?.id ||
    "";
  const chosenRouter = siteRouters.find((r) => r.id === chosen);

  const snap = useQuery({
    queryKey: ["snapshot", chosen],
    queryFn: () => snapshot({ data: { routerId: chosen } }),
    enabled: !!chosen,
    refetchInterval: 20_000,
    refetchIntervalInBackground: false,
    staleTime: 10_000,
  });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["snapshot", chosen] });
    void qc.invalidateQueries({ queryKey: ["voucher-codes", chosen] });
    void qc.invalidateQueries({ queryKey: ["business-snapshot"] });
    void qc.invalidateQueries({ queryKey: ["revenue-dashboard"] });
    void qc.invalidateQueries({ queryKey: ["plan-performance"] });
    void qc.invalidateQueries({ queryKey: ["voucher-ledger"] });
  };

  const dbLedger = useQuery({
    queryKey: ["voucher-codes", chosen],
    queryFn: () => listDbCodes({ data: { routerId: chosen } }),
    enabled: !!chosen,
    staleTime: 15_000,
  });
  const portal = useQuery({
    queryKey: ["portal-settings"],
    queryFn: () => portalSettings(),
    staleTime: 60_000,
  });
  const printLayout = useQuery({
    queryKey: ["voucher-print-layout"],
    queryFn: () => fetchPrintLayout(),
    staleTime: 60_000,
  });

  // Stable identities: a fresh `?? []` each render would re-run every memo.
  const users = useMemo(
    () => sellableVoucherUsers((snap.data?.users ?? []) as UserRow[]),
    [snap.data?.users],
  );
  const active = useMemo(() => (snap.data?.active ?? []) as ActiveRow[], [snap.data?.active]);

  const activeByUser = useMemo(() => {
    const m: Record<string, ActiveRow> = {};
    for (const a of active) if (a.user) m[a.user.toUpperCase()] = a;
    return m;
  }, [active]);

  // The app ledger is the voucher inventory and financial source of truth.
  // RouterOS contributes only runtime counters and the live-session flag. This
  // prevents built-in RouterOS users (for example `admin`) from appearing as
  // MikroTik Magic vouchers or contributing to a voucher/revenue mismatch.
  const vouchers: VoucherView[] = useMemo(() => {
    const routerByCode = new Map(users.map((user) => [String(user.name).toUpperCase(), user]));
    return (dbLedger.data ?? []).map((ledger: VoucherLedgerRow) => {
      const user = routerByCode.get(String(ledger.code).toUpperCase());
      const state = voucherState(ledger);
      const a = activeByUser[String(ledger.code).toUpperCase()];
      return {
        id: ledger.id,
        code: ledger.code,
        profile: voucherPlanLabel(ledger, user?.profile),
        comment: user?.comment ?? "",
        bytesIn: toNumber(user?.["bytes-in"]),
        bytesOut: toNumber(user?.["bytes-out"]),
        redeemedAt: ledger.first_seen_at,
        isUsed: state === "used",
        isLive: state !== "cancelled" && !!a,
        state,
        active: a,
        routerUserId: user?.[".id"],
        deviceMac: ledger.device_mac ?? null,
        priceMmk: Number(ledger.price_mmk ?? 0),
        expiresAt: ledger.expires_at ?? null,
      };
    });
  }, [users, activeByUser, dbLedger.data]);

  const drift = useMemo(() => {
    const routerCodes = new Set(users.map((user) => String(user.name).toUpperCase()));
    const ledger = dbLedger.data ?? [];
    const ledgerOnly = ledger.filter(
      (row) =>
        !routerCodes.has(String(row.code).toUpperCase()) &&
        !["cancelled", "deleted"].includes(String(row.status).toLowerCase()),
    );
    const dbOnly = ledgerOnly.map((row) => String(row.code));
    const dbCodeSet = new Set(ledger.map((row) => String(row.code).toUpperCase()));
    const routerOnly = users
      .filter((user) => !dbCodeSet.has(String(user.name).toUpperCase()))
      .map((user) => user.name);
    return { dbOnly, ledgerOnly, routerOnly };
  }, [users, dbLedger.data]);

  // --- Generator state ---
  const [planId, setPlanId] = useState("");
  const [count, setCount] = useState(10);
  const [lastBatch, setLastBatch] = useState<LastBatch | null>(null);
  const activePlans = useMemo(
    () => (plans.data ?? []).filter((p) => p.status !== "inactive"),
    [plans.data],
  );

  useEffect(() => {
    const rows = activePlans;
    if (!rows.length) return;
    if (planId && rows.some((p) => p.id === planId)) return;
    setPlanId(rows[0]!.id);
  }, [activePlans, planId]);

  // --- Filter/search state ---
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmDel, setConfirmDel] = useState<string[] | null>(null);
  const [redeemTarget, setRedeemTarget] = useState<VoucherView | null>(null);
  const [redeemNote, setRedeemNote] = useState("");
  const [slip, setSlip] = useState<VoucherView | null>(null);
  const [reconcileOpen, setReconcileOpen] = useState(false);
  const [reconcileIds, setReconcileIds] = useState<Set<string>>(new Set());
  const [reconcileAction, setReconcileAction] = useState<VoucherReconciliationAction>("obsolete");
  const [reconcileReason, setReconcileReason] = useState("");
  const [legacyImportOpen, setLegacyImportOpen] = useState(false);
  const [legacyScan, setLegacyScan] = useState<LegacyVoucherScan | null>(null);
  const [legacySelectedCodes, setLegacySelectedCodes] = useState<Set<string>>(new Set());
  const [legacyPlanByCode, setLegacyPlanByCode] = useState<Record<string, string>>({});
  const [legacyReason, setLegacyReason] = useState("");

  const counts = useMemo(() => {
    const c = {
      all: vouchers.length,
      unused: 0,
      redeemed: 0,
      used: 0,
      live: 0,
      expired: 0,
      cancelled: 0,
    };
    for (const v of vouchers) {
      if (v.isLive) c.live++;
      if (v.state === "used") c.used++;
      else if (v.state === "active") c.redeemed++;
      else if (v.state === "expired") c.expired++;
      else if (v.state === "cancelled") c.cancelled++;
      else c.unused++;
    }
    return c;
  }, [vouchers]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return vouchers.filter((v) => {
      if (filter === "live" && !v.isLive) return false;
      if (filter === "used" && v.state !== "used") return false;
      if (filter === "redeemed" && v.state !== "active") return false;
      if (filter === "unused" && v.state !== "unused") return false;
      if (filter === "expired" && v.state !== "expired") return false;
      if (filter === "cancelled" && v.state !== "cancelled") return false;
      if (!q) return true;
      return (
        v.code.toLowerCase().includes(q) ||
        v.profile.toLowerCase().includes(q) ||
        v.comment.toLowerCase().includes(q) ||
        (v.deviceMac ?? "").toLowerCase().includes(q)
      );
    });
  }, [vouchers, filter, search]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageRows = useMemo(
    () => filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE),
    [filtered, safePage],
  );
  const selectablePageIds = useMemo(
    () => pageRows.filter((v) => Boolean(v.routerUserId)).map((v) => v.id),
    [pageRows],
  );
  const pageSelection = useMemo(
    () => pageSelectionState(selected, selectablePageIds),
    [selected, selectablePageIds],
  );
  const pageSelectRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (pageSelectRef.current) {
      pageSelectRef.current.indeterminate = pageSelection.partiallySelected;
    }
  }, [pageSelection.partiallySelected]);

  function toggleCurrentPage(checked: boolean) {
    setSelected((current) => updatePageSelection(current, selectablePageIds, checked));
  }

  useEffect(() => {
    setPage(1);
  }, [chosen, filter, search]);

  // --- Mutations ---
  const chosenPlan = (plans.data ?? []).find((p) => p.id === planId);
  const isVipPlan = Boolean(chosenPlan?.is_vip);
  const effectiveCount = isVipPlan ? 1 : count;
  const issueMut = useMutation({
    mutationFn: () => issue({ data: { planId, routerId: chosen, count: effectiveCount } }),
    onSuccess: (r) => {
      setLastBatch({
        codes: r.issued,
        planLabel: r.planLabel,
        priceMmk: r.priceMmk,
        expiresAt: r.expiresAt,
        routerId: r.routerId,
      });
      const failed = r.failed?.length ?? 0;
      if (failed > 0) {
        toast.message(`Generated ${r.issued.length}, ${failed} failed — landed on the board`, {
          description: r.failed?.[0]?.error,
        });
      } else {
        toast.success(
          `${r.issued.length} voucher${r.issued.length !== 1 ? "s" : ""} landed on the board`,
        );
      }
      invalidate();
      void qc.invalidateQueries({ queryKey: ["magic-go-live"] });
    },
    onError: (e: Error) => toast.error("Create failed", { description: toErrorMessage(e) }),
  });
  const delMut = useMutation({
    mutationFn: (voucher: VoucherView) => {
      if (!voucher.routerUserId) throw new Error("This voucher is not present on RouterOS yet.");
      return del({ data: { routerId: chosen, id: voucher.routerUserId } });
    },
    onSuccess: () => {
      invalidate();
    },
    onError: (e: Error) => toast.error("Delete failed", { description: toErrorMessage(e) }),
  });
  const redeemMut = useMutation({
    mutationFn: (v: { voucher: VoucherView; note?: string }) => {
      if (!v.voucher.routerUserId) throw new Error("This voucher is not present on RouterOS yet.");
      return redeem({ data: { routerId: chosen, id: v.voucher.routerUserId, note: v.note } });
    },
    onSuccess: (_r, v) => {
      toast.success("Marked as handed out");
      setSlip(v.voucher);
      setRedeemTarget(null);
      setRedeemNote("");
      invalidate();
    },
    onError: (e: Error) => toast.error("Hand-out failed", { description: toErrorMessage(e) }),
  });
  const kickMut = useMutation({
    mutationFn: (id: string) => kick({ data: { routerId: chosen, id } }),
    onSuccess: () => {
      toast.success("Session kicked");
      invalidate();
    },
    onError: (e: Error) => toast.error("Kick failed", { description: toErrorMessage(e) }),
  });
  const reconcileMut = useMutation({
    mutationFn: () =>
      reconcileLedger({
        data: {
          voucherIds: [...reconcileIds],
          action: reconcileAction,
          reason: reconcileReason.trim(),
        },
      }),
    onSuccess: (result) => {
      const processed = Number(
        (result as { processed?: number } | null)?.processed ?? reconcileIds.size,
      );
      const changed = Number((result as { changed?: number } | null)?.changed ?? 0);
      toast.success(
        reconcileAction === "obsolete"
          ? `${changed || processed} voucher${processed === 1 ? "" : "s"} marked obsolete`
          : `${processed} voucher${processed === 1 ? "" : "s"} review recorded`,
      );
      setReconcileOpen(false);
      setReconcileIds(new Set());
      setReconcileReason("");
      invalidate();
    },
    onError: (e: Error) =>
      toast.error("Reconciliation was not recorded", { description: toErrorMessage(e) }),
  });

  const legacyScanMut = useMutation({
    mutationFn: () => scanLegacyVouchers({ data: { routerId: chosen } }),
    onSuccess: (result) => {
      const scan = result as LegacyVoucherScan;
      setLegacyScan(scan);
      setLegacySelectedCodes(new Set(scan.importable.map((row) => row.code)));
      setLegacyPlanByCode(
        Object.fromEntries(scan.importable.map((row) => [row.code, activePlans[0]?.id ?? ""])),
      );
      setLegacyReason("");
      setLegacyImportOpen(true);
    },
    onError: (e: Error) => toast.error("Device scan failed", { description: toErrorMessage(e) }),
  });
  const legacyImportMut = useMutation({
    mutationFn: () => {
      if (!legacyScan) throw new Error("Scan vouchers on the device first.");
      const imports = legacyScan.importable
        .filter((row) => legacySelectedCodes.has(row.code))
        .map((row) => ({
          code: row.code,
          routerProfile: row.routerProfile,
          planId: legacyPlanByCode[row.code] ?? "",
        }));
      return importLegacyVouchers({
        data: { routerId: chosen, imports, reason: legacyReason.trim() },
      });
    },
    onSuccess: (result) => {
      const imported = Number((result as { imported?: number } | null)?.imported ?? 0);
      toast.success(`${imported} legacy voucher${imported === 1 ? "" : "s"} imported`);
      setLegacyImportOpen(false);
      setLegacyScan(null);
      setLegacySelectedCodes(new Set());
      invalidate();
    },
    onError: (e: Error) =>
      toast.error("Legacy import was not recorded", { description: toErrorMessage(e) }),
  });

  function openReconciliation() {
    setReconcileIds(new Set(drift.ledgerOnly.map((row) => row.id)));
    setReconcileAction("obsolete");
    setReconcileReason("");
    setReconcileOpen(true);
  }

  function toggleReconciliationVoucher(id: string) {
    setReconcileIds((previous) => {
      const next = new Set(previous);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleLegacyVoucher(code: string) {
    setLegacySelectedCodes((previous) => {
      const next = new Set(previous);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
  }

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  function copyOne(code: string) {
    void copyText(code).then((ok) => {
      if (ok) toast.success("Code copied");
      else toast.error("Copy failed");
    });
  }
  function copySelected() {
    const list = filtered
      .filter((v) => selected.has(v.id))
      .map((v) => v.code)
      .join("\n");
    if (!list) return;
    void copyText(list).then((ok) => {
      if (ok) toast.success(`${selected.size} codes copied`);
      else toast.error("Copy failed");
    });
  }
  function exportCsv(rows: VoucherView[], name: string) {
    const csv =
      "code,plan,price_mmk,status,redeemed_at,used_down,used_up,comment\n" +
      rows
        .map((v) => {
          const status = v.isLive
            ? "live"
            : v.isUsed
              ? "used"
              : v.redeemedAt
                ? "redeemed"
                : "unused";
          return [
            csvCell(v.code),
            csvCell(v.profile),
            csvCell(v.priceMmk ?? ""),
            csvCell(status),
            csvCell(v.redeemedAt ? formatHandedOutTime(v.redeemedAt) : ""),
            v.bytesIn,
            v.bytesOut,
            csvCell(v.comment),
          ].join(",");
        })
        .join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${name}-${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const pageHeader = (
    <header className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 className="text-2xl font-semibold">{t.ui("Vouchers")}</h1>
        <p className="text-sm text-muted-foreground">
          {t.copy("Plans · bulk codes · printable slips · live status.")}
        </p>
      </div>
      {canManagePrintLayouts ? (
        <Link
          to="/app/voucher-layouts"
          className="rounded-md border border-border px-3 py-2 text-sm font-medium hover:border-primary hover:text-primary"
        >
          Print layouts
        </Link>
      ) : null}
    </header>
  );

  if (!siteRouters.length) {
    return (
      <div className="space-y-5">
        {pageHeader}
        <div className="panel p-6 text-center">
          <p className="text-sm text-muted-foreground">
            {selectedSite
              ? `No routers on site “${selectedSite.name}”. Pick another site or add a router.`
              : "Add a router first to create vouchers."}
          </p>
          <Link
            to="/app/routers"
            className="mt-3 inline-block rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground"
          >
            Add a router
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {pageHeader}
      {siteRouters.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-xs text-muted-foreground" htmlFor="voucher-router">
            Router
          </label>
          <select
            id="voucher-router"
            value={chosen}
            onChange={(e) => setRouterId(e.target.value)}
            className="h-9 rounded-md border border-border bg-surface px-2 text-xs"
            aria-label="Select router"
          >
            {siteRouters.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </div>
      )}
      {chosen ? (
        <HotspotGuestReadyBanner
          routerId={chosen}
          routerName={chosenRouter?.name}
          context="vouchers"
        />
      ) : null}

      {snap.isError && (
        <section
          className="rounded-xl border border-red-500/40 bg-red-500/10 p-4 text-sm text-red-100"
          role="alert"
        >
          <p className="font-medium">Could not read hotspot users from the router</p>
          <p className="mt-1 text-xs opacity-90">
            {(snap.error as Error)?.message ||
              "Snapshot failed — empty stock here usually means the board is unreachable, not that you have zero vouchers."}
          </p>
          <Link
            to="/app/routers"
            className="mt-2 inline-block text-xs font-semibold text-primary hover:underline"
          >
            Open Routers →
          </Link>
        </section>
      )}

      {(drift.dbOnly.length > 0 || drift.routerOnly.length > 0) && (
        <section
          className="rounded-xl border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-100"
          role="status"
        >
          <p className="font-medium">Ledger mismatch on this router</p>
          <p className="mt-1 text-xs text-amber-100/80">
            Voucher inventory and revenue read the app ledger. RouterOS contributes live session
            state only, so RouterOS-only accounts are excluded from voucher totals and revenue.
          </p>
          {drift.dbOnly.length > 0 && (
            <p className="mt-2 text-xs">
              <span className="font-semibold">Ledger only ({drift.dbOnly.length}):</span>{" "}
              {drift.dbOnly.slice(0, 8).join(", ")}
              {drift.dbOnly.length > 8 ? "…" : ""} — review before marking an unused code obsolete
              or issuing a replacement.
            </p>
          )}
          {drift.routerOnly.length > 0 && (
            <p className="mt-2 text-xs">
              <span className="font-semibold">Router only ({drift.routerOnly.length}):</span>{" "}
              {drift.routerOnly.slice(0, 8).join(", ")}
              {drift.routerOnly.length > 8 ? "…" : ""} — created outside the app (WinBox/MCP) or
              missing ledger row.
            </p>
          )}
          {drift.routerOnly.length > 0 &&
            (canReconcileLedger ? (
              <button
                type="button"
                onClick={() => legacyScanMut.mutate()}
                disabled={legacyScanMut.isPending}
                className="mt-3 mr-2 rounded-md border border-amber-400/60 px-3 py-2 text-xs font-semibold text-amber-50 hover:bg-amber-400/10 disabled:opacity-60"
              >
                {legacyScanMut.isPending ? "Scanning device…" : "Scan vouchers on device"}
              </button>
            ) : (
              <p className="mt-3 text-[11px] text-amber-100/75">
                A Primary or Developer can scan and import router-only voucher stock after review.
              </p>
            ))}
          {drift.dbOnly.length > 0 &&
            (canReconcileLedger ? (
              <button
                type="button"
                onClick={openReconciliation}
                className="mt-3 rounded-md border border-amber-400/60 px-3 py-2 text-xs font-semibold text-amber-50 hover:bg-amber-400/10"
              >
                Reconcile mismatch
              </button>
            ) : (
              <p className="mt-3 text-[11px] text-amber-100/75">
                A Primary or Developer can reconcile ledger-only codes after review.
              </p>
            ))}
        </section>
      )}

      <div id="plans">
        <PlansPanel
          routerId={chosen}
          routerName={chosenRouter?.name}
          hideRouterPicker
          onIssued={invalidate}
        />
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-7">
        <Stat label="Total" value={counts.all} tone="default" />
        <Stat label="Unused stock" value={counts.unused} tone="primary" />
        <Stat label="Handed out" value={counts.redeemed} tone="amber" />
        <Stat label="Used" value={counts.used} tone="muted" />
        <Stat label="Live now" value={counts.live} tone="green" />
        <Stat label="Expired" value={counts.expired} tone="muted" />
        <Stat label="Cancelled" value={counts.cancelled} tone="muted" />
      </div>

      {/* Generator */}
      <section id="create-codes" className="panel p-4 sm:p-5">
        <div className="mb-3">
          <h2 className="text-sm font-semibold">Create vouchers</h2>
          <p className="text-[11px] text-muted-foreground">
            Codes are bound to the selected plan. The HotSpot profile is created automatically on
            Generate. Guests still need Hotspot Wi‑Fi (SSID) set up on Routers.
          </p>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">Plan</span>
            <select
              className="h-10 w-full rounded-md border border-border bg-surface px-2 text-sm"
              value={planId}
              onChange={(e) => setPlanId(e.target.value)}
              aria-label="Voucher plan"
            >
              {activePlans.length === 0 && <option value="">No active plans</option>}
              {activePlans.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label}
                  {p.is_vip ? " (VIP)" : ""}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">
              {isVipPlan ? "Count (VIP = 1)" : "Count (1–100)"}
            </span>
            <Input
              type="number"
              min={1}
              max={100}
              value={effectiveCount}
              disabled={isVipPlan}
              onChange={(e) => setCount(Math.max(1, Math.min(100, Number(e.target.value) || 1)))}
              className="h-10"
            />
          </label>
          <div className="flex items-end">
            <button
              onClick={() => issueMut.mutate()}
              disabled={
                issueMut.isPending ||
                !planId ||
                !chosen ||
                (isVipPlan &&
                  !String(
                    (chosenPlan as { manual_code?: string | null } | undefined)?.manual_code ?? "",
                  ).trim())
              }
              className="h-10 w-full rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground disabled:opacity-60"
            >
              {issueMut.isPending ? "Generating…" : `Generate ${effectiveCount}`}
            </button>
          </div>
        </div>
        {isVipPlan &&
          !String(
            (chosenPlan as { manual_code?: string | null } | undefined)?.manual_code ?? "",
          ).trim() && (
            <p className="mt-2 text-[11px] text-amber-200">
              Set a VIP code on the plan before Generate.
            </p>
          )}

        {lastBatch && lastBatch.codes.length > 0 && (
          <div className="mt-4 rounded-lg border border-border bg-surface p-3">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">
                Last batch · {lastBatch.codes.length} created · {lastBatch.planLabel}
              </span>
              <div className="flex gap-2">
                <button
                  onClick={() => {
                    void copyText(lastBatch.codes.join("\n")).then((ok) => {
                      if (ok) toast.success("Codes copied");
                      else toast.error("Copy failed");
                    });
                  }}
                  className="h-8 rounded-md border border-border px-2 text-xs"
                >
                  Copy codes
                </button>
                <button
                  onClick={() => {
                    const csv =
                      "code,plan,price_mmk\n" +
                      lastBatch.codes
                        .map((code) =>
                          [
                            csvCell(code),
                            csvCell(lastBatch.planLabel),
                            csvCell(lastBatch.priceMmk),
                          ].join(","),
                        )
                        .join("\n");
                    const blob = new Blob([csv], { type: "text/csv" });
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement("a");
                    a.href = url;
                    a.download = `batch-${Date.now()}.csv`;
                    a.click();
                    URL.revokeObjectURL(url);
                  }}
                  className="h-8 rounded-md border border-border px-2 text-xs"
                >
                  Download CSV
                </button>
                <button
                  onClick={() =>
                    printSheet(
                      lastBatch.codes.map((code) => ({
                        code,
                        planLabel: lastBatch.planLabel,
                        priceMmk: lastBatch.priceMmk,
                        expiresAt: lastBatch.expiresAt,
                      })),
                      siteRouters.find((router) => router.id === lastBatch.routerId)?.name ??
                        chosenRouter?.name ??
                        "Hotspot",
                      printLayout.data ?? DEFAULT_VOUCHER_PRINT_LAYOUT,
                    )
                  }
                  className="h-8 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground"
                >
                  Print A4 cut sheet
                </button>
                <button
                  onClick={() =>
                    printThermalStrip(
                      lastBatch.codes.map((code) => ({
                        code,
                        planLabel: lastBatch.planLabel,
                        priceMmk: lastBatch.priceMmk,
                        expiresAt: lastBatch.expiresAt,
                      })),
                      printLayout.data ?? DEFAULT_VOUCHER_PRINT_LAYOUT,
                    )
                  }
                  className="h-8 rounded-md border border-border px-2 text-xs font-medium hover:border-primary hover:text-primary"
                >
                  Print thermal strip
                </button>
              </div>
            </div>
            <div className="max-h-40 overflow-auto rounded-md bg-surface-elevated p-2 font-mono text-[11px]">
              {lastBatch.codes.map((code) => (
                <div key={code}>{code}</div>
              ))}
            </div>
          </div>
        )}
      </section>

      {/* Filter + list */}
      <section className="panel overflow-hidden">
        <div className="space-y-3 border-b border-border p-3">
          <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 sm:flex sm:flex-wrap">
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search code, plan, comment, or device MAC"
              className="h-10 sm:h-9 sm:w-72"
              aria-label="Search vouchers"
            />
            <button
              onClick={() => exportCsv(filtered, "vouchers")}
              className="h-10 shrink-0 rounded-md border border-border px-3 text-xs sm:h-9"
              disabled={!filtered.length}
            >
              Export CSV
            </button>
          </div>

          <div className="flex flex-wrap gap-1.5">
            <Chip active={filter === "all"} onClick={() => setFilter("all")}>
              All · {counts.all}
            </Chip>
            <Chip active={filter === "unused"} onClick={() => setFilter("unused")}>
              Unused · {counts.unused}
            </Chip>
            <Chip active={filter === "redeemed"} onClick={() => setFilter("redeemed")}>
              Handed out · {counts.redeemed}
            </Chip>
            <Chip active={filter === "used"} onClick={() => setFilter("used")}>
              Used · {counts.used}
            </Chip>
            <Chip active={filter === "live"} onClick={() => setFilter("live")}>
              Live · {counts.live}
            </Chip>
            <Chip active={filter === "expired"} onClick={() => setFilter("expired")}>
              Expired · {counts.expired}
            </Chip>
            <Chip active={filter === "cancelled"} onClick={() => setFilter("cancelled")}>
              Cancelled · {counts.cancelled}
            </Chip>
          </div>

          {filtered.length > 0 && (
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <button
                type="button"
                onClick={() => toggleCurrentPage(!pageSelection.allSelected)}
                disabled={!selectablePageIds.length}
                className="h-8 rounded-md border border-border px-2.5 disabled:opacity-50"
              >
                {pageSelection.allSelected ? "Clear page" : "Select page"}
              </button>
              <span>
                Page {safePage} only · {selectablePageIds.length} printable voucher
                {selectablePageIds.length === 1 ? "" : "s"}
              </span>
            </div>
          )}

          {selected.size > 0 && (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-primary/5 px-3 py-2 text-xs">
              <span className="font-medium">{selected.size} selected</span>
              <div className="flex gap-2">
                <button
                  onClick={copySelected}
                  className="h-8 rounded-md border border-border bg-surface px-2"
                >
                  Copy codes
                </button>
                <button
                  onClick={() => {
                    const rows = filtered
                      .filter((v) => selected.has(v.id))
                      .map((v) => ({
                        code: v.code,
                        planLabel: v.profile,
                        priceMmk: v.priceMmk,
                        expiresAt: v.expiresAt,
                      }));
                    printSheet(
                      rows,
                      chosenRouter?.name ?? "Hotspot",
                      printLayout.data ?? DEFAULT_VOUCHER_PRINT_LAYOUT,
                    );
                  }}
                  className="h-8 rounded-md border border-border bg-surface px-2"
                >
                  Print A4 cut sheet
                </button>
                <button
                  onClick={() => {
                    const rows = filtered
                      .filter((v) => selected.has(v.id))
                      .map((v) => ({
                        code: v.code,
                        planLabel: v.profile,
                        priceMmk: v.priceMmk,
                        expiresAt: v.expiresAt,
                      }));
                    printThermalStrip(
                      rows,
                      printLayout.data ?? DEFAULT_VOUCHER_PRINT_LAYOUT,
                    );
                  }}
                  className="h-8 rounded-md border border-border bg-surface px-2"
                >
                  Print thermal strip
                </button>
                <button
                  onClick={() => setConfirmDel([...selected])}
                  className="h-8 rounded-md border border-red-500/50 px-2 text-red-600 dark:text-red-400"
                >
                  Delete
                </button>
                <button
                  onClick={() => setSelected(new Set())}
                  className="h-8 px-2 text-muted-foreground"
                >
                  Clear
                </button>
              </div>
            </div>
          )}
        </div>

        {snap.isLoading ? (
          <div className="space-y-2 p-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="h-14 animate-pulse rounded-md bg-surface-elevated/60" />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState
            hasAny={vouchers.length > 0}
            onCreate={() => issueMut.mutate()}
            canCreate={Boolean(planId && chosen)}
          />
        ) : (
          <>
            {/* Mobile cards */}
            <ul className="divide-y divide-border md:hidden">
              {pageRows.map((v) => (
                <li key={v.id} className="p-3">
                  <VoucherCard
                    v={v}
                    selected={selected.has(v.id)}
                    onSelect={() => toggle(v.id)}
                    onCopy={() => copyOne(v.code)}
                    onRedeem={() => setRedeemTarget(v)}
                    onSlip={() => setSlip(v)}
                    onKick={() => v.active && kickMut.mutate(v.active[".id"])}
                    onDelete={() => setConfirmDel([v.id])}
                  />
                </li>
              ))}
            </ul>

            {/* Desktop table */}
            <div className="hidden max-h-[560px] overflow-auto md:block">
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-surface-elevated text-muted-foreground">
                  <tr>
                    <Th className="w-8">
                      <input
                        type="checkbox"
                        ref={pageSelectRef}
                        aria-label={`Select page ${safePage}`}
                        checked={pageSelection.allSelected}
                        disabled={!selectablePageIds.length}
                        onChange={(e) => toggleCurrentPage(e.target.checked)}
                      />
                    </Th>
                    <Th>Code</Th>
                    <Th>Plan</Th>
                    <Th>Status</Th>
                    <Th>Used</Th>
                    <Th>Handed out</Th>
                    <Th className="text-right">Actions</Th>
                  </tr>
                </thead>
                <tbody>
                  {pageRows.map((v) => (
                    <tr key={v.id} className="border-t border-border hover:bg-surface-elevated/40">
                      <Td>
                        <input
                          type="checkbox"
                          aria-label={`Select ${v.code}`}
                          checked={selected.has(v.id)}
                          disabled={!v.routerUserId}
                          onChange={() => toggle(v.id)}
                        />
                      </Td>
                      <Td>
                        <button
                          onClick={() => copyOne(v.code)}
                          className="rounded px-1 font-mono text-sm font-semibold hover:bg-surface-elevated"
                          title="Click to copy"
                        >
                          {v.code}
                        </button>
                      </Td>
                      <Td className="text-muted-foreground">{v.profile}</Td>
                      <Td>
                        <StatusPill v={v} />
                      </Td>
                      <Td className="font-mono text-[11px]">
                        {v.isUsed
                          ? `${humanBytes(v.bytesIn)} ↓ / ${humanBytes(v.bytesOut)} ↑`
                          : "—"}
                      </Td>
                      <Td className="text-muted-foreground">{formatHandedOutTime(v.redeemedAt)}</Td>
                      <Td className="text-right">
                        <div className="flex flex-wrap justify-end gap-1">
                          <button
                            onClick={() => setSlip(v)}
                            className="rounded-md border border-border px-2 py-1 text-xs hover:bg-surface-elevated"
                          >
                            Slip
                          </button>
                          {v.routerUserId && v.state === "unused" && (
                            <button
                              onClick={() => setRedeemTarget(v)}
                              className="rounded-md border border-amber-500/50 px-2 py-1 text-xs text-amber-600 dark:text-amber-400"
                            >
                              Hand out
                            </button>
                          )}
                          {v.isLive && v.active && (
                            <button
                              onClick={() => kickMut.mutate(v.active![".id"])}
                              className="rounded-md border border-border px-2 py-1 text-xs"
                            >
                              Kick
                            </button>
                          )}
                          {v.routerUserId ? (
                            <button
                              onClick={() => setConfirmDel([v.id])}
                              className="rounded-md border border-red-500/50 px-2 py-1 text-xs text-red-600 dark:text-red-400"
                            >
                              Delete
                            </button>
                          ) : (
                            <span className="px-2 py-1 text-[10px] text-amber-600 dark:text-amber-300">
                              Awaiting sync
                            </span>
                          )}
                        </div>
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <RecordPagination page={safePage} total={filtered.length} onPageChange={setPage} />
          </>
        )}
      </section>

      {/* Hand-out dialog */}
      <Dialog
        open={!!redeemTarget}
        onOpenChange={(o) => {
          if (!o) {
            setRedeemTarget(null);
            setRedeemNote("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hand out voucher</DialogTitle>
            <DialogDescription>
              {t.copy("Mark")} <span className="font-mono font-semibold">{redeemTarget?.code}</span>{" "}
              {t.copy(
                "as handed to a customer. This stamps the voucher comment and opens a printable slip; it does not record income. Record the settled cash or online sale in Payments first.",
              )}
            </DialogDescription>
          </DialogHeader>
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">
              Note (optional — e.g. customer name, table #)
            </span>
            <Input
              value={redeemNote}
              onChange={(e) => setRedeemNote(e.target.value)}
              placeholder="Table 4"
              maxLength={80}
              autoFocus
            />
          </label>
          <DialogFooter>
            <button onClick={() => setRedeemTarget(null)} className="h-10 rounded-md px-3 text-sm">
              Cancel
            </button>
            <button
              onClick={() =>
                redeemTarget &&
                redeemMut.mutate({ voucher: redeemTarget, note: redeemNote.trim() || undefined })
              }
              disabled={redeemMut.isPending}
              className="h-10 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-60"
            >
              {redeemMut.isPending ? t.action("Saving…") : t.action("Mark handed out & print")}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Ledger reconciliation is app-ledger-only; it does not call RouterOS. */}
      <Dialog
        open={reconcileOpen}
        onOpenChange={(open) => {
          if (!open && !reconcileMut.isPending) setReconcileOpen(false);
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Reconcile voucher ledger mismatch</DialogTitle>
            <DialogDescription>
              Preview the app-ledger-only codes before recording a decision. This flow never changes
              RouterOS and never deletes a voucher record.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-950 dark:text-amber-100">
            Marking a code obsolete changes only an unused, unpaid ledger row to Cancelled. Used or
            settled vouchers are blocked and must follow the payment/refund workflow.
          </div>
          <div className="max-h-56 overflow-y-auto rounded-md border border-border">
            {drift.ledgerOnly.map((row) => (
              <label
                key={row.id}
                className="flex cursor-pointer items-center gap-3 border-b border-border px-3 py-2 last:border-b-0"
              >
                <input
                  type="checkbox"
                  checked={reconcileIds.has(row.id)}
                  onChange={() => toggleReconciliationVoucher(row.id)}
                  className="h-4 w-4"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-sm font-semibold">{row.code}</span>
                  <span className="block text-[11px] text-muted-foreground">
                    {row.plan_label || row.plan_key} · {row.status} · issued{" "}
                    {new Date(row.created_at).toLocaleDateString()}
                  </span>
                </span>
                <span className="text-[11px] text-muted-foreground">Not found on RouterOS</span>
              </label>
            ))}
          </div>
          <label className="block text-xs">
            <span className="mb-1 block font-medium">Decision</span>
            <select
              value={reconcileAction}
              onChange={(event) =>
                setReconcileAction(event.target.value as VoucherReconciliationAction)
              }
              className="h-10 w-full rounded-md border border-border bg-surface px-3 text-sm"
            >
              <option value="obsolete">Mark selected unused codes obsolete</option>
              <option value="keep">Keep in ledger — record review only</option>
              <option value="investigate">Investigate later — record review only</option>
            </select>
          </label>
          <label className="block text-xs">
            <span className="mb-1 block font-medium">Reason required</span>
            <textarea
              value={reconcileReason}
              onChange={(event) => setReconcileReason(event.target.value)}
              maxLength={500}
              placeholder="For example: Router was reset after a replacement; the unused paper slips were destroyed."
              className="min-h-24 w-full rounded-md border border-border bg-surface p-3 text-sm"
            />
            <span className="mt-1 block text-[11px] text-muted-foreground">
              Your account, timestamp, reason, and before/after status are retained in the audit
              trail.
            </span>
          </label>
          <div className="rounded-md bg-surface-elevated p-3 text-xs text-muted-foreground">
            Need replacements? Finish this review, then use{" "}
            <a href="#create-codes" className="font-semibold text-primary underline">
              Create vouchers
            </a>{" "}
            to issue new codes. Reissuing is a separate, router-changing step.
          </div>
          <DialogFooter>
            <button
              type="button"
              onClick={() => setReconcileOpen(false)}
              disabled={reconcileMut.isPending}
              className="h-10 rounded-md px-3 text-sm disabled:opacity-60"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => reconcileMut.mutate()}
              disabled={
                reconcileMut.isPending ||
                reconcileIds.size === 0 ||
                reconcileReason.trim().length < 3
              }
              className="h-10 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-60"
            >
              {reconcileMut.isPending
                ? "Recording…"
                : reconcileAction === "obsolete"
                  ? `Mark ${reconcileIds.size} obsolete`
                  : `Record review for ${reconcileIds.size}`}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Router-only legacy import. The scan and import both re-read RouterOS; neither changes it. */}
      <Dialog
        open={legacyImportOpen}
        onOpenChange={(open) => {
          if (!open && !legacyImportMut.isPending) setLegacyImportOpen(false);
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Import scanned router vouchers</DialogTitle>
            <DialogDescription>
              This creates app-ledger inventory only. It does not create RouterOS accounts, change
              the router, or reconstruct historic revenue.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-950 dark:text-amber-100">
            Each selected code is freshly checked again immediately before import. Accounts with
            RouterOS historic usage remain review-only so a past sale is never treated as new app
            revenue.
          </div>
          {legacyScan ? (
            <>
              <p className="text-xs text-muted-foreground">
                {legacyScan.importable.length} importable · {legacyScan.alreadySynced} already in
                ledger · {legacyScan.needsReview.length} need review · {legacyScan.excluded} system
                or trial accounts excluded
              </p>
              <div className="max-h-56 overflow-y-auto rounded-md border border-border">
                {legacyScan.importable.map((row) => (
                  <label
                    key={row.code}
                    className="flex items-center gap-3 border-b border-border px-3 py-2 last:border-b-0"
                  >
                    <input
                      type="checkbox"
                      checked={legacySelectedCodes.has(row.code)}
                      onChange={() => toggleLegacyVoucher(row.code)}
                      className="h-4 w-4"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-mono text-sm font-semibold">
                        {row.code}
                      </span>
                      <span className="block text-[11px] text-muted-foreground">
                        Router profile: {row.routerProfile}
                      </span>
                    </span>
                    <select
                      aria-label={`App plan for ${row.code}`}
                      value={legacyPlanByCode[row.code] ?? ""}
                      onChange={(event) =>
                        setLegacyPlanByCode((previous) => ({
                          ...previous,
                          [row.code]: event.target.value,
                        }))
                      }
                      className="h-9 max-w-44 rounded-md border border-border bg-surface px-2 text-xs"
                    >
                      <option value="">Choose app plan</option>
                      {activePlans.map((plan) => (
                        <option key={plan.id} value={plan.id}>
                          {plan.label}
                        </option>
                      ))}
                    </select>
                  </label>
                ))}
                {legacyScan.importable.length === 0 ? (
                  <p className="p-3 text-sm text-muted-foreground">
                    No unused, router-only voucher accounts are safe to import.
                  </p>
                ) : null}
              </div>
              {legacyScan.needsReview.length > 0 ? (
                <p className="text-[11px] text-muted-foreground">
                  Review-only examples:{" "}
                  {legacyScan.needsReview
                    .slice(0, 3)
                    .map((row) => `${row.code} (${row.reason})`)
                    .join(", ")}
                  {legacyScan.needsReview.length > 3 ? "…" : ""}
                </p>
              ) : null}
            </>
          ) : null}
          <label className="block text-xs">
            <span className="mb-1 block font-medium">Reason required</span>
            <textarea
              value={legacyReason}
              onChange={(event) => setLegacyReason(event.target.value)}
              maxLength={500}
              placeholder="For example: Initial audited import of unused paper vouchers created before MikroMagic."
              className="min-h-20 w-full rounded-md border border-border bg-surface p-3 text-sm"
            />
          </label>
          <DialogFooter>
            <button
              type="button"
              onClick={() => setLegacyImportOpen(false)}
              disabled={legacyImportMut.isPending}
              className="h-10 rounded-md px-3 text-sm disabled:opacity-60"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => legacyImportMut.mutate()}
              disabled={
                legacyImportMut.isPending ||
                legacySelectedCodes.size === 0 ||
                legacyReason.trim().length < 3 ||
                [...legacySelectedCodes].some((code) => !legacyPlanByCode[code])
              }
              className="h-10 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-60"
            >
              {legacyImportMut.isPending
                ? "Rechecking & importing…"
                : `Import ${legacySelectedCodes.size} selected`}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Slip / QR dialog */}
      <SlipDialog
        v={slip}
        router={chosenRouter?.name ?? "Hotspot"}
        layout={
          printLayout.data ?? {
            ...DEFAULT_VOUCHER_PRINT_LAYOUT,
            business_name: portal.data?.business_name ?? DEFAULT_VOUCHER_PRINT_LAYOUT.business_name,
            support_contact:
              portal.data?.seller_phone ?? DEFAULT_VOUCHER_PRINT_LAYOUT.support_contact,
            terms: portal.data?.terms ?? DEFAULT_VOUCHER_PRINT_LAYOUT.terms,
          }
        }
        onClose={() => setSlip(null)}
      />

      {/* Delete confirm */}
      <AlertDialog open={!!confirmDel} onOpenChange={(o) => !o && setConfirmDel(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {confirmDel?.length} voucher{confirmDel && confirmDel.length > 1 ? "s" : ""}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the hotspot user
              {confirmDel && confirmDel.length > 1 ? "s" : ""} from the router. Any active session
              will end at the next re-auth.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                const ids = confirmDel ?? [];
                setConfirmDel(null);
                setSelected(new Set());
                const selectedVouchers = vouchers.filter(
                  (voucher) => ids.includes(voucher.id) && voucher.routerUserId,
                );
                void Promise.all(selectedVouchers.map((voucher) => delMut.mutateAsync(voucher)))
                  .then(() => toast.success(`Deleted ${selectedVouchers.length}`))
                  .catch((e: Error) =>
                    toast.error("Delete failed", { description: toErrorMessage(e) }),
                  );
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/* ---------- subcomponents ---------- */

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "default" | "primary" | "amber" | "muted" | "green";
}) {
  const toneCls =
    tone === "primary"
      ? "text-primary"
      : tone === "amber"
        ? "text-amber-600 dark:text-amber-400"
        : tone === "green"
          ? "text-emerald-600 dark:text-emerald-400"
          : tone === "muted"
            ? "text-muted-foreground"
            : "";
  return (
    <div className="rounded-lg border border-border bg-surface px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className={`text-xl font-semibold ${toneCls}`}>{value}</div>
    </div>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`min-h-11 shrink-0 rounded-full border px-3 text-[11px] font-medium transition sm:min-h-8 ${
        active
          ? "border-primary bg-primary text-primary-foreground"
          : "border-border bg-surface text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

function StatusPill({ v }: { v: VoucherView }) {
  if (v.state === "cancelled")
    return (
      <Pill dot="bg-rose-500" text="text-rose-700 dark:text-rose-300" bg="bg-rose-500/10">
        Cancelled
      </Pill>
    );
  if (v.state === "expired")
    return (
      <Pill dot="bg-slate-400" text="text-slate-600 dark:text-slate-300" bg="bg-slate-500/10">
        Expired
      </Pill>
    );
  if (v.isLive)
    return (
      <Pill
        dot="bg-emerald-500 animate-pulse"
        text="text-emerald-700 dark:text-emerald-300"
        bg="bg-emerald-500/10"
      >
        Live
      </Pill>
    );
  if (v.isUsed)
    return (
      <Pill dot="bg-slate-400" text="text-slate-600 dark:text-slate-300" bg="bg-slate-500/10">
        Used
      </Pill>
    );
  if (v.redeemedAt)
    return (
      <Pill dot="bg-amber-500" text="text-amber-700 dark:text-amber-300" bg="bg-amber-500/10">
        Handed out
      </Pill>
    );
  return (
    <Pill dot="bg-primary" text="text-primary" bg="bg-primary/10">
      Unused
    </Pill>
  );
}
function Pill({
  dot,
  text,
  bg,
  children,
}: {
  dot: string;
  text: string;
  bg: string;
  children: React.ReactNode;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${bg} ${text}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${dot}`} />
      {children}
    </span>
  );
}

function VoucherCard({
  v,
  selected,
  onSelect,
  onCopy,
  onRedeem,
  onSlip,
  onKick,
  onDelete,
}: {
  v: VoucherView;
  selected: boolean;
  onSelect: () => void;
  onCopy: () => void;
  onRedeem: () => void;
  onSlip: () => void;
  onKick: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      className={`space-y-3 rounded-lg border border-border bg-surface p-3 ${selected ? "ring-2 ring-primary" : ""}`}
    >
      <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3">
        <input
          type="checkbox"
          className="mt-1 h-5 w-5"
          checked={selected}
          disabled={!v.routerUserId}
          onChange={onSelect}
          aria-label={`Select ${v.code}`}
        />
        <div className="min-w-0">
          <button
            onClick={onCopy}
            className="block truncate rounded px-1 font-mono text-base font-semibold hover:bg-surface-elevated"
            title="Copy"
          >
            {v.code}
          </button>
          <div className="mt-1 text-[11px] text-muted-foreground">
            {v.profile}
            {v.redeemedAt ? ` · ${formatHandedOutTime(v.redeemedAt)}` : ""}
          </div>
        </div>
        <StatusPill v={v} />
      </div>
      {v.isUsed && (
        <div className="rounded-md bg-surface-elevated px-2 py-1 text-[11px] font-mono">
          {humanBytes(v.bytesIn)} ↓ / {humanBytes(v.bytesOut)} ↑
        </div>
      )}
      <div className="grid grid-cols-2 gap-2">
        <button onClick={onSlip} className="h-11 rounded-md border border-border text-sm">
          Slip / QR
        </button>
        {!v.routerUserId ? (
          <span className="flex h-11 items-center justify-center rounded-md border border-amber-500/40 px-2 text-center text-xs text-amber-700 dark:text-amber-200">
            Awaiting sync
          </span>
        ) : v.state === "unused" ? (
          <button
            onClick={onRedeem}
            className="h-11 rounded-md bg-primary text-sm font-medium text-primary-foreground"
          >
            Hand out
          </button>
        ) : v.isLive ? (
          <button onClick={onKick} className="h-11 rounded-md border border-border text-sm">
            Kick session
          </button>
        ) : (
          <button onClick={onCopy} className="h-11 rounded-md border border-border text-sm">
            Copy code
          </button>
        )}
        {v.routerUserId && (
          <button
            onClick={onDelete}
            className="col-span-2 h-11 rounded-md border border-red-500/50 text-sm font-medium text-red-600 dark:text-red-400"
          >
            Delete
          </button>
        )}
      </div>
    </div>
  );
}

function SlipDialog({
  v,
  router,
  layout,
  onClose,
}: {
  v: VoucherView | null;
  router: string;
  layout: import("@/lib/voucher-print-layout").VoucherPrintLayout;
  onClose: () => void;
}) {
  const [qr, setQr] = useState<string | null>(null);
  useEffect(() => {
    if (!v) {
      setQr(null);
      return;
    }
    QRCode.toDataURL(v.code, { width: 320, margin: 1, errorCorrectionLevel: "M" })
      .then(setQr)
      .catch(() => setQr(null));
  }, [v]);
  if (!v) return null;

  async function share() {
    if (!v) return;
    const text = `Wi-Fi voucher\nCode: ${v.code}\nAccess: ${router}`;
    if (typeof navigator.share === "function") {
      try {
        await navigator.share({ title: "Wi-Fi voucher", text });
        return;
      } catch {
        /* cancelled */
      }
    }
    const ok = await copyText(text);
    if (ok) toast.success("Voucher copied");
    else toast.error("Copy failed");
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Voucher slip</DialogTitle>
          <DialogDescription>Print, share, or hand this to the customer.</DialogDescription>
        </DialogHeader>
        <div className="rounded-2xl border-2 border-dashed border-border bg-surface p-5 text-center">
          <div className="text-[10px] uppercase tracking-widest text-muted-foreground">
            {layout.business_name}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">Wi-Fi Voucher</div>
          {layout.show_qr && qr && (
            <img
              src={qr}
              alt={`QR code for voucher ${v.code}`}
              className="mx-auto my-3 h-40 w-40"
            />
          )}
          <div className="font-mono text-3xl font-bold tracking-widest">{v.code}</div>
          <div className="mt-2 text-[11px] text-muted-foreground">Plan: {v.profile}</div>
          {layout.show_price && v.priceMmk ? (
            <div className="text-[11px] text-muted-foreground">
              Price: {v.priceMmk.toLocaleString()} MMK
            </div>
          ) : null}
          {layout.show_expiry && v.expiresAt ? (
            <div className="text-[11px] text-muted-foreground">
              Expiry: {new Date(v.expiresAt).toLocaleString()}
            </div>
          ) : null}
          <div className="mt-2 text-[11px] text-muted-foreground">
            Wi-Fi: {layout.wifi_name || router} · Support: {layout.support_contact}
          </div>
          <div className="mt-1 text-[10px] text-muted-foreground">{layout.terms}</div>
          {v.redeemedAt && (
            <div className="text-[11px] text-muted-foreground">
              Handed out: {formatHandedOutTime(v.redeemedAt)}
            </div>
          )}
        </div>
        <DialogFooter>
          <button
            onClick={() =>
              void copyText(v.code).then((ok) => {
                if (ok) toast.success("Copied");
                else toast.error("Copy failed");
              })
            }
            className="h-10 rounded-md border border-border px-3 text-sm"
          >
            Copy
          </button>
          <button onClick={share} className="h-10 rounded-md border border-border px-3 text-sm">
            Share
          </button>
          <button
            onClick={() => void printVoucherThermalReceipt(v, layout)}
            className="h-10 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground"
          >
            Thermal print
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EmptyState({
  hasAny,
  onCreate,
  canCreate,
}: {
  hasAny: boolean;
  onCreate: () => void;
  canCreate: boolean;
}) {
  if (hasAny)
    return (
      <div className="p-8 text-center text-sm text-muted-foreground">
        No vouchers match your filter.
      </div>
    );
  return (
    <div className="space-y-3 p-8 text-center">
      <div
        className="mx-auto grid h-12 w-12 place-items-center rounded-full bg-primary/10 text-primary"
        aria-hidden="true"
      >
        <svg
          width="22"
          height="22"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M20 12a2 2 0 0 1 0-4V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v2a2 2 0 0 1 0 4v2a2 2 0 0 1 0 4v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2a2 2 0 0 1 0-4Z" />
          <path d="M9 8v8" />
        </svg>
      </div>
      <h2 className="text-sm font-semibold">No vouchers yet</h2>
      <p className="mx-auto max-w-xs text-xs text-muted-foreground">
        Generate a batch of voucher codes from a plan to hand out to customers.
      </p>
      <button
        onClick={onCreate}
        disabled={!canCreate}
        className="inline-block rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60"
      >
        Generate first batch
      </button>
    </div>
  );
}

function Th({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <th className={`px-3 py-2 text-left font-medium ${className}`}>{children}</th>;
}
function Td({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <td className={`px-3 py-2 align-middle ${className}`}>{children}</td>;
}

/* ---------- print sheet (opens a formatted window) ---------- */

function printSheet(
  vouchers: SheetVoucher[],
  router: string,
  layout: VoucherPrintLayout = DEFAULT_VOUCHER_PRINT_LAYOUT,
) {
  if (!vouchers.length) return;
  void printVoucherA4CutSheet(
    vouchers.map((voucher) => ({
      code: voucher.code,
      profile: voucher.planLabel,
      priceMmk: voucher.priceMmk,
      expiresAt: voucher.expiresAt,
    })),
    router,
    {
      showPrice: layout.show_price,
      showExpiry: layout.show_expiry,
      columns: layout.a4_columns,
      rowsPerPage: layout.a4_rows_per_page,
    },
  );
}

function printThermalStrip(
  vouchers: SheetVoucher[],
  layout: VoucherPrintLayout = DEFAULT_VOUCHER_PRINT_LAYOUT,
) {
  if (!vouchers.length) return;
  void printVoucherThermalCutStrip(
    vouchers.map((voucher) => ({
      code: voucher.code,
      profile: voucher.planLabel,
      priceMmk: voucher.priceMmk,
      expiresAt: voucher.expiresAt,
    })),
    layout,
  );
}

function escapeHtml(s: string) {
  return s.replace(
    /[&<>"']/g,
    (m) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[m]!,
  );
}
