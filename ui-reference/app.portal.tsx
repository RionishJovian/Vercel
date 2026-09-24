import { createFileRoute, Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { useEffect, useMemo, useState } from "react";
import { useT } from "@/lib/i18n";
import {
  getPortalSettings,
  savePortalSettings,
  uploadPortalAsset,
  getSignedAssetUrl,
  buildPortalZip,
  listDeployableRouters,
  publishPortalToRouter,
  listPortalDeploys,
  rollbackPortalDeploy,
  getPortalDeployProbe,
  scanPortalOnDevice,
} from "@/lib/portal.functions";
import { getMyPortalModePermissions } from "@/lib/portal-grants.functions";
import { toast } from "sonner";
import { HotspotGuestReadyBanner } from "@/components/HotspotGuestReadyBanner";
import { PortalDeployStatusBanner } from "@/components/PortalDeployStatusBanner";
import { TypedConfirmDialog } from "@/components/TypedConfirmDialog";
import { portalDeployGate } from "@/lib/test-router";
import { PortalGuestModePanel, type PortalGuestDraft } from "@/components/PortalGuestModePanel";
import { PlansPanel } from "@/components/PlansPanel";
import {
  DEFAULT_PAYMENT_METHODS,
  parsePaymentMethods,
  type PortalGuestMode,
} from "@/lib/portal/modes";
import { useSelectedSite } from "@/hooks/useSelectedSite";
import { cssUrl, safeCssHex } from "@/lib/html-escape";
import { toErrorMessage } from "@/lib/error-message";
import { ROUTER_OPS_AUDIT_QUERY_KEY } from "@/lib/audit.functions";

export const Route = createFileRoute("/_authenticated/app/portal")({
  head: () => ({
    meta: [
      { title: "Portal designer — MikroTik Hotspot Admin" },
      {
        name: "description",
        content:
          "Design and publish the hotspot login portal: branding, guest modes, logo, hero assets, and one-click router deploy.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: PortalPage,
});

interface Draft extends PortalGuestDraft {
  business_name: string;
  welcome_text: string;
  terms: string;
  primary_hex: string;
  glass_tint_hex: string;
  logo_path: string | null;
  hero_path: string | null;
}

function PortalPage() {
  const t = useT();
  const qc = useQueryClient();
  const fetchSettings = useServerFn(getPortalSettings);
  const saveSettings = useServerFn(savePortalSettings);
  const upload = useServerFn(uploadPortalAsset);
  const signUrl = useServerFn(getSignedAssetUrl);
  const buildZip = useServerFn(buildPortalZip);
  const fetchPerms = useServerFn(getMyPortalModePermissions);

  const settings = useQuery({ queryKey: ["portal-settings"], queryFn: () => fetchSettings() });
  const perms = useQuery({
    queryKey: ["portal-mode-permissions"],
    queryFn: () => fetchPerms(),
  });
  const [draft, setDraft] = useState<Draft | null>(null);
  const [logoUrl, setLogoUrl] = useState<string | null>(null);
  const [heroUrl, setHeroUrl] = useState<string | null>(null);

  useEffect(() => {
    if (settings.data && !draft) {
      const s = settings.data as Record<string, unknown>;
      setDraft({
        business_name: settings.data.business_name,
        welcome_text: settings.data.welcome_text,
        terms: settings.data.terms ?? "",
        primary_hex: settings.data.primary_hex,
        glass_tint_hex: settings.data.glass_tint_hex,
        logo_path: settings.data.logo_path,
        hero_path: settings.data.hero_path,
        guest_mode: (s.guest_mode as PortalGuestMode) || "voucher_only",
        payment_methods: parsePaymentMethods(s.payment_methods ?? s.commerce_channels),
        seller_phone: (s.seller_phone as string | null) ?? "",
        seller_label: (s.seller_label as string | null) ?? "Talk to our seller",
        trial_minutes: Number(s.trial_minutes ?? 10),
        trial_cooldown_hours: Number(s.trial_cooldown_hours ?? 12),
        need_code_label: (s.need_code_label as string | null) ?? "I don't have a code",
        ask_desk_hint:
          (s.ask_desk_hint as string | null) ?? "Ask the front desk for a Wi-Fi voucher code.",
      });
    }
  }, [settings.data, draft]);

  useEffect(() => {
    if (draft?.logo_path)
      signUrl({ data: { path: draft.logo_path } }).then((r) => setLogoUrl(r.url));
    if (draft?.hero_path)
      signUrl({ data: { path: draft.hero_path } }).then((r) => setHeroUrl(r.url));
  }, [draft?.logo_path, draft?.hero_path, signUrl]);

  const saveMut = useMutation({
    mutationFn: (d: Draft) =>
      saveSettings({
        data: {
          business_name: d.business_name,
          welcome_text: d.welcome_text,
          terms: d.terms,
          primary_hex: d.primary_hex,
          glass_tint_hex: d.glass_tint_hex,
          logo_path: d.logo_path,
          hero_path: d.hero_path,
          guest_mode: d.guest_mode,
          payment_methods: d.payment_methods.length ? d.payment_methods : DEFAULT_PAYMENT_METHODS,
          seller_phone: d.seller_phone || null,
          seller_label: d.seller_label,
          trial_minutes: d.trial_minutes,
          trial_cooldown_hours: d.trial_cooldown_hours,
          need_code_label: d.need_code_label,
          ask_desk_hint: d.ask_desk_hint,
        },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["portal-settings"] });
      toast.success("Portal settings saved");
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const uploadMut = useMutation({
    mutationFn: (v: { kind: "logo" | "hero"; dataUrl: string }) => upload({ data: v }),
    onSuccess: (r, v) => {
      setDraft((d) => (d ? ({ ...d, [`${v.kind}_path`]: r.path } as Draft) : d));
      qc.invalidateQueries({ queryKey: ["portal-settings"] });
    },
  });

  const zipMut = useMutation({
    mutationFn: () => buildZip(),
    onSuccess: (r) => {
      const bin = atob(r.base64);
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      const url = URL.createObjectURL(new Blob([bytes], { type: "application/zip" }));
      const a = document.createElement("a");
      a.href = url;
      a.download = r.filename;
      a.click();
      URL.revokeObjectURL(url);
    },
  });

  if (settings.isError) {
    return (
      <div className="panel space-y-3 p-5">
        <h2 className="text-sm font-semibold text-danger">Couldn’t load portal settings</h2>
        <p className="text-xs text-muted-foreground">{(settings.error as Error).message}</p>
        <button
          onClick={() => settings.refetch()}
          className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!draft) return <div className="panel p-5">Loading portal settings…</div>;

  const onFile = (kind: "logo" | "hero") => (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > 3_000_000) return alert("Image must be under 3 MB");
    const reader = new FileReader();
    reader.onload = () => uploadMut.mutate({ kind, dataUrl: reader.result as string });
    reader.readAsDataURL(file);
  };

  return (
    <div className="space-y-6">
      <h1 className="sr-only">Portal Customization</h1>
      <PortalGuestModePanel
        draft={{
          guest_mode: draft.guest_mode,
          payment_methods: draft.payment_methods,
          seller_phone: draft.seller_phone,
          seller_label: draft.seller_label,
          trial_minutes: draft.trial_minutes,
          trial_cooldown_hours: draft.trial_cooldown_hours,
          need_code_label: draft.need_code_label,
          ask_desk_hint: draft.ask_desk_hint,
        }}
        allowedModes={perms.data?.allowed ?? ["voucher_only"]}
        onChange={(guest) => setDraft({ ...draft, ...guest })}
      />
      {draft.guest_mode === "commerce" && (
        <section id="guest-packages" className="scroll-mt-24 space-y-3">
          <div className="panel flex flex-wrap items-center justify-between gap-3 p-4">
            <div className="min-w-0">
              <h2 className="text-sm font-semibold">Guest packages</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                These plans appear on the Hotspot <code className="text-[10px]">packages.html</code>{" "}
                page when Guest self-checkout is enabled. Add packages here, then deploy the portal.
              </p>
            </div>
            <Link
              to="/app/vouchers"
              hash="plans"
              className="inline-flex min-h-11 shrink-0 items-center rounded-full border border-[color:var(--glass-border)] px-4 text-xs font-medium transition hover:border-primary hover:text-primary"
            >
              Open vouchers →
            </Link>
          </div>
          <PlansPanel />
        </section>
      )}
      {draft.guest_mode !== "commerce" && (
        <section className="panel mb-4 flex flex-wrap items-center justify-between gap-3 p-4">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold">{t.ui("Voucher plans")}</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Manage voucher profiles in Vouchers. Switch to Guest self-checkout to show the package
              picker on the captive portal.
            </p>
          </div>
          <Link
            to="/app/vouchers"
            hash="plans"
            className="inline-flex min-h-11 shrink-0 items-center rounded-full border border-[color:var(--glass-border)] px-4 text-xs font-medium transition hover:border-primary hover:text-primary"
          >
            Open vouchers →
          </Link>
        </section>
      )}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,420px)_1fr]">
        <section className="panel p-5 space-y-4">
          <h2 className="text-sm font-semibold">Portal content</h2>
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">Business name</span>
            <input
              className="input"
              value={draft.business_name}
              onChange={(e) => setDraft({ ...draft, business_name: e.target.value })}
            />
          </label>
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">Welcome text</span>
            <textarea
              rows={3}
              className="input"
              value={draft.welcome_text}
              onChange={(e) => setDraft({ ...draft, welcome_text: e.target.value })}
            />
          </label>
          <label className="block text-xs">
            <span className="mb-1 block text-muted-foreground">Terms footer</span>
            <textarea
              rows={2}
              className="input"
              value={draft.terms}
              onChange={(e) => setDraft({ ...draft, terms: e.target.value })}
            />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-xs">
              <span className="mb-1 block text-muted-foreground">Primary color</span>
              <input
                type="color"
                className="h-10 w-full rounded-md border border-border bg-surface"
                value={draft.primary_hex}
                onChange={(e) => setDraft({ ...draft, primary_hex: e.target.value })}
              />
            </label>
            <label className="block text-xs">
              <span className="mb-1 block text-muted-foreground">Glass tint</span>
              <input
                type="color"
                className="h-10 w-full rounded-md border border-border bg-surface"
                value={draft.glass_tint_hex}
                onChange={(e) => setDraft({ ...draft, glass_tint_hex: e.target.value })}
              />
            </label>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-xs">
              <span className="mb-1 block text-muted-foreground">Logo</span>
              <input type="file" accept="image/*" onChange={onFile("logo")} className="text-xs" />
            </label>
            <label className="block text-xs">
              <span className="mb-1 block text-muted-foreground">Hero image</span>
              <input type="file" accept="image/*" onChange={onFile("hero")} className="text-xs" />
            </label>
          </div>
          {uploadMut.isPending && <p className="text-xs text-muted-foreground">Uploading…</p>}
          {(saveMut.error || uploadMut.error || zipMut.error) && (
            <p className="text-xs text-danger">
              {((saveMut.error ?? uploadMut.error ?? zipMut.error) as Error).message}
            </p>
          )}
          <div className="flex gap-2 pt-2">
            <button
              onClick={() => saveMut.mutate(draft)}
              disabled={saveMut.isPending}
              className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60"
            >
              {saveMut.isPending ? "Saving…" : "Save"}
            </button>
            <button
              onClick={() => zipMut.mutate()}
              disabled={zipMut.isPending}
              className="rounded-md border border-border px-3 py-2 text-sm"
            >
              {zipMut.isPending ? "Building ZIP…" : "Download ZIP"}
            </button>
          </div>
          <p className="text-[11px] text-muted-foreground">
            Save your edits, then use the <b>Deploy</b> panel below to push directly to the router —
            no WinBox or manual upload needed. The ZIP is only for offline backups.
          </p>
        </section>

        <section className="panel p-5">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold">Live preview</h2>
            <span className="text-[11px] text-muted-foreground">
              This is what mobile users will see.
            </span>
          </div>
          <PortalPreview draft={draft} logoUrl={logoUrl} heroUrl={heroUrl} signUrl={signUrl} />
        </section>
      </div>

      <DeployPanel />
    </div>
  );
}

function DeployPanel() {
  const qc = useQueryClient();
  const fetchRouters = useServerFn(listDeployableRouters);
  const fetchDeploys = useServerFn(listPortalDeploys);
  const fetchProbe = useServerFn(getPortalDeployProbe);
  const scanDevice = useServerFn(scanPortalOnDevice);
  const publish = useServerFn(publishPortalToRouter);
  const rollback = useServerFn(rollbackPortalDeploy);
  const { site: selectedSite } = useSelectedSite();

  const routers = useQuery({ queryKey: ["deployable-routers"], queryFn: () => fetchRouters() });
  const deploys = useQuery({ queryKey: ["portal-deploys"], queryFn: () => fetchDeploys() });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [forceRepublish, setForceRepublish] = useState(false);
  const [scan, setScan] = useState<Awaited<ReturnType<typeof scanDevice>> | null>(null);

  const visibleRouters = useMemo(
    () => (routers.data ?? []).filter((r) => (selectedSite ? r.site_id === selectedSite.id : true)),
    [routers.data, selectedSite],
  );

  useEffect(() => {
    if (visibleRouters.length === 0) {
      setSelected(new Set());
      return;
    }
    setSelected((prev) => {
      const next = new Set([...prev].filter((id) => visibleRouters.some((r) => r.id === id)));
      if (next.size === 0) return new Set(visibleRouters.map((r) => r.id));
      return next;
    });
  }, [visibleRouters]);

  const selectedRouters = useMemo(
    () => visibleRouters.filter((r) => selected.has(r.id)),
    [visibleRouters, selected],
  );
  const primaryRouter = selectedRouters[0] ?? null;
  const portalProbe = useQuery({
    queryKey: ["portal-deploy-probe", primaryRouter?.id],
    queryFn: () => fetchProbe({ data: { routerId: primaryRouter!.id } }),
    enabled: Boolean(primaryRouter?.id) && selected.size === 1,
    staleTime: 20_000,
  });
  const portalMatches =
    selected.size === 1 && portalProbe.data?.status === "matches" && !portalProbe.isLoading;
  const gate = useMemo(() => {
    if (!selectedRouters.length) return { error: "Select at least one router." as const };
    if (selectedRouters.length === 1 && portalProbe.isLoading) return { pending: true as const };
    if (selectedRouters.length === 1 && portalProbe.isError)
      return {
        error: `Portal publish is blocked until the router probe succeeds. ${toErrorMessage(portalProbe.error)}`,
      };
    try {
      return portalDeployGate(selectedRouters);
    } catch (e) {
      return { error: e instanceof Error ? e.message : String(e) };
    }
  }, [portalProbe.error, portalProbe.isError, portalProbe.isLoading, selectedRouters]);

  const scanMut = useMutation({
    mutationFn: (routerId: string) => scanDevice({ data: { routerId } }),
    onSuccess: (result) => setScan(result),
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const publishMut = useMutation({
    mutationFn: (input: { ids: string[]; confirmation: string; forceRepublish: boolean }) =>
      publish({
        data: {
          routerIds: input.ids,
          confirmation: input.confirmation,
          forceRepublish: input.forceRepublish,
        },
      }),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["portal-deploys"] });
      qc.invalidateQueries({ queryKey: ["portal-deploy-probe"] });
      void qc.invalidateQueries({ queryKey: ["magic-go-live"] });
      void qc.invalidateQueries({ queryKey: ROUTER_OPS_AUDIT_QUERY_KEY });
      setConfirmOpen(false);
      setForceRepublish(false);
      const skipped = data.results.filter((r) => r.skipped);
      const bad = data.results.filter((r) => !r.ok && !r.skipped);
      if (bad.length) toast.error(bad[0]!.error || "Portal deploy failed");
      else if (skipped.length === data.results.length)
        toast.success("Portal already matches saved settings — no files written");
      else if (skipped.length)
        toast.success("Published where needed; unchanged routers were skipped");
      else toast.success("Portal published to the board");
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });

  const rollbackMut = useMutation({
    mutationFn: (id: string) => rollback({ data: { deployId: id } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["portal-deploys"] });
      void qc.invalidateQueries({ queryKey: ROUTER_OPS_AUDIT_QUERY_KEY });
    },
  });

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <section className="panel p-5 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">Deploy portal to router</h2>
          <p className="text-[11px] text-muted-foreground">
            Pushes HTML/CSS to a versioned hotspot directory, then switches the selected hotspot
            profiles on the connected MikroTik. A per-file snapshot is saved so you can roll back
            with one click.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            onClick={() => primaryRouter && scanMut.mutate(primaryRouter.id)}
            disabled={scanMut.isPending || selected.size !== 1}
            className="rounded-md border border-border px-3 py-2 text-sm disabled:opacity-60"
          >
            {scanMut.isPending ? "Scanning device…" : "Scan portal on device"}
          </button>
          <button
            onClick={() => {
              setForceRepublish(portalMatches);
              setConfirmOpen(true);
            }}
            disabled={
              publishMut.isPending || selected.size === 0 || "error" in gate || "pending" in gate
            }
            className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60"
          >
            {publishMut.isPending
              ? "Publishing…"
              : portalMatches
                ? "Republish anyway"
                : `Publish to ${selected.size} router${selected.size === 1 ? "" : "s"}`}
          </button>
        </div>
      </div>
      <p className="text-[11px] text-muted-foreground">
        Scan is read-only and needs exactly one router. It inventories the active Hotspot files
        without opening device HTML in the app. Magic-managed layouts can be republished from saved
        settings; a custom portal is reported for backup and review before replacement.
      </p>
      {scan && (
        <div className="rounded-md border border-border bg-surface p-3 text-xs space-y-2">
          <p className="font-medium">
            {scan.routerName}: {scan.summary}
          </p>
          {scan.profiles.map((profile) => (
            <p key={profile.id} className="text-muted-foreground">
              {profile.name} → {profile.htmlDirectory}
            </p>
          ))}
          {scan.directories.map((directory) => (
            <p key={directory.directory} className="text-muted-foreground">
              {directory.directory}: {directory.kind} · {directory.files.length} readable file(s)
              {directory.managedFingerprint ? ` · fingerprint ${directory.managedFingerprint}` : ""}
            </p>
          ))}
        </div>
      )}
      {portalMatches && selected.size === 1 && (
        <p className="text-xs text-emerald-400">
          Saved settings already match the live portal on {primaryRouter?.name}. Use Republish
          anyway only if phones still show old branding.
        </p>
      )}
      {"error" in gate && selected.size > 0 && <p className="text-xs text-danger">{gate.error}</p>}

      {selectedRouters.slice(0, 1).map((r) => (
        <div key={r.id} className="space-y-2">
          <HotspotGuestReadyBanner routerId={r.id} routerName={r.name} context="portal" />
          {selected.size === 1 && <PortalDeployStatusBanner routerId={r.id} routerName={r.name} />}
        </div>
      ))}

      <div className="grid gap-2 sm:grid-cols-2">
        {(routers.data?.length ?? 0) === 0 && (
          <p className="text-xs text-muted-foreground">No routers registered yet.</p>
        )}
        {(routers.data?.length ?? 0) > 0 && visibleRouters.length === 0 && (
          <p className="text-xs text-muted-foreground">
            {selectedSite
              ? `No routers on site “${selectedSite.name}”. Clear the site filter or assign a router on Sites.`
              : "No routers match the current filter."}
          </p>
        )}
        {visibleRouters.map((r) => (
          <label
            key={r.id}
            className="flex cursor-pointer items-center gap-2 rounded-md border border-border bg-surface px-3 py-2 text-xs"
          >
            <input type="checkbox" checked={selected.has(r.id)} onChange={() => toggle(r.id)} />
            <span className="font-medium">{r.name}</span>
            <span className="text-muted-foreground">{r.host}</span>
          </label>
        ))}
      </div>

      {publishMut.data && (
        <div className="rounded-md border border-border bg-surface p-3 text-xs space-y-1">
          {publishMut.data.results.map((r) => (
            <div key={r.routerId} className="flex items-start gap-2">
              <span className={r.ok ? "text-emerald-500" : "text-danger"}>{r.ok ? "✓" : "✗"}</span>
              <span className="font-medium">{r.routerName}</span>
              <span className="text-muted-foreground">
                {r.skipped
                  ? "skipped — already matches saved settings"
                  : r.ok
                    ? `${r.files?.length ?? 0} files pushed`
                    : r.error}
              </span>
            </div>
          ))}
        </div>
      )}
      {publishMut.error && (
        <p className="text-xs text-danger">{(publishMut.error as Error).message}</p>
      )}

      <div>
        <h3 className="mb-2 text-xs font-semibold text-muted-foreground">Recent deploys</h3>
        <div className="space-y-1 text-xs">
          {deploys.data?.length === 0 && <p className="text-muted-foreground">No deploys yet.</p>}
          {deploys.data?.map((d) => (
            <div
              key={d.id}
              className="flex items-center justify-between rounded-md border border-border bg-surface px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <span className={d.ok ? "text-emerald-500" : "text-danger"}>
                  {d.ok ? "✓" : "✗"}
                </span>
                <span className="font-medium">{d.router_name}</span>
                <span className="text-muted-foreground">
                  {new Date(d.created_at).toLocaleString()}
                </span>
                {d.duration_ms != null && (
                  <span className="text-muted-foreground">· {d.duration_ms}ms</span>
                )}
                {!d.ok && d.error && (
                  <span className="min-w-0 break-words text-danger">
                    · {toErrorMessage(d.error)}
                  </span>
                )}
              </div>
              {d.ok && (
                <button
                  onClick={() => rollbackMut.mutate(d.id)}
                  disabled={rollbackMut.isPending}
                  className="rounded-md border border-border px-2 py-1 text-[11px] disabled:opacity-60"
                >
                  Rollback
                </button>
              )}
            </div>
          ))}
        </div>
        {rollbackMut.error && (
          <p className="mt-2 text-xs text-danger">{(rollbackMut.error as Error).message}</p>
        )}
        {rollbackMut.data && (
          <p className="mt-2 text-xs text-emerald-500">
            Restored {rollbackMut.data.restored} file(s).
          </p>
        )}
      </div>
      <TypedConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => {
          if (!publishMut.isPending) setConfirmOpen(open);
        }}
        title={portalMatches ? "Republish portal anyway?" : "Deploy the captive portal?"}
        description={
          portalMatches
            ? `The live portal on ${primaryRouter?.name ?? "this router"} already matches your saved settings. Republishing writes a fresh folder. Continue, then type the confirmation phrase.`
            : `This publishes the login HTML to ${selected.size} router(s). Continue, then type the confirmation phrase.`
        }
        typeHint="Type the English phrase below exactly to confirm."
        phrase={"expected" in gate ? gate.expected : ""}
        confirmLabel="Publish"
        pending={publishMut.isPending}
        pendingLabel="Publishing…"
        onConfirm={(typed) => {
          publishMut.mutate({
            ids: Array.from(selected),
            confirmation: typed,
            forceRepublish: portalMatches || forceRepublish,
          });
        }}
      />
    </section>
  );
}

function PortalPreview({
  draft,
  logoUrl,
  heroUrl,
  signUrl,
}: {
  draft: Draft;
  logoUrl: string | null;
  heroUrl: string | null;
  signUrl: (args: { data: { path: string } }) => Promise<{ url: string }>;
}) {
  const payMethod = draft.payment_methods.find((m) => m.enabled && m.action === "pay_info");
  const showPayTab = draft.guest_mode === "commerce" && !!payMethod;
  const [tab, setTab] = useState<"login" | "mmqr">(showPayTab ? "mmqr" : "login");
  const [qrUrl, setQrUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!showPayTab && tab === "mmqr") setTab("login");
  }, [showPayTab, tab]);

  useEffect(() => {
    let cancelled = false;
    const path = payMethod?.qrImagePath;
    if (!path) {
      setQrUrl(null);
      return;
    }
    signUrl({ data: { path } })
      .then((r) => {
        if (!cancelled) setQrUrl(r.url);
      })
      .catch(() => {
        if (!cancelled) setQrUrl(null);
      });
    return () => {
      cancelled = true;
    };
  }, [payMethod?.qrImagePath, signUrl]);

  const mesh = `radial-gradient(35% 40% at 20% 25%, ${safeCssHex(draft.primary_hex, "#ffb547")} 0%, transparent 60%),
    radial-gradient(30% 35% at 80% 20%, ${safeCssHex(draft.glass_tint_hex, "#7ad0ff")} 0%, transparent 60%),
    radial-gradient(40% 45% at 60% 90%, #7aa2ff 0%, transparent 65%),
    radial-gradient(30% 35% at 10% 90%, #ff7ad9 0%, transparent 60%)`;

  return (
    <div className="space-y-3">
      {showPayTab ? (
        <div className="flex gap-1 rounded-lg border border-border bg-background/40 p-1">
          <button
            type="button"
            onClick={() => setTab("login")}
            className={`flex-1 rounded-md px-2 py-1.5 text-xs font-medium ${
              tab === "login"
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            Login
          </button>
          <button
            type="button"
            onClick={() => setTab("mmqr")}
            className={`flex-1 rounded-md px-2 py-1.5 text-xs font-medium ${
              tab === "mmqr"
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            MMQR pay
          </button>
        </div>
      ) : null}
      <div className="relative mx-auto aspect-[9/16] max-w-[320px] overflow-hidden rounded-[36px] border border-border shadow-2xl">
        <div
          className="absolute inset-0"
          style={{
            background: heroUrl ? `${cssUrl(heroUrl)} center/cover` : "#05070f",
          }}
        />
        <div
          className="absolute -inset-8 opacity-90"
          style={{ background: mesh, filter: "blur(50px) saturate(140%)" }}
        />
        <div className="relative flex h-full w-full items-center justify-center p-5">
          <div
            className="w-full rounded-[24px] border border-white/25 p-5 text-center text-white shadow-2xl"
            style={{
              background: "linear-gradient(180deg, rgba(255,255,255,.18), rgba(255,255,255,.06))",
              backdropFilter: "blur(24px) saturate(180%)",
            }}
          >
            {tab === "login" || !payMethod ? (
              <>
                <div
                  className="mx-auto mb-3 h-12 w-12 rounded-2xl border border-white/25 bg-white/15"
                  style={logoUrl ? { background: `${cssUrl(logoUrl)} center/cover` } : undefined}
                />
                <div className="text-base font-semibold">{draft.business_name}</div>
                <p className="mt-1 text-[11px] text-white/75">{draft.welcome_text}</p>
                <div className="mt-4 space-y-2">
                  <div className="rounded-xl border border-white/25 bg-white/10 px-3 py-2 text-[11px] tracking-widest text-white/70">
                    VOUCHER CODE
                  </div>
                  <button
                    type="button"
                    className="w-full rounded-xl py-2 text-xs font-semibold text-slate-900"
                    style={{
                      background: `linear-gradient(180deg, #fff, ${safeCssHex(draft.primary_hex, "#ffb547")})`,
                    }}
                  >
                    Connect to Wi-Fi
                  </button>
                </div>
                <p className="mt-3 text-[9px] text-white/60">{draft.terms}</p>
              </>
            ) : (
              <>
                <div className="text-base font-semibold">{payMethod.label}</div>
                <p className="mt-1 text-[11px] text-white/75">
                  Temporary internet for {draft.trial_minutes} minutes to pay, then unlock with your
                  voucher code.
                </p>
                {payMethod.infoTitle ? (
                  <p className="mt-3 text-[11px] font-medium text-white/90">{payMethod.infoTitle}</p>
                ) : null}
                {qrUrl ? (
                  <img
                    src={qrUrl}
                    alt="MMQR payment QR"
                    className="mx-auto mt-3 max-h-40 rounded-xl bg-white p-2"
                  />
                ) : (
                  <div className="mx-auto mt-3 flex h-36 max-w-[180px] items-center justify-center rounded-xl border border-dashed border-white/30 bg-white/10 px-3 text-[10px] text-white/65">
                    Attach your MMQR / wallet QR photo on the pay method card to preview it here.
                  </div>
                )}
                {payMethod.infoBody ? (
                  <p className="mt-3 max-h-24 overflow-auto whitespace-pre-wrap text-left text-[10px] text-white/80">
                    {payMethod.infoBody}
                  </p>
                ) : null}
                {payMethod.copyValue ? (
                  <div className="mt-2 rounded-xl border border-white/25 bg-white/10 px-3 py-2 text-[11px] text-white/80">
                    {payMethod.copyValue}
                  </div>
                ) : null}
                <button
                  type="button"
                  className="mt-3 w-full rounded-xl py-2 text-xs font-semibold text-slate-900"
                  style={{
                    background: `linear-gradient(180deg, #fff, ${safeCssHex(payMethod.accentHex, draft.primary_hex)})`,
                  }}
                >
                  Connect
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
