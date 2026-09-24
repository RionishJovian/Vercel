import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  getFleetHealth,
  listFleetScans,
  getAiScanQuota,
  runAiScanNow,
  applyInsightFix,
} from "@/lib/fleet.functions";
import { listHealthHistory as listHealthHistoryFn } from "@/lib/health.functions";
import { getMe } from "@/lib/auth.functions";
import { isPrivilegedAccount } from "@/lib/app-role";
import {
  fleetInsights,
  fleetLiveTotals,
  fleetModeLabel,
  fleetRefreshLabel,
  fmtBytes,
  fmtUptime,
  memoryUsedPercent,
  readAlertKeys,
  relativeAge,
} from "@/lib/fleet-health";
import { DelayedFallback, SkeletonList } from "@/components/ui/skeleton";
import { ServiceCountsStrip } from "@/components/ServiceCountsStrip";
import { useT } from "@/lib/i18n";
import { useInAppNotice } from "@/components/InAppNotice.context";
import { NOTICE_ICON, noticeToneForSeverity } from "@/lib/notify/tone";
import { useSelectedSite } from "@/hooks/useSelectedSite";
import { z } from "zod";

export const Route = createFileRoute("/_authenticated/app/fleet")({
  validateSearch: z.object({
    router: z.string().uuid().optional(),
  }).parse,
  head: () => ({
    meta: [
      { title: "Fleet health · MikroTik Magic" },
      {
        name: "description",
        content:
          "Live health, traffic and events for every router and access point, with on-demand AI scans.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: FleetPage,
});

function sevColor(s: string) {
  return s === "critical"
    ? "text-red-300 border-red-500/40 bg-red-500/10"
    : s === "warning"
      ? "text-amber-300 border-amber-400/40 bg-amber-500/10"
      : "text-white border-white/25 bg-white/5";
}

function FleetPage() {
  const t = useT();
  const { pushNotice } = useInAppNotice();
  const navigate = useNavigate({ from: Route.fullPath });
  const search = Route.useSearch();
  const focusedId = search.router;
  const { site: selectedSite } = useSelectedSite();

  const fetchMe = useServerFn(getMe);
  const me = useQuery({ queryKey: ["me"], queryFn: () => fetchMe() });
  const roles = me.data?.roles ?? [];
  const privileged = isPrivilegedAccount(roles, me.data?.isPlatformAdmin);
  const fetchHealth = useServerFn(getFleetHealth);
  const fetchScans = useServerFn(listFleetScans);
  const health = useQuery({
    queryKey: ["fleet-health"],
    queryFn: () => fetchHealth(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });
  const aiScans = useQuery({
    queryKey: ["fleet-ai-scans"],
    queryFn: () => fetchScans({ data: { kind: "ai", limit: 5 } }),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });

  const READ_KEY = "mm.fleet.readAlerts";
  const DISMISS_KEY = "mm.fleet.dismissedAlerts";
  const [readKeys, setReadKeys] = useState<Set<string>>(new Set());
  const [dismissedKeys, setDismissedKeys] = useState<Set<string>>(new Set());
  const [hydrated, setHydrated] = useState(false);
  useEffect(() => {
    try {
      setReadKeys(new Set(readAlertKeys(localStorage.getItem(READ_KEY))));
      const gone = localStorage.getItem(DISMISS_KEY);
      if (gone) setDismissedKeys(new Set(JSON.parse(gone) as string[]));
    } catch {
      /* ignore */
    }
    setHydrated(true);
  }, []);
  const persist = (next: Set<string>) => {
    setReadKeys(next);
    try {
      localStorage.setItem(READ_KEY, JSON.stringify([...next].slice(-500)));
    } catch {
      /* ignore */
    }
  };
  const persistDismissed = (next: Set<string>) => {
    setDismissedKeys(next);
    try {
      localStorage.setItem(DISMISS_KEY, JSON.stringify([...next].slice(-500)));
    } catch {
      /* ignore */
    }
  };
  const markRead = (key: string) => persist(new Set([...readKeys, key]));
  const removeInsight = (key: string) => {
    markRead(key);
    persistDismissed(new Set([...dismissedKeys, key]));
  };

  const poppedRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    if (!hydrated) return;
    const latest = aiScans.data?.[0];
    if (!latest) return;
    const insights = fleetInsights(latest.payload);
    for (const i of insights) {
      const key = `${latest.id}:${i.id}`;
      if (poppedRef.current.has(key) || readKeys.has(key) || dismissedKeys.has(key)) continue;
      poppedRef.current.add(key);
      pushNotice({
        id: `fleet:${key}`,
        tone: noticeToneForSeverity(i.severity),
        title: i.title,
        body: i.subtitle,
      });
    }
  }, [aiScans.data, hydrated, readKeys, dismissedKeys, pushNotice]);

  const fetchHealthHistory = useServerFn(listHealthHistoryFn);
  const healthHistory = useQuery({
    queryKey: ["health-history", routers.map((r) => r.id)],
    queryFn: () => fetchHealthHistory({
      data: {
        subjectKind: "router",
        subjectId: routers.map((r) => r.id).join(","),
        limit: 24,
      }
    }),
    refetchInterval: 120_000,
    refetchIntervalInBackground: false,
    staleTime: 60_000,
  });
  const quota = useQuery({ queryKey: ["ai-scan-quota"], queryFn: () => fetchQuota() });
  const scanMut = useMutation({
    mutationFn: () => runScan(),
    onMutate: () => setScanErr(null),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["fleet-ai-scans"] });
      qc.invalidateQueries({ queryKey: ["ai-scan-quota"] });
      qc.invalidateQueries({ queryKey: ["incidents-open"] });
    },
    onError: (e: Error) => setScanErr(e.message),
  });
  const applyMut = useMutation({
    mutationFn: (input: { routerId: string; fixCommand: string; insightId: string }) =>
      applyFix({ data: input }),
    onMutate: () => setApplyMsg(null),
    onSuccess: (r) => {
      setApplyMsg(`Applied on ${r.routerName} (${r.ms}ms): ${r.script}`);
      qc.invalidateQueries({ queryKey: ["fleet-health"] });
      qc.invalidateQueries({ queryKey: ["fleet-ai-scans"] });
    },
    onError: (e: Error) => setApplyMsg(e.message),
  });

  const routers = useMemo(() => {
    const all = health.data?.routers ?? [];
    if (!selectedSite) return all;
    return all.filter((r) => r.site_id === selectedSite.id);
  }, [health.data?.routers, selectedSite]);
  const { online, activeSessions: totalSessions } = fleetLiveTotals(routers);
  const latestAi = aiScans.data?.[0];
  const insights = fleetInsights(latestAi?.payload).filter(
    (i) => !dismissedKeys.has(`${latestAi?.id}:${i.id}`),
  );
  const unreadCount = latestAi
    ? insights.filter((i) => !readKeys.has(`${latestAi.id}:${i.id}`)).length
    : 0;
  const criticalUnread = latestAi
    ? insights.filter((i) => i.severity === "critical" && !readKeys.has(`${latestAi.id}:${i.id}`))
        .length
    : 0;

  const anomalies = routers.flatMap((r) => {
    const out: string[] = [];
    if (!r.online) out.push(`${r.name} is unreachable`);
    if ((r.cpu_load ?? 0) >= 85) out.push(`${r.name} CPU at ${r.cpu_load}%`);
    const memPct = memoryUsedPercent(r.total_memory, r.free_memory);
    if (memPct != null && memPct >= 85) out.push(`${r.name} memory at ${memPct}%`);
    if ((r.temperature_c ?? 0) >= 70) out.push(`${r.name} temp ${r.temperature_c}°C`);
    return out;
  });

  useEffect(() => {
    if (!focusedId) return;
    const el = document.getElementById(`fleet-router-${focusedId}`);
    el?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [focusedId, routers.length]);

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold">{t.ui("Fleet")}</h1>
          <p className="text-sm text-muted-foreground">
            {t.copy("Every router · live health · manual AI scans.")}
          </p>
        </div>
        <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
          <button
            type="button"
            onClick={() => {
              setManualRefresh(true);
              void health.refetch().finally(() => setManualRefresh(false));
            }}
            disabled={manualRefresh}
            className="min-h-[36px] shrink-0 whitespace-nowrap rounded-md border border-border bg-surface px-3 py-1.5 font-medium text-foreground transition-[border-color,color,opacity,background-color] hover:border-primary hover:text-primary disabled:opacity-50"
          >
            {fleetRefreshLabel(manualRefresh)}
          </button>
          <span className="rounded-md border border-border bg-surface px-3 py-1.5">
            <b className="text-foreground">{online}</b>/{routers.length} online
          </span>
          <span className="rounded-md border border-border bg-surface px-3 py-1.5">
            <b className="text-foreground">{totalSessions}</b> live clients
          </span>
          {criticalUnread > 0 && (
            <span className="rounded-md border border-red-500/40 bg-red-500/10 px-3 py-1.5 text-red-200">
              <b>{criticalUnread}</b> critical unread
            </span>
          )}
          <span className="rounded-md border border-border bg-surface px-3 py-1.5">
            Last AI scan:{" "}
            {latestAi
              ? `${relativeAge(latestAi.generated_at)} · ${new Date(latestAi.generated_at).toLocaleTimeString()}`
              : "—"}
          </span>
          {!quota.data?.expired && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => scanMut.mutate()}
                disabled={
                  scanMut.isPending || (!quota.data?.unlimited && (quota.data?.remaining ?? 1) <= 0)
                }
                className="min-h-[36px] rounded-md border border-primary/50 bg-primary/15 px-3 py-1.5 font-medium text-primary transition hover:bg-primary/25 disabled:opacity-50"
              >
                {scanMut.isPending ? "Scanning…" : "Run AI scan"}
              </button>
              {!quota.data?.unlimited && quota.data && (
                <span
                  className="rounded-md border border-border bg-surface px-2 py-1.5"
                  title={
                    quota.data.carry
                      ? `${quota.data.monthlyGrant}/month · unused carries · resets 1 January ${quota.data.periodLabel}`
                      : `Resets on the 1st of each month — ${quota.data.periodLabel}`
                  }
                >
                  {quota.data.carry
                    ? `${quota.data.remaining}/${quota.data.limit} scans left · carries unused`
                    : `${quota.data.remaining}/${quota.data.limit} scans left this month`}
                </span>
              )}
            </div>
          )}
        </div>
      </header>

      {scanErr && (
        <div className="rounded-md border border-red-500/40 bg-red-950/50 p-3 text-sm text-red-200">
          {scanErr}
        </div>
      )}
      {applyMsg && (
        <div
          className={`rounded-md border p-3 text-sm ${
            applyMut.isError
              ? "border-red-500/40 bg-red-950/50 text-red-200"
              : "border-emerald-500/40 bg-emerald-950/40 text-emerald-100"
          }`}
        >
          {applyMsg}
        </div>
      )}

      {anomalies.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-100">
          <div className="min-w-0">
            <div className="font-semibold">
              {t.copy("Health anomaly detected — run an AI scan to diagnose.")}
            </div>
            <div className="mt-0.5 text-xs text-amber-200/80">
              {anomalies.slice(0, 3).join(" · ")}
              {anomalies.length > 3 ? ` · +${anomalies.length - 3}` : ""}
            </div>
          </div>
          {!quota.data?.expired && (
            <button
              type="button"
              onClick={() => scanMut.mutate()}
              disabled={
                scanMut.isPending || (!quota.data?.unlimited && (quota.data?.remaining ?? 1) <= 0)
              }
              className="min-h-[36px] rounded-md border border-amber-400/60 bg-amber-400/15 px-3 py-1.5 text-xs font-medium text-amber-100 transition hover:bg-amber-400/25 disabled:opacity-50"
            >
              {scanMut.isPending ? "Scanning…" : "Run AI scan"}
            </button>
          )}
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {health.isLoading && !health.data && (
          <div className="col-span-full">
            <DelayedFallback
              loading
              label="Loading fleet health"
              fallback={<SkeletonList rows={3} />}
            />
          </div>
        )}
        {health.isError && (
          <div className="panel col-span-full p-6 text-sm text-red-200">
            Could not load fleet health
            {health.error instanceof Error ? `: ${health.error.message}` : "."}
          </div>
        )}
        {health.data && routers.length === 0 && (
          <div className="panel col-span-full p-6 text-sm text-muted-foreground">
            {selectedSite
              ? `No routers on site “${selectedSite.name}”. Pick another site or assign a router on Sites.`
              : "No routers yet — add one from the Routers tab."}
          </div>
        )}
        {routers.map((r) => {
          const memPct = memoryUsedPercent(r.total_memory, r.free_memory);
          const focused = focusedId === r.id;
          const dot = r.online
            ? "bg-emerald-400 shadow-[0_0_10px_2px_rgba(52,211,153,0.7)]"
            : "bg-red-500 shadow-[0_0_10px_2px_rgba(239,68,68,0.7)]";
          return (
            <div
              key={r.id}
              id={`fleet-router-${r.id}`}
              className={`panel p-4 ${focused ? "ring-2 ring-primary/60" : ""}`}
            >
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <span className={`relative inline-flex h-2.5 w-2.5`}>
                      {r.online && (
                        <span
                          className={`absolute inset-0 rounded-full ${dot} animate-ping opacity-75`}
                        />
                      )}
                      <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${dot}`} />
                    </span>
                    <span className="font-semibold">{r.name}</span>
                    <span className="chip">{fleetModeLabel(r)}</span>
                  </div>
                  <div className="mt-0.5 text-xs text-muted-foreground">{r.host}</div>
                  <div className="mt-0.5 text-[11px] text-muted-foreground">
                    {r.board_name ?? "Board unknown"}
                    {r.identity ? ` · ${r.identity}` : ""}
                  </div>
                </div>
                <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                  {r.version ?? "—"}
                </span>
              </div>

              {r.online ? (
                <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
                  <Metric
                    label="CPU"
                    value={r.cpu_load != null ? `${r.cpu_load}%` : "—"}
                    bar={r.cpu_load}
                    warn={85}
                  />
                  <Metric
                    label="Memory"
                    value={memPct != null ? `${memPct}%` : "—"}
                    bar={memPct}
                    warn={85}
                  />
                  <Metric label="Clients" value={String(r.active_sessions ?? 0)} />
                  <Metric
                    label="Temp"
                    value={r.temperature_c != null ? `${r.temperature_c}°C` : "—"}
                    bar={r.temperature_c}
                    warn={70}
                  />
                  <Metric label="Blocked" value={String(r.blocked_bindings ?? 0)} />
                  <Metric label="Hosts" value={String(r.hosts ?? 0)} />
                  <Metric label="Uptime" value={fmtUptime(r.uptime)} />
                  <Metric label="Free mem" value={fmtBytes(r.free_memory)} />
                  <Metric label="Users" value={String(r.hotspot_users ?? 0)} />
                </div>
              ) : (
                <div className="mt-3 rounded-md border border-red-500/30 bg-red-500/5 p-2 text-xs text-red-200">
                  {r.error ?? "Unreachable"}
                </div>
              )}
              {r.online ? (
                <ServiceCountsStrip
                  counts={{
                    firewall_rules: r.firewall_rules ?? null,
                    wireguard: r.wireguard_peers ?? null,
                    queue_trees: r.queue_trees ?? null,
                    syslog_today: r.syslog_today ?? null,
                  }}
                />
              ) : null}
            </div>
          );
        })}
      </section>

      {/* Health History Section */}
      <section className="panel p-5">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <div className="text-xs uppercase tracking-wider text-muted-foreground">Health history</div>
            <div className="text-sm font-semibold">
              {healthHistory.data && healthHistory.data.length > 0
                ? `Last readings for ${healthHistory.data.length} routers`
                : healthHistory.isLoading
                ? "Loading health history..."
                : "No health history yet"}
            </div>
          </div>
        </div>

        {healthHistory.isError && (
          <div className="rounded-md border border-red-500/40 bg-red-950/50 p-3 text-sm text-red-200">
            Could not load health history
          </div>
        )}

        {healthHistory.isSuccess && healthHistory.data.length > 0 && (
          <div className="space-y-3">
            {healthHistory.data.map((sample: any) => (
              <div
                key={sample.id}
                className="rounded-lg border border-border bg-surface/40 p-4 text-sm"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <div className={`h-2.5 w-2.5 rounded-full ${sample.reachable ? "bg-emerald-400" : "bg-red-500"}`} />
                    <span className="font-semibold">
                      {sample.subject_kind === "router" ? "Router" : sample.subject_kind}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {sample.subject_id}
                    </span>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {relativeAge(sample.observed_at)}
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
                  {sample.uptime_seconds != null && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">Uptime</div>
                      <div className="font-semibold">
                        {sample.uptime_seconds >= 86400
                          ? `${Math.floor(sample.uptime_seconds / 86400)}d`
                          : sample.uptime_seconds >= 3600
                          ? `${Math.floor(sample.uptime_seconds / 3600)}h`
                          : `${Math.floor(sample.uptime_seconds / 60)}m`}
                      </div>
                    </div>
                  )}

                  {sample.latency_ms != null && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">Latency</div>
                      <div className="font-semibold">
                        {sample.latency_ms < 100
                          ? `${sample.latency_ms}ms`
                          : sample.latency_ms < 500
                          ? `~${Math.round(sample.latency_ms / 100) * 100}ms`
                          : sample.latency_ms < 2000
                          ? `~${Math.round(sample.latency_ms / 500) * 500}ms`
                          : `~${Math.round(sample.latency_ms / 1000)}s`}
                      </div>
                    </div>
                  )}

                  {sample.cpu_usage_pct != null && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">CPU</div>
                      <div className="font-semibold">
                        {sample.cpu_usage_pct.toFixed(1)}%
                      </div>
                    </div>
                  )}

                  {sample.memory_usage_pct != null && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">Memory</div>
                      <div className="font-semibold">
                        {sample.memory_usage_pct.toFixed(1)}%
                      </div>
                    </div>
                  )}

                  {sample.wan_state && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">WAN</div>
                      <div className="font-semibold">
                        {sample.wan_state === "up"
                          ? "Up"
                          : sample.wan_state === "degraded"
                          ? "Degraded"
                          : sample.wan_state === "down"
                          ? "Down"
                          : sample.wan_state}
                      </div>
                    </div>
                  )}

                  {sample.connector_state && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">Connector</div>
                      <div className="font-semibold">
                        {sample.connector_state === "connected"
                          ? "Connected"
                          : sample.connector_state === "stale"
                          ? "Stale"
                          : sample.connector_state === "disconnected"
                          ? "Disconnected"
                          : sample.connector_state}
                      </div>
                    </div>
                  )}

                  {sample.tunnel_state && (
                    <div>
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground">Tunnel</div>
                      <div className="font-semibold">
                        {sample.tunnel_state === "up"
                          ? "Up"
                          : sample.tunnel_state === "down"
                          ? "Down"
                          : sample.tunnel_state}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="panel p-5">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <div className="text-xs uppercase tracking-wider text-muted-foreground">AI scan</div>
            <div className="text-sm font-semibold">
              {latestAi
                ? `Latest run · ${relativeAge(latestAi.generated_at)} · ${new Date(latestAi.generated_at).toLocaleString()}`
                : t.copy("No runs yet — start a scan when you want a diagnosis.")}
            </div>
          </div>
          <div className="flex items-center gap-2">
            {latestAi && unreadCount > 0 && (
              <button
                type="button"
                onClick={() => {
                  persist(new Set([...readKeys, ...insights.map((i) => `${latestAi.id}:${i.id}`)]));
                }}
                className="rounded-md border border-border bg-surface px-2 py-1 text-[11px] text-muted-foreground transition hover:text-foreground"
              >
                Acknowledge all
              </button>
            )}
            {latestAi && (
              <span
                className={`rounded-md border px-2 py-0.5 text-[11px] uppercase ${sevColor(latestAi.max_severity)}`}
              >
                {latestAi.max_severity}
              </span>
            )}
          </div>
        </div>
        {insights.length === 0 ? (
          <div className="text-sm text-muted-foreground">
            {latestAi
              ? t.copy("No issues detected.")
              : t.copy("No runs yet — start a scan when you want a diagnosis.")}
          </div>
        ) : (
          <ul className="space-y-2">
            {insights.map((i) => {
              const key = `${latestAi?.id}:${i.id}`;
              const isRead = readKeys.has(key);
              const canApply = privileged && !!i.fix_command && !!i.router_id;
              return (
                <li
                  key={key}
                  className={`rounded-lg border p-3 text-sm transition ${
                    isRead
                      ? "border-border/40 bg-surface/20 text-muted-foreground opacity-55 shadow-none"
                      : sevColor(i.severity)
                  }`}
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2 font-semibold">
                      {!isRead && (
                        <span
                          aria-label="Unread"
                          className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-current"
                        />
                      )}
                      <span aria-hidden>{NOTICE_ICON[noticeToneForSeverity(i.severity)]}</span>
                      {i.title}
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-[10px] uppercase opacity-70">{i.router}</span>
                      {isRead && (
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            removeInsight(key);
                          }}
                          className="rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground hover:bg-red-500/15 hover:text-red-300"
                        >
                          Remove
                        </button>
                      )}
                    </div>
                  </div>
                  {i.subtitle && <div className="mt-1 text-xs opacity-80">{i.subtitle}</div>}
                  {i.suggestion && <div className="mt-1 text-xs">{i.suggestion}</div>}
                  {i.fix_command && (
                    <pre
                      className={`mt-2 overflow-x-auto rounded bg-black/40 p-2 text-[11px] ${
                        isRead ? "text-muted-foreground" : "text-foreground"
                      }`}
                    >
                      {i.fix_command}
                    </pre>
                  )}
                  <div className="mt-2 flex flex-wrap gap-2">
                    {!isRead && (
                      <button
                        type="button"
                        onClick={() => markRead(key)}
                        className="rounded-md border border-border bg-surface px-2 py-1 text-[11px] text-muted-foreground transition hover:text-foreground"
                      >
                        Acknowledge
                      </button>
                    )}
                    {i.router_id && (
                      <button
                        type="button"
                        onClick={() =>
                          navigate({
                            search: (prev) => ({ ...prev, router: i.router_id }),
                          })
                        }
                        className="rounded-md border border-border bg-surface px-2 py-1 text-[11px] text-muted-foreground transition hover:text-foreground"
                      >
                        Focus router
                      </button>
                    )}
                    {canApply && (
                      <button
                        type="button"
                        disabled={applyMut.isPending}
                        onClick={() => {
                          if (
                            !confirm(
                              `Apply this fix on the physical router?\n\n${i.fix_command}\n\nIt will run via RouterOS /execute and be audited.`,
                            )
                          )
                            return;
                          applyMut.mutate({
                            routerId: i.router_id!,
                            fixCommand: i.fix_command!,
                            insightId: i.id,
                          });
                          markRead(key);
                        }}
                        className="rounded-md border border-primary/50 bg-primary/15 px-2 py-1 text-[11px] font-medium text-primary transition hover:bg-primary/25 disabled:opacity-50"
                      >
                        {applyMut.isPending ? "Applying…" : "Apply fix"}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}

function Metric({
  label,
  value,
  bar,
  warn,
}: {
  label: string;
  value: string;
  bar?: number;
  warn?: number;
}) {
  const isWarn = bar !== undefined && warn !== undefined && bar >= warn;
  return (
    <div className="min-w-0 rounded-md border border-border bg-surface/40 p-2">
      <div className="flex min-w-0 items-baseline justify-between gap-1">
        <span className="shrink-0 text-[10px] uppercase tracking-wide text-muted-foreground">
          {label}
        </span>
        <span
          className={`min-w-0 truncate font-medium tabular-nums ${isWarn ? "text-amber-300" : "text-foreground"}`}
          title={value}
        >
          {value}
        </span>
      </div>
      {bar !== undefined && (
        <div className="mt-1 h-1 overflow-hidden rounded bg-black/30">
          <div
            className={`h-full ${isWarn ? "bg-amber-400" : "bg-emerald-400"}`}
            style={{ width: `${Math.min(100, Math.max(0, bar))}%` }}
          />
        </div>
      )}
    </div>
  );
}
