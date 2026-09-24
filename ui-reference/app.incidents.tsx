import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import {
  listIncidents,
  acknowledgeIncident,
  dismissIncident,
  listAlertRules,
  saveAlertRule,
  runHealthSweep,
  getNotificationPrefs,
  saveNotificationPrefs,
} from "@/lib/health.functions";
import { INCIDENT_LABEL, FRESHNESS_LABEL, freshnessOf, INCIDENT_KINDS } from "@/lib/alerts/rules";
import { DelayedFallback, SkeletonList } from "@/components/ui/skeleton";
import { ButtonSpinner } from "@/components/ui/button";
import { getMe } from "@/lib/auth.functions";
import { meHasFeature } from "@/lib/operator-features";
import { toErrorMessage } from "@/lib/error-message";

export const Route = createFileRoute("/_authenticated/app/incidents")({
  head: () => ({
    meta: [
      { title: "Incidents & alerts — MikroTik Magic" },
      {
        name: "description",
        content:
          "Site offline, degraded internet uplink, stale connector and access point outages, with quiet periods so alerts never spam you.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: IncidentsPage,
});

type Incident = {
  id: string;
  kind: keyof typeof INCIDENT_LABEL;
  severity: "info" | "warning" | "critical";
  subject_label: string;
  detail: string | null;
  opened_at: string;
  last_seen_at: string;
  resolved_at: string | null;
  acknowledged_at: string | null;
};

function SeverityPill({ severity }: { severity: Incident["severity"] }) {
  const tone =
    severity === "critical"
      ? "border-red-500/40 bg-red-500/10 text-red-300"
      : severity === "warning"
        ? "border-amber-400/40 bg-amber-500/10 text-amber-300"
        : "border-white/25 bg-white/5 text-white";
  const icon = severity === "critical" ? "❗️" : severity === "warning" ? "⚠️" : "✉️";
  return (
    <span className={`rounded-full border px-2.5 py-0.5 text-[11px] capitalize ${tone}`}>
      <span className="mr-1" aria-hidden>
        {icon}
      </span>
      {severity}
    </span>
  );
}

function IncidentsPage() {
  const t = useT();
  const qc = useQueryClient();
  const fetchMe = useServerFn(getMe);
  const me = useQuery({ queryKey: ["me"], queryFn: () => fetchMe(), staleTime: 5 * 60_000 });
  const canAlerts = meHasFeature(me.data, "alerts");
  const fetchIncidents = useServerFn(listIncidents);
  const fetchRules = useServerFn(listAlertRules);
  const fetchPrefs = useServerFn(getNotificationPrefs);
  const ack = useServerFn(acknowledgeIncident);
  const dismiss = useServerFn(dismissIncident);
  const sweep = useServerFn(runHealthSweep);
  const saveRule = useServerFn(saveAlertRule);
  const savePrefs = useServerFn(saveNotificationPrefs);

  const incidents = useQuery({
    queryKey: ["incidents"],
    queryFn: () => fetchIncidents({ data: {} }),
  });
  const rules = useQuery({ queryKey: ["alert-rules"], queryFn: () => fetchRules() });
  const prefs = useQuery({ queryKey: ["notification-prefs"], queryFn: () => fetchPrefs() });

  const running = useMutation({
    mutationFn: () => sweep(),
    onSuccess: (r) => {
      toast.success(
        `Checked ${r.sampled} device${r.sampled === 1 ? "" : "s"} — ${r.opened} new, ${r.resolved} resolved`,
      );
      qc.invalidateQueries({ queryKey: ["incidents"] });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const acking = useMutation({
    mutationFn: (id: string) => ack({ data: { id } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["incidents"] }),
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const dismissing = useMutation({
    mutationFn: (id: string) => dismiss({ data: { id } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["incidents"] }),
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const rulePatch = useMutation({
    mutationFn: (r: {
      kind: Incident["kind"];
      enabled: boolean;
      severity: Incident["severity"];
      cooldownMinutes: number;
      threshold: number;
    }) => saveRule({ data: r }),
    onSuccess: () => {
      toast.success("Alert rule saved");
      qc.invalidateQueries({ queryKey: ["alert-rules"] });
      qc.invalidateQueries({ queryKey: ["business-snapshot"] });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const prefPatch = useMutation({
    mutationFn: (p: {
      inApp: boolean;
      email: boolean;
      quietHoursStart: number | null;
      quietHoursEnd: number | null;
      minSeverity: Incident["severity"];
    }) => savePrefs({ data: p }),
    onSuccess: () => {
      toast.success("Notification preferences saved");
      qc.invalidateQueries({ queryKey: ["notification-prefs"] });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const rows = (incidents.data ?? []) as unknown as Incident[];
  const p = prefs.data;

  return (
    <div className="grid gap-6">
      <header className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold">{t.label("Incidents")}</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            {t.copy(
              "Problems worth your attention, raised only after repeated failed checks and kept quiet for a while after the first alert.",
            )}
          </p>
        </div>
        <button
          type="button"
          onClick={() => running.mutate()}
          disabled={running.isPending}
          aria-busy={running.isPending || undefined}
          className="inline-flex min-h-11 items-center justify-center gap-2 rounded-full bg-primary px-5 text-sm font-semibold text-primary-foreground transition-all duration-200 disabled:opacity-50"
        >
          {running.isPending && <ButtonSpinner />}
          {running.isPending ? "Checking…" : "Check now"}
        </button>
      </header>

      <section className="grid gap-3">
        {incidents.isLoading && (
          <DelayedFallback loading label="Loading incidents" fallback={<SkeletonList rows={4} />} />
        )}
        {incidents.isError && (
          <p className="text-sm text-red-300">
            {t.copy("We could not load incidents. Try again in a moment.")}
          </p>
        )}
        {!incidents.isLoading && rows.length === 0 && (
          <p className="panel p-5 text-sm text-muted-foreground">
            {t.copy("Nothing is wrong right now. Run a check to refresh this view.")}
          </p>
        )}
        {rows.map((i) => {
          const fresh = freshnessOf(i.last_seen_at);
          return (
            <article key={i.id} className="panel grid gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_auto]">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <SeverityPill severity={i.severity} />
                  <h2 className="break-words text-sm font-semibold">
                    {t.label(INCIDENT_LABEL[i.kind])} — {i.subject_label}
                  </h2>
                </div>
                {i.detail && (
                  <p className="mt-1 break-words text-xs text-muted-foreground">{i.detail}</p>
                )}
                <p className="mt-1 text-[11px] text-muted-foreground">
                  {t.copy("Started")} {new Date(i.opened_at).toLocaleString()} ·{" "}
                  {t.copy(FRESHNESS_LABEL[fresh])}
                </p>
              </div>
              <div className="flex items-start gap-2">
                <button
                  type="button"
                  disabled={!!i.acknowledged_at || acking.isPending}
                  onClick={() => acking.mutate(i.id)}
                  className="inline-flex min-h-11 items-center rounded-full border border-[color:var(--glass-border)] px-4 text-xs disabled:opacity-40"
                >
                  {i.acknowledged_at ? "Acknowledged" : "Acknowledge"}
                </button>
                {i.acknowledged_at && (
                  <button
                    type="button"
                    disabled={dismissing.isPending}
                    onClick={() => dismissing.mutate(i.id)}
                    className="inline-flex min-h-11 items-center rounded-full border border-red-500/40 px-4 text-xs text-red-300 hover:bg-red-500/10 disabled:opacity-40"
                  >
                    Remove
                  </button>
                )}
              </div>
            </article>
          );
        })}
      </section>

      <section className="panel p-5">
        <h2 className="text-lg font-semibold">{t.label("Alert rules")}</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {t.copy(
            "Turn an alert off, or change how many failed checks it takes and how long we stay quiet afterwards.",
          )}
        </p>
        <div className="mt-4 grid gap-3">
          {(rules.data ?? []).flatMap((r) => {
            const kind = INCIDENT_KINDS.find((value) => value === r.kind);
            const level =
              r.severity === "info" || r.severity === "warning" || r.severity === "critical"
                ? r.severity
                : null;
            if (!kind || !level) return [];
            return [
              <div
                key={r.kind}
                className="grid gap-2 rounded-xl border border-[color:var(--glass-border)] p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto]"
              >
                <div className="min-w-0 self-center">
                  <div className="break-words text-sm font-medium">
                    {t.label(INCIDENT_LABEL[kind])}
                  </div>
                </div>
                <label className="flex items-center gap-2 text-xs">
                  <input
                    type="checkbox"
                    checked={r.enabled}
                    disabled={!canAlerts}
                    onChange={(e) =>
                      rulePatch.mutate({
                        kind,
                        enabled: e.target.checked,
                        severity: level,
                        cooldownMinutes: r.cooldown_minutes,
                        threshold: r.threshold,
                      })
                    }
                  />
                  {t.label("On")}
                </label>
                <label className="flex items-center gap-2 text-xs">
                  {t.label("Failed checks")}
                  <input
                    type="number"
                    min={1}
                    max={20}
                    defaultValue={r.threshold}
                    disabled={!canAlerts}
                    className="input w-20"
                    onBlur={(e) =>
                      rulePatch.mutate({
                        kind,
                        enabled: r.enabled,
                        severity: level,
                        cooldownMinutes: r.cooldown_minutes,
                        threshold: Number(e.target.value),
                      })
                    }
                  />
                </label>
                <label className="flex items-center gap-2 text-xs">
                  {t.label("Quiet minutes")}
                  <input
                    type="number"
                    min={1}
                    max={1440}
                    defaultValue={r.cooldown_minutes}
                    disabled={!canAlerts}
                    className="input w-24"
                    onBlur={(e) =>
                      rulePatch.mutate({
                        kind,
                        enabled: r.enabled,
                        severity: level,
                        cooldownMinutes: Number(e.target.value),
                        threshold: r.threshold,
                      })
                    }
                  />
                </label>
              </div>,
            ];
          })}
        </div>
      </section>

      {p && (
        <section className="panel p-5">
          <h2 className="text-lg font-semibold">{t.label("Notifications")}</h2>
          <div className="mt-4 flex flex-wrap items-center gap-4 text-xs">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={p.in_app}
                onChange={(e) =>
                  prefPatch.mutate({
                    inApp: e.target.checked,
                    email: p.email,
                    quietHoursStart: p.quiet_hours_start,
                    quietHoursEnd: p.quiet_hours_end,
                    minSeverity: p.min_severity as Incident["severity"],
                  })
                }
              />
              {t.label("Show in the app")}
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={p.email}
                onChange={(e) =>
                  prefPatch.mutate({
                    inApp: p.in_app,
                    email: e.target.checked,
                    quietHoursStart: p.quiet_hours_start,
                    quietHoursEnd: p.quiet_hours_end,
                    minSeverity: p.min_severity as Incident["severity"],
                  })
                }
              />
              {t.label("Email me")}
            </label>
            <label className="flex items-center gap-2">
              {t.label("Minimum severity")}
              <select
                className="input w-32"
                value={p.min_severity}
                onChange={(e) =>
                  prefPatch.mutate({
                    inApp: p.in_app,
                    email: p.email,
                    quietHoursStart: p.quiet_hours_start,
                    quietHoursEnd: p.quiet_hours_end,
                    minSeverity: e.target.value as Incident["severity"],
                  })
                }
              >
                <option value="info">Info</option>
                <option value="warning">Warning</option>
                <option value="critical">Critical</option>
              </select>
            </label>
          </div>
        </section>
      )}
    </div>
  );
}
