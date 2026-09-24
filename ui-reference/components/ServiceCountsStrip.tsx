import { Link } from "@tanstack/react-router";
import { useT } from "@/lib/i18n";
import type { ServiceCounts } from "@/lib/service-counts";

/**
 * Real RouterOS / ingest counts — not hardcoded slideshow cards.
 * Probe failures stay "—"; zeros are real zeros.
 */
export function ServiceCountsStrip({ counts }: { counts: ServiceCounts }) {
  const t = useT();
  const tiles: Array<{
    k: string;
    label: string;
    sub: string;
    to?: "/app/syslog";
  }> = [
    {
      k: "FW",
      label: t.label("Firewall"),
      sub: counts.firewall_rules == null ? "—" : t.copy("{n} rules", { n: counts.firewall_rules }),
    },
    {
      k: "VPN",
      label: t.label("WireGuard"),
      sub: !counts.wireguard
        ? "—"
        : counts.wireguard.total === 0
          ? t.copy("0 peers")
          : t.copy("{ok}/{total} handshake", {
              ok: counts.wireguard.handshake_ok,
              total: counts.wireguard.total,
            }),
    },
    {
      k: "QOS",
      label: t.label("Queues"),
      sub: counts.queue_trees == null ? "—" : t.copy("{n} trees", { n: counts.queue_trees }),
    },
    {
      k: "LOG",
      label: t.label("Syslog"),
      sub: counts.syslog_today == null ? "—" : t.copy("{n} today", { n: counts.syslog_today }),
      to: "/app/syslog",
    },
  ];

  return (
    <section aria-label={t.label("Service counts")} className="mt-3 grid grid-cols-2 gap-2">
      {tiles.map((tile) => {
        const body = (
          <>
            <div className="flex size-6 items-center justify-center rounded bg-black/40 font-mono text-[10px] text-primary">
              {tile.k}
            </div>
            <div className="text-[11px] font-bold uppercase tracking-wide">{tile.label}</div>
            <div className="font-mono text-[9px] text-muted-foreground">{tile.sub}</div>
          </>
        );
        const cls = "flex flex-col gap-1 rounded-lg border border-border bg-surface p-3 text-left";
        if (tile.to) {
          return (
            <Link key={tile.k} to={tile.to} className={`${cls} hover:border-primary/40`}>
              {body}
            </Link>
          );
        }
        return (
          <div key={tile.k} className={cls}>
            {body}
          </div>
        );
      })}
    </section>
  );
}
