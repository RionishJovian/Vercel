import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { getBusinessSnapshot } from "@/lib/business.functions";
import { getFleetHealth } from "@/lib/fleet.functions";
import { fleetLiveTotals } from "@/lib/fleet-health";
import { useT } from "@/lib/i18n";
import { RevenueSparkline } from "@/components/RevenueSparkline";

/**
 * Voucher revenue hero — MMK windows, hotspot sessions, and offline sites.
 */
export function BusinessSummary() {
  const tr = useT();
  const fetchSnapshot = useServerFn(getBusinessSnapshot);
  const fetchHealth = useServerFn(getFleetHealth);

  const snap = useQuery({
    queryKey: ["business-snapshot"],
    queryFn: () => fetchSnapshot(),
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
  const health = useQuery({
    queryKey: ["fleet-health"],
    queryFn: () => fetchHealth(),
    staleTime: 30_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });

  const fleetRouters = health.data?.routers ?? [];
  const { activeSessions } = fleetLiveTotals(fleetRouters);
  const online = new Set(fleetRouters.filter((router) => router.online).map((router) => router.id));
  const sites = snap.data?.sites ?? [];
  const offlineSites = sites.filter(
    (s) => s.routerIds.length > 0 && !s.routerIds.some((id) => online.has(id)),
  );

  const money = (v: number | undefined) =>
    v == null ? "—" : `${v.toLocaleString()} ${snap.data?.revenue.currency ?? "MMK"}`;

  const failed = snap.isError;

  return (
    <section
      aria-labelledby="business-summary-heading"
      className="glass-panel rounded-[1.75rem] p-5 sm:p-6"
    >
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 sm:flex sm:justify-between">
        <div className="min-w-0">
          <h2 id="business-summary-heading" className="text-title truncate text-lg">
            {tr.label("Voucher revenue today")}
          </h2>
          <p className="text-sub mt-1 text-xs">
            {tr.copy("MMK from hotspot voucher sales across your RouterBoard sites.")}
          </p>
        </div>
        <Link
          to="/app/revenue"
          className="inline-flex min-h-11 shrink-0 items-center rounded-full border border-[color:var(--glass-border)] bg-[color:var(--surface-tint)] px-4 text-xs font-medium transition hover:border-primary hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          Open revenue
        </Link>
      </div>

      {failed ? (
        <div
          role="alert"
          className="mt-4 rounded-2xl border border-danger/40 bg-danger/10 p-3 text-xs text-danger"
        >
          {tr.copy("We could not load your business figures. Retry in a moment.")}
        </div>
      ) : (
        <>
          <p
            className={`text-title mt-4 text-3xl tracking-tight sm:text-4xl ${
              snap.isLoading ? "animate-pulse opacity-60" : ""
            }`}
            aria-busy={snap.isLoading ? "true" : undefined}
          >
            {snap.isLoading ? "…" : money(snap.data?.revenue.today)}
          </p>
          <RevenueSparkline values={snap.data?.revenueDaily7 ?? []} loading={snap.isLoading} />
          <div className="mt-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Metric
              label={tr.label("Last 7 days")}
              value={money(snap.data?.revenue.last7)}
              loading={snap.isLoading}
            />
            <Metric
              label={tr.label("Last 30 days")}
              value={money(snap.data?.revenue.last30)}
              loading={snap.isLoading}
            />
            <Metric
              label={tr.label("Active sessions")}
              value={health.data ? String(activeSessions) : "—"}
              loading={health.isLoading}
            />
            <Metric
              label={tr.label("Offline sites")}
              value={
                snap.isLoading || health.isLoading
                  ? "—"
                  : sites.length === 0
                    ? tr.copy("No sites yet")
                    : `${offlineSites.length} / ${sites.length}`
              }
              loading={snap.isLoading || health.isLoading}
              tone={offlineSites.length > 0 ? "warn" : "normal"}
            />
          </div>
        </>
      )}
    </section>
  );
}

function Metric({
  label,
  value,
  loading,
  tone = "normal",
}: {
  label: string;
  value: string;
  loading?: boolean;
  tone?: "normal" | "warn";
}) {
  return (
    <div className="rounded-2xl border border-[color:var(--glass-border)] bg-[color:var(--surface-tint)] p-3">
      <div className="text-kicker text-[10px] uppercase tracking-wide">{label}</div>
      <div
        className={`mt-1 truncate text-base font-semibold ${
          tone === "warn" ? "text-warning" : "text-title"
        } ${loading ? "animate-pulse opacity-60" : ""}`}
        aria-busy={loading ? "true" : undefined}
      >
        {loading ? "…" : value}
      </div>
    </div>
  );
}
