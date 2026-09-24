import { createFileRoute, redirect } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { useEffect, useMemo, useState } from "react";
import { Sparkles, X } from "lucide-react";
import { toast } from "sonner";
import { SiteTopologyWorkspace } from "@/components/SiteTopologyWorkspace";
import { TopologyPoolPanel } from "@/components/TopologyPoolPanel";
import { toErrorMessage } from "@/lib/error-message";
import type {
  TopologyDownstreamDevice,
  TopologyNode,
  TopologyPortLabel,
} from "@/lib/topology/types";
import {
  assertTopologyPageAccess,
  getSiteTopology,
  investigateTopologyWithMagicDude,
  listTopologySites,
  saveSiteTopologyLabels,
} from "@/lib/topology.functions";

export const Route = createFileRoute("/_authenticated/app/topology")({
  head: () => ({
    meta: [
      { title: "Site topology — MikroTik Magic" },
      {
        name: "description",
        content:
          "Platform-admin network diagram: WAN → router → LAN ports → managed CRS/AP tree → learned devices.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  beforeLoad: async () => {
    try {
      await assertTopologyPageAccess();
    } catch {
      throw redirect({ to: "/app" });
    }
  },
  component: TopologyPage,
});

function TopologyPage() {
  const qc = useQueryClient();
  const fetchSites = useServerFn(listTopologySites);
  const fetchTopology = useServerFn(getSiteTopology);
  const investigateWithMagicDude = useServerFn(investigateTopologyWithMagicDude);
  const saveLabels = useServerFn(saveSiteTopologyLabels);

  const sites = useQuery({
    queryKey: ["topology-sites"],
    queryFn: () => fetchSites(),
  });

  const [siteId, setSiteId] = useState("");
  const [routerId, setRouterId] = useState("");
  const [draftLabels, setDraftLabels] = useState<TopologyPortLabel[]>([]);
  const [draftDownstream, setDraftDownstream] = useState<TopologyDownstreamDevice[]>([]);
  const [selectedNode, setSelectedNode] = useState<TopologyNode | null>(null);

  useEffect(() => {
    if (!siteId && sites.data?.[0]) setSiteId(sites.data[0].siteId);
  }, [sites.data, siteId]);

  const selectedSite = sites.data?.find((s) => s.siteId === siteId);

  useEffect(() => {
    if (!selectedSite) return;
    const pick = selectedSite.routers.find((r) => r.id === routerId) ?? selectedSite.routers[0];
    if (pick && pick.id !== routerId) setRouterId(pick.id);
  }, [selectedSite, routerId]);

  const topology = useQuery({
    queryKey: ["site-topology", siteId, routerId],
    queryFn: () => fetchTopology({ data: { siteId, routerId: routerId || undefined } }),
    enabled: Boolean(siteId && routerId),
    refetchInterval: 45_000,
  });

  useEffect(() => {
    if (topology.data) {
      setDraftLabels(topology.data.portLabels);
      setDraftDownstream(topology.data.downstreamDevices);
      setSelectedNode((current) =>
        current && topology.data.nodes.some((node) => node.id === current.id)
          ? (topology.data.nodes.find((node) => node.id === current.id) ?? null)
          : null,
      );
    }
  }, [topology.data]);

  const saveMut = useMutation({
    mutationFn: () =>
      saveLabels({
        data: {
          siteId,
          routerId,
          portLabels: draftLabels.filter((p) => p.port && p.label.trim()),
          downstreamDevices: draftDownstream.filter((d) => d.parentPort && d.label.trim()),
        },
      }),
    onSuccess: async () => {
      toast.success("Topology labels saved");
      await qc.invalidateQueries({ queryKey: ["site-topology", siteId, routerId] });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const magicDude = useMutation({
    mutationFn: (node: TopologyNode) =>
      investigateWithMagicDude({ data: { siteId, routerId, nodeId: node.id } }),
  });

  const portOptions = useMemo(() => {
    const fromProbe =
      topology.data?.nodes.filter((n) => n.kind === "port").map((n) => n.label) ?? [];
    const fromDraft = draftLabels.map((p) => p.port);
    return [...new Set([...fromProbe, ...fromDraft])].sort((a, b) =>
      a.localeCompare(b, undefined, { numeric: true }),
    );
  }, [topology.data, draftLabels]);

  function updateLabel(port: string, patch: Partial<TopologyPortLabel>) {
    setDraftLabels((prev) => {
      const idx = prev.findIndex((p) => p.port === port);
      if (idx >= 0) {
        const next = [...prev];
        next[idx] = { ...next[idx]!, ...patch };
        return next;
      }
      return [...prev, { port, label: "", kind: "other", ...patch }];
    });
  }

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <p className="text-[11px] font-semibold uppercase tracking-wide text-amber-300/90">
          Internal · platform administrators
        </p>
        <h1 className="text-title text-2xl">Site topology</h1>
        <p className="text-sm text-muted-foreground">
          Live WAN → router → LAN ports → learned devices. Phase B adds guest IP pool health —
          hotspot and DHCP should share <span className="font-mono">hotspot-pool</span>.
        </p>
      </header>

      <div className="grid gap-3 rounded-xl border border-border/60 bg-white/5 p-4 md:grid-cols-3">
        <label className="block text-xs">
          <span className="mb-1 block text-muted-foreground">Site</span>
          <select
            className="w-full rounded-md border border-border bg-surface px-2 py-1.5 text-sm"
            value={siteId}
            onChange={(e) => {
              setSiteId(e.target.value);
              setRouterId("");
            }}
          >
            {(sites.data ?? []).map((s) => (
              <option key={s.siteId} value={s.siteId}>
                {s.siteName}
                {s.location ? ` · ${s.location}` : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-xs">
          <span className="mb-1 block text-muted-foreground">Gateway router</span>
          <select
            className="w-full rounded-md border border-border bg-surface px-2 py-1.5 text-sm"
            value={routerId}
            onChange={(e) => setRouterId(e.target.value)}
            disabled={!selectedSite?.routers.length}
          >
            {(selectedSite?.routers ?? []).map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </label>
        <div className="flex items-end">
          <button
            type="button"
            className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-muted/40"
            onClick={() => void topology.refetch()}
            disabled={topology.isFetching}
          >
            {topology.isFetching ? "Refreshing…" : "Refresh now"}
          </button>
        </div>
      </div>

      {topology.isLoading && (
        <p className="text-sm text-muted-foreground">Probing router and building diagram…</p>
      )}
      {topology.error && <p className="text-sm text-red-300">{toErrorMessage(topology.error)}</p>}
      {topology.data && (
        <>
          {topology.data.probeError && (
            <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-100">
              {topology.data.probeError} — diagram shows last known layout; fix Connect → Test on
              the router first.
            </p>
          )}
          <SiteTopologyWorkspace
            snapshot={topology.data}
            selectedNodeId={selectedNode?.id}
            onSelectNode={(node) => {
              magicDude.reset();
              setSelectedNode(node);
            }}
          />
          {!Object.values(topology.data.discoveryAccess).every(Boolean) && (
            <p className="rounded-md border border-amber-400/30 bg-amber-400/10 px-3 py-2 text-xs text-amber-100">
              Downstream discovery access is incomplete:{" "}
              {[
                !topology.data.discoveryAccess.bridgeHostTable && "/interface/bridge/host",
                !topology.data.discoveryAccess.dhcpLeases && "/ip/dhcp-server/lease",
                !topology.data.discoveryAccess.arp && "/ip/arp",
                !topology.data.discoveryAccess.neighbors && "/ip/neighbor",
              ]
                .filter(Boolean)
                .join(", ")}
              . The router account needs RouterOS read and rest-api policies for these read-only
              checks.
            </p>
          )}
          {topology.data.discoveredDevices.length > 0 && (
            <p className="rounded-md border border-sky-400/25 bg-sky-400/5 px-3 py-2 text-xs text-sky-100">
              {topology.data.discoveredDevices.length} device
              {topology.data.discoveredDevices.length === 1 ? "" : "s"} learned from the router
              bridge host table and shown under their LAN ports. Physical switch-port mapping is not
              confirmed.
            </p>
          )}
          {selectedNode && (
            <section
              aria-live="polite"
              className="rounded-xl border border-violet-400/30 bg-[radial-gradient(circle_at_top_left,rgba(124,58,237,0.16),transparent_42%)] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex gap-3">
                  <span className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-violet-300/35 bg-violet-400/10 text-violet-200">
                    <Sparkles size={16} aria-hidden="true" />
                  </span>
                  <div>
                    <p className="text-sm font-semibold">Magic Dude insight</p>
                    <h2 className="mt-1 text-base font-semibold text-foreground">
                      {selectedNode.status === "down"
                        ? `${selectedNode.label} is down`
                        : selectedNode.status === "unknown"
                          ? `${selectedNode.label} needs verification`
                          : `${selectedNode.label} is responding normally`}
                    </h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {magicDude.data
                        ? magicDude.data.insight.summary
                        : selectedNode.status === "down"
                          ? "No link is currently reported. Inspect the cable, peer device, and administrative port state before making a configuration change."
                          : selectedNode.status === "unknown"
                            ? "The latest probe could not confirm this link. Refresh the topology or inspect the router connection before acting."
                            : "The latest topology probe reports this link as up. Use the saved label and connected path to confirm it is the expected device."}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  aria-label="Close Magic Dude insight"
                  className="rounded-md p-1 text-muted-foreground hover:bg-white/10 hover:text-foreground"
                  onClick={() => {
                    magicDude.reset();
                    setSelectedNode(null);
                  }}
                >
                  <X size={16} aria-hidden="true" />
                </button>
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                <button
                  type="button"
                  className="rounded-md border border-violet-300/50 bg-violet-400/10 px-3 py-1.5 text-xs font-medium text-violet-100 hover:bg-violet-400/20"
                  onClick={() => magicDude.mutate(selectedNode)}
                  disabled={magicDude.isPending}
                >
                  {magicDude.isPending ? "Investigating…" : "Investigate with Magic Dude"}
                </button>
                <p className="self-center text-xs text-muted-foreground">
                  Uses one AI scan and analyzes only this selected topology node. No router changes
                  are made.
                </p>
              </div>
              {magicDude.error && (
                <p className="mt-3 text-xs text-red-200">{toErrorMessage(magicDude.error)}</p>
              )}
              {magicDude.data && (
                <div className="mt-4 grid gap-3 rounded-lg border border-white/10 bg-black/20 p-3 text-sm">
                  <div>
                    <p className="font-medium">{magicDude.data.insight.title}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Confidence: {magicDude.data.insight.confidence}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      Evidence
                    </p>
                    <ul className="mt-1 list-disc space-y-1 pl-4 text-xs text-muted-foreground">
                      {magicDude.data.insight.evidence.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    <span className="font-medium text-foreground">Impact: </span>
                    {magicDude.data.insight.impact}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    <span className="font-medium text-foreground">Recommended next step: </span>
                    {magicDude.data.insight.recommendation}
                  </p>
                </div>
              )}
            </section>
          )}
          <TopologyPoolPanel snapshot={topology.data} />
        </>
      )}

      <section className="space-y-3 rounded-xl border border-border/60 bg-white/5 p-4">
        <h2 className="text-sm font-semibold">Port labels (saved per site)</h2>
        <p className="text-xs text-muted-foreground">
          Example: eth3 → CRS326 (switch), eth4 → TP-Link AP. Learned hosts and /ip/neighbor rows
          appear under LAN ports automatically. Use Managed downstream below to declare the CRS
          faceplate (port → AP) until SwOS/SNMP can poll real switch-port state.
        </p>
        <div className="space-y-2">
          {portOptions.map((port) => {
            const row = draftLabels.find((p) => p.port === port) ?? {
              port,
              label: "",
              kind: "other" as const,
            };
            return (
              <div key={port} className="grid gap-2 md:grid-cols-[88px_1fr_120px]">
                <span className="self-center font-mono text-xs text-foreground">{port}</span>
                <input
                  className="rounded-md border border-border bg-surface px-2 py-1.5 text-sm"
                  placeholder="Device name (CRS326, TP-Link AP…)"
                  value={row.label}
                  onChange={(e) => updateLabel(port, { label: e.target.value })}
                />
                <select
                  className="rounded-md border border-border bg-surface px-2 py-1.5 text-sm"
                  value={row.kind}
                  onChange={(e) =>
                    updateLabel(port, { kind: e.target.value as TopologyPortLabel["kind"] })
                  }
                >
                  <option value="switch">Switch</option>
                  <option value="ap">Access point</option>
                  <option value="other">Other</option>
                </select>
              </div>
            );
          })}
        </div>
        <button
          type="button"
          disabled={!siteId || !routerId || saveMut.isPending}
          onClick={() => saveMut.mutate()}
          className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
        >
          {saveMut.isPending ? "Saving…" : "Save topology"}
        </button>
      </section>

      <section className="space-y-3 rounded-xl border border-border/60 bg-white/5 p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold">Managed downstream (Phase C)</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Build the declared device tree below each gateway port. Save a management IP and MAC
              for switches, APs, and child devices so they remain identifiable even when discovery
              is unavailable. Declared child-port status stays unknown until a real poll path
              exists.
            </p>
          </div>
          <button
            type="button"
            className="rounded-md border border-border px-3 py-1.5 text-xs hover:bg-muted/40"
            onClick={() =>
              setDraftDownstream((prev) => [
                ...prev,
                {
                  id: crypto.randomUUID(),
                  parentPort: portOptions[0] ?? "ether3",
                  label: "CRS326",
                  role: "switch",
                  ipAddress: null,
                  macAddress: null,
                  vendorHint: "MikroTik",
                  switchPorts: [
                    {
                      id: crypto.randomUUID(),
                      name: "ether3",
                      label: "Lobby AP",
                      role: "ap",
                      ipAddress: null,
                      macAddress: null,
                    },
                  ],
                },
              ])
            }
          >
            Add managed device
          </button>
        </div>

        {draftDownstream.length === 0 && (
          <p className="text-xs text-muted-foreground">
            No managed downstream rows yet. Add a switch on the uplink ether, then list its child
            ports and APs.
          </p>
        )}

        <div className="space-y-3">
          {draftDownstream.map((device, index) => (
            <div
              key={device.id}
              className="space-y-2 rounded-lg border border-border/50 bg-black/20 p-3"
            >
              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[130px_minmax(12rem,1fr)_150px_auto]">
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  Gateway port
                  <select
                    className="rounded-md border border-border bg-surface px-2 py-2 font-mono text-xs normal-case tracking-normal text-foreground"
                    value={device.parentPort}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index ? { ...row, parentPort: e.target.value } : row,
                        ),
                      )
                    }
                  >
                    {portOptions.map((port) => (
                      <option key={port} value={port}>
                        {port}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  Device name
                  <input
                    className="rounded-md border border-border bg-surface px-2 py-2 text-sm normal-case tracking-normal text-foreground"
                    placeholder="CRS326, Lobby AP…"
                    value={device.label}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index ? { ...row, label: e.target.value } : row,
                        ),
                      )
                    }
                  />
                </label>
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  Device type
                  <select
                    className="rounded-md border border-border bg-surface px-2 py-2 text-sm normal-case tracking-normal text-foreground"
                    value={device.role}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index
                            ? {
                                ...row,
                                role: e.target.value as TopologyDownstreamDevice["role"],
                                switchPorts: e.target.value === "switch" ? row.switchPorts : [],
                              }
                            : row,
                        ),
                      )
                    }
                  >
                    <option value="switch">Switch</option>
                    <option value="ap">Access point</option>
                    <option value="other">Other</option>
                  </select>
                </label>
                <button
                  type="button"
                  className="self-end rounded-md border border-border px-3 py-2 text-xs text-red-200 hover:bg-red-500/10"
                  onClick={() => setDraftDownstream((prev) => prev.filter((_, i) => i !== index))}
                >
                  Remove device
                </button>
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  Vendor / model
                  <input
                    className="rounded-md border border-border bg-surface px-2 py-2 text-sm normal-case tracking-normal text-foreground"
                    placeholder="MikroTik CRS326"
                    value={device.vendorHint ?? ""}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index ? { ...row, vendorHint: e.target.value || null } : row,
                        ),
                      )
                    }
                  />
                </label>
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  Management IP
                  <input
                    inputMode="decimal"
                    className="rounded-md border border-border bg-surface px-2 py-2 font-mono text-xs normal-case tracking-normal text-foreground"
                    placeholder="192.168.88.2"
                    value={device.ipAddress ?? ""}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index ? { ...row, ipAddress: e.target.value || null } : row,
                        ),
                      )
                    }
                  />
                </label>
                <label className="grid gap-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                  MAC address
                  <input
                    autoCapitalize="characters"
                    spellCheck={false}
                    className="rounded-md border border-border bg-surface px-2 py-2 font-mono text-xs normal-case tracking-normal text-foreground"
                    placeholder="AA:BB:CC:DD:EE:FF"
                    value={device.macAddress ?? ""}
                    onChange={(e) =>
                      setDraftDownstream((prev) =>
                        prev.map((row, i) =>
                          i === index ? { ...row, macAddress: e.target.value || null } : row,
                        ),
                      )
                    }
                  />
                </label>
              </div>

              {device.role === "switch" && (
                <div className="space-y-2 rounded-md border border-emerald-400/20 bg-emerald-400/5 p-2">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-[11px] font-medium uppercase tracking-wide text-emerald-100/90">
                      CRS / switch ports (declared)
                    </p>
                    <button
                      type="button"
                      className="rounded border border-border px-2 py-0.5 text-[11px] hover:bg-muted/40"
                      onClick={() =>
                        setDraftDownstream((prev) =>
                          prev.map((row, i) =>
                            i === index
                              ? {
                                  ...row,
                                  switchPorts: [
                                    ...row.switchPorts,
                                    {
                                      id: crypto.randomUUID(),
                                      name: `ether${row.switchPorts.length + 1}`,
                                      label: "",
                                      role: "ap",
                                      ipAddress: null,
                                      macAddress: null,
                                    },
                                  ],
                                }
                              : row,
                          ),
                        )
                      }
                    >
                      Add port
                    </button>
                  </div>
                  {device.switchPorts.map((child, childIndex) => (
                    <div
                      key={child.id}
                      className="space-y-2 rounded-md border border-white/10 bg-black/15 p-2"
                    >
                      <div className="grid gap-2 md:grid-cols-[110px_1fr_120px_auto]">
                        <input
                          aria-label="Switch port name"
                          className="rounded-md border border-border bg-surface px-2 py-1.5 font-mono text-xs"
                          placeholder="ether5"
                          value={child.name}
                          onChange={(e) =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.map((c, ci) =>
                                        ci === childIndex ? { ...c, name: e.target.value } : c,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        />
                        <input
                          aria-label="Connected device name"
                          className="rounded-md border border-border bg-surface px-2 py-1.5 text-sm"
                          placeholder="Connected device name"
                          value={child.label}
                          onChange={(e) =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.map((c, ci) =>
                                        ci === childIndex ? { ...c, label: e.target.value } : c,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        />
                        <select
                          aria-label="Connected device type"
                          className="rounded-md border border-border bg-surface px-2 py-1.5 text-xs"
                          value={child.role}
                          onChange={(e) =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.map((c, ci) =>
                                        ci === childIndex
                                          ? {
                                              ...c,
                                              role: e.target.value as "ap" | "other" | "uplink",
                                            }
                                          : c,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        >
                          <option value="ap">Access point</option>
                          <option value="other">Other device</option>
                          <option value="uplink">Uplink</option>
                        </select>
                        <button
                          type="button"
                          className="rounded-md border border-border px-2 py-1 text-[11px] text-red-200 hover:bg-red-500/10"
                          onClick={() =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.filter(
                                        (_, ci) => ci !== childIndex,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        >
                          Remove port
                        </button>
                      </div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        <input
                          aria-label="Connected device IP address"
                          inputMode="decimal"
                          className="rounded-md border border-border bg-surface px-2 py-1.5 font-mono text-xs"
                          placeholder="IP address · 192.168.88.20"
                          value={child.ipAddress ?? ""}
                          onChange={(e) =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.map((c, ci) =>
                                        ci === childIndex
                                          ? { ...c, ipAddress: e.target.value || null }
                                          : c,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        />
                        <input
                          aria-label="Connected device MAC address"
                          autoCapitalize="characters"
                          spellCheck={false}
                          className="rounded-md border border-border bg-surface px-2 py-1.5 font-mono text-xs"
                          placeholder="MAC address · AA:BB:CC:DD:EE:FF"
                          value={child.macAddress ?? ""}
                          onChange={(e) =>
                            setDraftDownstream((prev) =>
                              prev.map((row, i) =>
                                i === index
                                  ? {
                                      ...row,
                                      switchPorts: row.switchPorts.map((c, ci) =>
                                        ci === childIndex
                                          ? { ...c, macAddress: e.target.value || null }
                                          : c,
                                      ),
                                    }
                                  : row,
                              ),
                            )
                          }
                        />
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
