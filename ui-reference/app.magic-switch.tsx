import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { toErrorMessage } from "@/lib/error-message";
import { Switch } from "@/components/ui/switch";
import { MagicHubSparkles } from "@/components/MagicHubSparkle";
import { DelayedFallback, SkeletonList } from "@/components/ui/skeleton";
import {
  listMagicSwitches,
  magicSwitchUnlockStatus,
  purchaseMagicSwitchUnlock,
  createMagicSwitch,
  readMagicSwitchPorts,
  setMagicSwitchFeature,
  type MagicSwitch,
} from "@/lib/crs.functions";
import { listRouters } from "@/lib/routers.functions";

export const Route = createFileRoute("/_authenticated/app/magic-switch")({
  component: MagicSwitchPage,
  head: () => ({ meta: [{ title: "Magic Switch — MikroTik Magic" }] }),
});

const glass = "rounded-2xl border border-[color:var(--glass-border)] bg-[color:var(--glass-bg)] backdrop-blur-xl";

const statusChip = (cls: string, label: string) => (
  <span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider", cls)}>
    {label}
  </span>
);

export function MagicSwitchPage() {
  const t = useT();
  const qc = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [method, setMethod] = useState<"own-peer" | "router-relayed">("router-relayed");

  const list = useServerFn(listMagicSwitches);
  const unlockStatus = useServerFn(magicSwitchUnlockStatus);
  const purchase = useServerFn(purchaseMagicSwitchUnlock);

  const listQ = useQuery({ queryKey: ["magic-switches"], queryFn: () => list(), retry: false });
  const unlockQ = useQuery({ queryKey: ["magic-switch-unlock"], queryFn: () => unlockStatus(), retry: false });

  const purchaseMut = useMutation({
    mutationFn: () => purchase(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["magic-switch-unlock"] });
      toast.success("Magic Switch unlocked for 30 days.");
    },
    onError: (e) => toast.error(toErrorMessage(e)),
  });

  const switches: MagicSwitch[] = listQ.data ?? [];
  const selected = switches.find((s) => s.id === selectedId) ?? null;
  const key = unlockQ.data?.key as { active?: boolean; not_required?: boolean; price_coins?: number } | undefined;
  const ownPeerAvailable = Boolean(key?.active);
  const price = key?.price_coins ?? unlockQ.data?.price_coins ?? 20;

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-6">
      <header className="mb-6">
        <p className="text-kicker text-xs uppercase tracking-widest text-muted-foreground">{t.label("Operations")}</p>
        <div className="mt-1 flex items-center gap-3">
          <h1 className="text-3xl font-black tracking-tight text-title">{t.label("Magic Switch")}</h1>
          {statusChip("bg-emerald-400/15 text-emerald-300 border-emerald-400/40", t.label("MikroTik CRS"))}
        </div>
        <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
          {t.copy("Dual-boot RouterOS CRS switches, managed from the cloud. Router-relayed switches are included free; own-peer switches unlock with Magic Coins for 30 days.")}
        </p>
      </header>

      {/* Connection method — same card language as the Routers page */}
      <fieldset className="mb-4">
        <legend className="mb-1 block text-xs text-muted-foreground">{t.label("Connection method")}</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          <button
            type="button"
            aria-pressed={method === "own-peer"}
            onClick={() => setMethod("own-peer")}
            className={cn(
              "magic-hub-method rounded-xl border p-4 text-left text-xs transition",
              method === "own-peer"
                ? "is-selected overflow-visible border-primary/70 bg-primary/15"
                : "border-[color:var(--glass-border)] bg-white/5 hover:border-primary/40",
            )}
          >
            <MagicHubSparkles />
            <div className="text-kicker relative z-[1] text-[10px] uppercase tracking-wide">
              {ownPeerAvailable ? t.label("Own peer") : t.label("Locked")}
            </div>
            <div className="relative z-[1] text-sm font-semibold">{t.label("Magic Hub (own peer)")}</div>
            <p className="relative z-[1] mt-1 leading-snug text-muted-foreground">
              {ownPeerAvailable
                ? t.copy("The CRS dials into Magic Hub itself — no router in front.")
                : t.copy(`The CRS dials into Magic Hub itself. Unlock for ${price} Magic Coins / 30 days.`)}
            </p>
          </button>

          <button
            type="button"
            aria-pressed={method === "router-relayed"}
            onClick={() => setMethod("router-relayed")}
            className={cn(
              "rounded-xl border p-4 text-left text-xs transition",
              method === "router-relayed"
                ? "border-primary/60 bg-primary/10"
                : "border-[color:var(--glass-border)] bg-white/5 hover:border-primary/40",
            )}
          >
            <div className="text-kicker text-[10px] uppercase tracking-wide">{t.label("Free")}</div>
            <div className="text-sm font-semibold">{t.label("Router-relayed")}</div>
            <p className="mt-1 leading-snug text-muted-foreground">
              {t.copy("The switch sits behind a router already on Magic Hub. Included at no cost.")}
            </p>
          </button>
        </div>

        {unlockQ.data && !ownPeerAvailable && (
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-500/30 bg-amber-500/5 p-4">
            <p className="text-xs text-amber-100">
              {t.copy("Own-peer CRS is locked. Unlock it to add a CRS as its own Magic Hub peer.")}
            </p>
            <button
              className="cta-glow animate-glow rounded-full px-5 py-2 text-sm font-bold"
              onClick={() => purchaseMut.mutate()}
              disabled={purchaseMut.isPending}
            >
              {purchaseMut.isPending ? "Unlocking…" : `Unlock for ${price} coins`}
            </button>
          </div>
        )}
      </fieldset>

      {/* Add action for the selected method */}
      <AddMagicSwitch
        method={method}
        ownPeerAvailable={ownPeerAvailable}
        onAdded={() => {
          qc.invalidateQueries({ queryKey: ["magic-switches"] });
        }}
      />

      {listQ.isLoading && (
        <DelayedFallback loading label="Loading switches" fallback={<SkeletonList rows={3} />} />
      )}
      {listQ.isError && <p className="text-sm text-red-500">{toErrorMessage(listQ.error)}</p>}
      <div className="mt-6 grid gap-6 lg:grid-cols-[280px_1fr]">
        <div className="space-y-2">
          {switches.map((s) => {
            const active = selectedId === s.id;
            return (
              <button
                key={s.id}
                onClick={() => setSelectedId(s.id)}
                className={cn(
                  "w-full rounded-2xl border px-4 py-3 text-left transition hover:-translate-y-0.5 hover:shadow-lg backdrop-blur-xl",
                  active
                    ? "border-primary/50 bg-primary/10 shadow-lg"
                    : "border-[color:var(--glass-border)] bg-[color:var(--glass-bg)] hover:border-primary/40",
                )}
              >
                <span className="font-semibold text-title">{s.name}</span>
                <div className="mt-1 flex items-center gap-2">
                  <span className="text-xs text-muted-foreground">{s.model ?? "Unknown model"}</span>
                  {s.ownsPeer
                    ? s.unlocked
                      ? statusChip("bg-emerald-400/15 text-emerald-300 border-emerald-400/40", "own-peer")
                      : statusChip("bg-amber-400/15 text-amber-300 border-amber-400/40", "own-peer · locked")
                    : statusChip("bg-sky-400/15 text-sky-300 border-sky-400/40", "router-relayed")}
                </div>
              </button>
            );
          })}
        </div>

        <div className="grid gap-6 md:grid-cols-2">
          <PortsPanel deviceId={selected?.id ?? null} />
          <SwitchConfigPanel deviceId={selected?.id ?? null} />
        </div>
      </div>
    </div>
  );
}

function AddMagicSwitch({
  method,
  ownPeerAvailable,
  onAdded,
}: {
  method: "own-peer" | "router-relayed";
  ownPeerAvailable: boolean;
  onAdded: () => void;
}) {
  const t = useT();
  const create = useServerFn(createMagicSwitch);
  const fetchRouters = useServerFn(listRouters);
  const [name, setName] = useState("");
  const [model, setModel] = useState("");
  const [routerId, setRouterId] = useState("");
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [script, setScript] = useState<string | null>(null);

  const routersQ = useQuery({
    queryKey: ["routers"],
    queryFn: () => fetchRouters(),
    enabled: method === "router-relayed",
    retry: false,
  });

  const mut = useMutation({
    mutationFn: () =>
      create({
        data: {
          name: name.trim(),
          model: model.trim() || undefined,
          method,
          routerId: method === "router-relayed" ? routerId || undefined : undefined,
          username: method === "own-peer" ? username.trim() || "admin" : undefined,
          password: method === "own-peer" ? password : undefined,
          port: 443,
        },
      }),
    onSuccess: (res) => {
      const r = res as { method: string; script: string | null };
      toast.success(r.method === "own-peer" ? "Switch peer created — paste the script on the CRS." : "Switch added.");
      setScript(r.script ?? null);
      setName("");
      setModel("");
      setPassword("");
      onAdded();
    },
    onError: (e) => toast.error(toErrorMessage(e)),
  });

  const disabled = !name.trim() || (method === "router-relayed" ? !routerId : !ownPeerAvailable) || mut.isPending;

  return (
    <section className={cn("mb-6 p-5", glass)}>
      <h2 className="mb-1 text-lg font-bold text-title">
        {method === "own-peer" ? t.label("Add own-peer Magic Switch") : t.label("Add router-relayed Magic Switch")}
      </h2>
      <p className="mb-4 text-xs text-muted-foreground">
        {method === "own-peer"
          ? t.copy("Creates the switch's own Magic Hub peer and gives you a RouterOS paste script.")
          : t.copy("Registers a CRS that sits behind a router already connected to Magic Hub.")}
      </p>

      <div className="grid gap-3 sm:grid-cols-3">
        <label className="block">
          <span className="mb-1 block text-xs text-muted-foreground">{t.label("Switch name")}</span>
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="CRS317" />
        </label>
        <label className="block">
          <span className="mb-1 block text-xs text-muted-foreground">{t.label("Model")}</span>
          <input className="input w-full" value={model} onChange={(e) => setModel(e.target.value)} placeholder="CRS317-1G-16S+" />
        </label>

        {method === "router-relayed" ? (
          <label className="block">
            <span className="mb-1 block text-xs text-muted-foreground">{t.label("Behind router")}</span>
            <select className="input w-full" value={routerId} onChange={(e) => setRouterId(e.target.value)}>
              <option value="">{t.label("Select a router")}</option>
              {(routersQ.data ?? []).map((r: { id: string; name: string }) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <>
            <label className="block">
              <span className="mb-1 block text-xs text-muted-foreground">{t.label("WinBox user")}</span>
              <input className="input w-full" value={username} onChange={(e) => setUsername(e.target.value)} />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-muted-foreground">{t.label("WinBox password")}</span>
              <input
                className="input w-full"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
          </>
        )}
      </div>

      {method === "own-peer" && !ownPeerAvailable && (
        <p className="mt-3 text-xs text-amber-200">{t.copy("Unlock own-peer above first.")}</p>
      )}

      <button
        className="mt-4 rounded-lg border border-white/15 bg-white/5 px-4 py-2 text-sm font-semibold uppercase tracking-wider text-title transition hover:bg-white/10 disabled:opacity-40"
        disabled={disabled}
        onClick={() => mut.mutate()}
      >
        {mut.isPending
          ? "Working…"
          : method === "own-peer"
            ? t.label("Add & get script")
            : t.label("Add switch")}
      </button>

      {script && (
        <div className="mt-4 rounded-xl border border-emerald-500/40 bg-emerald-500/5 p-4">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="text-xs font-semibold text-emerald-100">
              {t.copy("Paste this into the CRS New Terminal (WinBox):")}
            </p>
            <button
              className="rounded-lg border border-white/15 bg-white/5 px-3 py-1 text-xs font-semibold text-title hover:bg-white/10"
              onClick={() => {
                void navigator.clipboard.writeText(script).then(() => toast.success("Script copied."));
              }}
            >
              {t.label("Copy")}
            </button>
          </div>
          <textarea
            readOnly
            className="input h-56 w-full font-mono text-[11px]"
            value={script}
            onFocus={(e) => e.currentTarget.select()}
          />
        </div>
      )}
    </section>
  );
}

function PortsPanel({ deviceId }: { deviceId: string | null }) {
  const t = useT();
  const readPorts = useServerFn(readMagicSwitchPorts);

  const portsQ = useQuery({
    queryKey: ["magic-switch-ports", deviceId],
    queryFn: () => readPorts({ data: { deviceId: deviceId! } }),
    enabled: !!deviceId,
    retry: false,
  });

  return (
    <section className={cn("p-5", glass)}>
      <h2 className="mb-1 text-lg font-bold text-title">{t.label("Ports")}</h2>
      <p className="mb-4 text-xs text-muted-foreground">{t.copy("Plan A — port power, link state and PoE")}</p>
      {!deviceId && <p className="text-sm text-muted-foreground">{t.copy("Select a switch to inspect its ports.")}</p>}
      {portsQ.isLoading && (
        <DelayedFallback loading label="Loading ports" fallback={<SkeletonList rows={3} />} />
      )}
      {portsQ.isError && <p className="text-sm text-red-500">{toErrorMessage(portsQ.error)}</p>}
      {portsQ.data &&
        (styledPorts(portsQ.data.ports) ?? []).map((p) => (
          <div key={p.ref} className="flex items-center justify-between border-b border-[color:var(--glass-border)] py-2.5 last:border-0">
            <div className="flex items-center gap-2">
              <span className="font-mono text-sm text-title">{p.name}</span>
              {p.down ? (
                statusChip("bg-red-500/15 text-red-300 border-red-500/40", t.label("Down"))
              ) : (
                statusChip("bg-emerald-400/15 text-emerald-300 border-emerald-400/40", t.label("Up"))
              )}
              {p.poe ? (
                <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                  {t.label("PoE")} {p.poeWatts != null ? `${p.poeWatts}W` : ""}
                </span>
              ) : null}
            </div>
            <span className="text-[10px] uppercase tracking-wide text-muted-foreground/50">{t.label("Config disabled")}</span>
          </div>
        ))}
    </section>
  );
}

function styledPorts(ports?: unknown): Array<{ ref: string; name: string; down: boolean; poe: boolean; poeWatts: number | null }> | undefined {
  return (ports as Array<{ ref?: string; name?: string; enabled?: boolean; poe?: boolean; poeWatts?: number | null }> | undefined)?.map((p) => ({
    ref: p.ref ?? p.name ?? "",
    name: p.name ?? "?",
    down: p.enabled === false,
    poe: Boolean(p.poe),
    poeWatts: p.poeWatts ?? null,
  }));
}

function SwitchConfigPanel({ deviceId }: { deviceId: string | null }) {
  const t = useT();

  const rows: Array<{ key: string; label: string; hint: string }> = [
    { key: "rstp", label: "RSTP (Spanning Tree)", hint: "Prevents loops on redundant uplinks." },
    { key: "loopProtect", label: "Loop protection", hint: "Blocks ports that see the switch's own frames." },
    { key: "igmpSnooping", label: "IGMP snooping", hint: "Sends multicast only to interested ports." },
    { key: "stormControl", label: "Storm control", hint: "Limits broadcast/multicast flooding." },
  ];

  return (
    <section className={cn("p-5", glass)}>
      <h2 className="mb-1 text-lg font-bold text-title">{t.label("Switch Config")}</h2>
      <p className="mb-4 text-xs text-muted-foreground">{t.copy("Plan B — bridge-wide behavior, applied via the hub link")}</p>
      {!deviceId && <p className="text-sm text-muted-foreground">{t.copy("Select a switch to configure it.")}</p>}
      {rows.map((r) => (
        <div key={r.key} className="flex items-center justify-between gap-3 border-b border-[color:var(--glass-border)] py-3 last:border-0">
          <div>
            <p className="text-sm font-semibold text-title">{t.label(r.label)}</p>
            <p className="text-xs text-muted-foreground">{t.copy(r.hint)}</p>
          </div>
          <Switch disabled checked={false} />
        </div>
      ))}
    </section>
  );
}
