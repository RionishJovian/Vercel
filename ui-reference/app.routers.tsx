import { createFileRoute, useSearch } from "@tanstack/react-router";
import { DeviceLimitCard } from "@/components/DeviceLimitCard";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import {
  listRouters,
  saveRouter,
  deleteRouter,
  testRouter,
  routersStatus,
  listRouterTests,
  rebootRouter,
  reactivateRouterWithKey,
} from "@/lib/routers.functions";
import { listSites } from "@/lib/sites.functions";
import { listConnectors } from "@/lib/connectors.functions";
import { getMe } from "@/lib/auth.functions";
import { ROUTER_OPS_AUDIT_QUERY_KEY, ROUTER_SAVE_AUDIT_QUERY_KEY } from "@/lib/audit.functions";
import { meHasFeature } from "@/lib/operator-features";
import { DelayedFallback, SkeletonList } from "@/components/ui/skeleton";
import { ClientOnly } from "@tanstack/react-router";
import { lazy, Suspense } from "react";
import { RouterActionsDrawer } from "@/components/RouterActionsDrawer";
import { MagicHubPasteDialog } from "@/components/MagicHubPasteDialog";
import { MagicHubSparkles } from "@/components/MagicHubSparkle";
import { TypedConfirmDialog } from "@/components/TypedConfirmDialog";
import { hubHostFromName, isHubMode } from "@/lib/connection-mode";

const RouterActionsDrawerLazy = lazy(() => import("@/components/RouterActionsDrawer"));
import {
  connectionMethodsForRole,
  isStaffRoles,
  type ConnMethodId,
} from "@/lib/connection-methods";
import type { CloudProvisionResult } from "@/lib/cloud-router.functions";
import { useSelectedSite, setSelectedSite } from "@/hooks/useSelectedSite";
import { useT } from "@/lib/i18n";
import { VerifyIdentityButton } from "@/components/VerifyIdentityButton";
import { toErrorMessage } from "@/lib/error-message";
import { PlatformActiveSitesPanel } from "@/components/PlatformActiveSitesPanel";

/** "remote" = legacy edit of an existing direct row only — not offered in the chooser. */
type ConnMethod = ConnMethodId | "remote";

export const Route = createFileRoute("/_authenticated/app/routers")({
  head: () => ({
    meta: [
      { title: "Routers — MikroTik Hotspot Admin" },
      {
        name: "description",
        content:
          "Add, edit, and test remote MikroTik router connections used by the hotspot admin app.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  validateSearch: (raw: Record<string, unknown>): { method?: ConnMethod } => {
    const m = raw.method;
    if (m === "connector" || m === "hub") return { method: m };
    // Legacy Public IP / DDNS deep-links → Magic Hub (Public IP was removed).
    if (m === "remote" || m === "cloud") return { method: "hub" };
    if (m === "wireguard") return { method: "hub" };
    return {};
  },
  component: RoutersPage,
});

type FormState = {
  id?: string;
  name: string;
  host: string;
  port: number;
  username: string;
  password: string;
  useTls: boolean;
  allowInsecureTls: boolean;
  insecureTlsReason: string;
  isDefault: boolean;
  siteId: string;
  connectorId: string;
};

const EMPTY: FormState = {
  name: "",
  host: "",
  port: 443,
  username: "admin",
  password: "",
  useTls: true,
  allowInsecureTls: false,
  insecureTlsReason: "",
  isDefault: false,
  siteId: "",
  connectorId: "",
};

function RoutersPage() {
  const t = useT();
  const search = useSearch({ from: "/_authenticated/app/routers" });
  const qc = useQueryClient();
  const fetchList = useServerFn(listRouters);
  const save = useServerFn(saveRouter);
  const del = useServerFn(deleteRouter);
  const test = useServerFn(testRouter);
  const fetchStatus = useServerFn(routersStatus);
  const fetchTests = useServerFn(listRouterTests);
  const fetchSites = useServerFn(listSites);
  const fetchConnectors = useServerFn(listConnectors);
  const fetchMe = useServerFn(getMe);

  const list = useQuery({ queryKey: ["routers"], queryFn: () => fetchList() });
  const me = useQuery({ queryKey: ["me"], queryFn: () => fetchMe(), staleTime: 5 * 60_000 });
  const privileged = meHasFeature(me.data, "reboot");
  const canConfigureRouter = meHasFeature(me.data, "router_config");
  const isStaff = isStaffRoles(me.data?.roles, me.data?.isPlatformAdmin);
  const methodOptions = connectionMethodsForRole(isStaff);

  const sites = useQuery({ queryKey: ["sites"], queryFn: () => fetchSites(), staleTime: 60_000 });
  const connectors = useQuery({
    queryKey: ["connectors"],
    queryFn: () => fetchConnectors(),
    staleTime: 30_000,
  });
  const status = useQuery({
    queryKey: ["routers-status"],
    queryFn: () => fetchStatus(),
    enabled: (list.data?.length ?? 0) > 0,
    refetchOnWindowFocus: false,
    // Keep Online / Offline fresh for cloud-connected routers.
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    staleTime: 30_000,
  });
  const tests = useQuery({
    queryKey: ["router-tests"],
    queryFn: () => fetchTests(),
    refetchOnWindowFocus: false,
  });
  const statusById = useMemo(() => {
    const m = new Map<string, { online: boolean; mode: string; error?: string }>();
    for (const r of status.data?.routers ?? []) m.set(r.id, r);
    return m;
  }, [status.data]);
  const siteName = useMemo(() => {
    const m = new Map<string, string>();
    for (const s of sites.data ?? []) m.set(s.id, s.name);
    return m;
  }, [sites.data]);

  const [form, setForm] = useState<FormState>(EMPTY);
  // Magic Hub (Cloud Remote) · Local Connector. Public IP / DDNS removed.
  const [method, setMethod] = useState<ConnMethod>(search.method ?? "hub");
  useEffect(() => {
    if (search.method) setMethod(search.method);
  }, [search.method]);
  // Magic Hub asks for no address: the board dials out and the cloud uses the
  // WireGuard peer. Keep the stored label column filled from the router name so
  // the hidden field is never empty (an explicit label on an existing board is
  // preserved).
  // NOTE: must stay AFTER the `form` / `method` state declarations above — the
  // dependency array is evaluated during render, so referencing them earlier is
  // a temporal-dead-zone crash ("Cannot access 'X' before initialization").
  useEffect(() => {
    if (method !== "hub") return;
    setForm((f) => {
      const nextHost = f.host.trim() ? f.host : hubHostFromName(f.name.trim() || "router");
      if (f.host === nextHost && f.port === 443) return f;
      return { ...f, host: nextHost, port: 443 };
    });
  }, [method, form.name]);
  const [hubArtifacts, setHubArtifacts] = useState<Record<string, CloudProvisionResult>>({});
  /** Page-level paste dialog — opens even if the router card is filtered out of the list. */
  const [pagePaste, setPagePaste] = useState<{
    routerId: string;
    script: string;
    rollbackScript: string;
  } | null>(null);
  const [testResult, setTestResult] = useState<
    Record<string, Awaited<ReturnType<typeof testRouter>>>
  >({});
  const [query, setQuery] = useState("");
  const [manualStatusRefresh, setManualStatusRefresh] = useState(false);
  const [actionsOpen, setActionsOpen] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null);
  const { site: selectedSite } = useSelectedSite();

  const filtered = useMemo(() => {
    const base = (list.data ?? []).filter((r) =>
      selectedSite ? r.site_id === selectedSite.id : true,
    );
    const q = query.trim().toLowerCase();
    if (!q) return base;
    return base.filter((r) =>
      [r.name, r.host, r.username].some((v) => (v ?? "").toString().toLowerCase().includes(q)),
    );
  }, [list.data, query, selectedSite]);

  const blankForm = (): FormState => ({
    ...EMPTY,
    siteId: selectedSite?.id ?? "",
  });

  useEffect(() => {
    if (form.id) return;
    const next = selectedSite?.id ?? "";
    setForm((f) => (f.siteId === next ? f : { ...f, siteId: next }));
  }, [selectedSite?.id, form.id]);

  const saveMut = useMutation({
    mutationFn: (data: FormState) =>
      save({
        data: {
          ...data,
          siteId: data.siteId ? data.siteId : null,
          connectorId: method === "connector" && data.connectorId ? data.connectorId : null,
          connectionMethod: method === "remote" ? undefined : method,
        },
      }),
    onSuccess: async (res, vars) => {
      const newId =
        vars.id ?? (res && typeof res === "object" && "id" in res ? String(res.id) : null);
      const hub =
        res && typeof res === "object" && "hub" in res
          ? (res.hub as CloudProvisionResult | null)
          : null;
      const hubError =
        res && typeof res === "object" && "hubError" in res
          ? (res.hubError as string | undefined)
          : undefined;
      if (hub && newId) setHubArtifacts((s) => ({ ...s, [newId]: hub }));
      if (hub?.routerScript && newId) {
        setPagePaste({
          routerId: newId,
          script: hub.routerScript,
          rollbackScript: hub.rollbackScript,
        });
        // Site switcher must not hide the new Unassigned card (paste lived inside CloudPanel).
        if (selectedSite && !(vars.siteId && vars.siteId === selectedSite.id)) {
          setSelectedSite(null);
        }
      }
      // Invalidate AND refetch before presenting completion, so the UI shows
      // exactly what the database holds after the credential save.
      await qc.invalidateQueries({ queryKey: ["routers"] });
      await qc.refetchQueries({ queryKey: ["routers"] });
      void qc.invalidateQueries({ queryKey: ["routers-status"] });
      void qc.invalidateQueries({ queryKey: ROUTER_SAVE_AUDIT_QUERY_KEY });
      void qc.invalidateQueries({ queryKey: ROUTER_OPS_AUDIT_QUERY_KEY });
      toast.success(
        vars.id
          ? "Router updated"
          : method === "hub" && hub?.routerScript
            ? "Router added — paste window opened (Magic Hub, shown once)"
            : method === "hub" && hubError
              ? "Router saved — Magic Hub not connected yet"
              : "Router added",
      );
      if (hubError) {
        toast.error(toErrorMessage(hubError), { duration: 14_000 });
      } else if (method === "hub" && !vars.id && newId && !hub?.routerScript) {
        toast.error(
          "Router saved, but Magic Hub did not return a paste script. Tap Connect via Hub on the card.",
          { duration: 10_000 },
        );
      }
      if (method === "hub" && newId) {
        setActionsOpen(newId);
      }
      // After a hub paste, start the next Add form unassigned so the site filter
      // cannot hide the card we just created.
      setForm(hub?.routerScript ? { ...EMPTY, siteId: "" } : blankForm());
      setMethod("hub");
    },
    onError: (e: Error) => toast.error(toErrorMessage(e, "Save failed")),
  });

  const delMut = useMutation({
    mutationFn: (input: { id: string; confirmation?: string }) => del({ data: input }),
    onSuccess: () => {
      toast.success("Router deleted");
      setDeleteTarget(null);
      qc.invalidateQueries({ queryKey: ["routers"] });
      qc.invalidateQueries({ queryKey: ["routers-status"] });
      void qc.invalidateQueries({ queryKey: ROUTER_SAVE_AUDIT_QUERY_KEY });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e, "Delete failed")),
  });

  const rebootFn = useServerFn(rebootRouter);
  const reactivateFn = useServerFn(reactivateRouterWithKey);
  const reactivateMut = useMutation({
    mutationFn: (routerId: string) => reactivateFn({ data: { routerId } }),
    onSuccess: async (result) => {
      await qc.invalidateQueries({ queryKey: ["routers"] });
      await qc.invalidateQueries({ queryKey: ["profile"] });
      toast.success(`Router unlocked until ${new Date(result.expires_at).toLocaleString()}.`);
    },
    onError: (e: Error) => toast.error(toErrorMessage(e)),
  });
  const rebootMut = useMutation({
    mutationFn: (id: string) => rebootFn({ data: { id } }),
    onSuccess: () => {
      toast.success("Reboot command sent — the router will be back in ~1 minute");
      void qc.invalidateQueries({ queryKey: ROUTER_SAVE_AUDIT_QUERY_KEY });
    },
    onError: (e: Error) => toast.error(toErrorMessage(e, "Reboot failed")),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => test({ data: { id } }),
    onSuccess: (r, id) => {
      setTestResult((s) => ({ ...s, [id]: r }));
      qc.invalidateQueries({ queryKey: ["router-tests"] });
      void qc.invalidateQueries({ queryKey: ROUTER_SAVE_AUDIT_QUERY_KEY });
      if (r.ok) toast.success("Router reachable — REST OK");
      else toast.error(toErrorMessage(("error" in r ? r.error : null) ?? "Test failed"));
    },
    onError: (e: Error) => toast.error(toErrorMessage(e, "Test failed")),
  });

  const testAllMut = useMutation({
    mutationFn: async () => {
      const rows = filtered;
      const out: Record<string, Awaited<ReturnType<typeof testRouter>>> = {};
      for (const r of rows) out[r.id] = await test({ data: { id: r.id } });
      return out;
    },
    onSuccess: (all) => {
      setTestResult((s) => ({ ...s, ...all }));
      qc.invalidateQueries({ queryKey: ["router-tests"] });
      qc.invalidateQueries({ queryKey: ["routers-status"] });
      void qc.invalidateQueries({ queryKey: ROUTER_SAVE_AUDIT_QUERY_KEY });
      const ok = Object.values(all).filter((r) => r.ok).length;
      toast.success(`Tested ${Object.keys(all).length} routers — ${ok} reachable`);
    },
    onError: (e: Error) => toast.error(toErrorMessage(e, "Bulk test failed")),
  });

  return (
    <div className="space-y-6">
      <h1 className="sr-only">{t.label("Router Management")}</h1>
      <DeviceLimitCard kind="routers" />

      {(me.data?.isPlatformAdmin || me.data?.roles.includes("primary")) && (
        <PlatformActiveSitesPanel />
      )}

      <div className="grid gap-8 lg:grid-cols-2">
        <section className="panel p-5" id="add-router">
          <h2 className="text-lg font-semibold">
            {form.id ? t.label("Edit router") : t.label("Add a router")}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {isStaff
              ? t.copy(
                  "Magic Hub is Cloud Remote — the main way to manage a board from the app (Starlink included). After Add router, a paste-script window opens. Local Connector is a PC on site. Credentials are encrypted at rest with AES-256-GCM.",
                )
              : t.copy(
                  "Magic Hub is Cloud Remote — the main way to manage a board from the app (Starlink included). After Add router, a paste-script window opens. Local Connector is a PC on site. Credentials are encrypted at rest with AES-256-GCM.",
                )}
          </p>
          <form
            className="mt-4 space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (saveMut.isPending) return;
              const host =
                method === "hub" && !form.host.trim() ? hubHostFromName(form.name) : form.host;
              saveMut.mutate({ ...form, host });
            }}
          >
            <Field label="Router name">
              <input
                required
                className="input"
                value={form.name}
                placeholder="Hotspot RouterBoard"
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </Field>
            {method !== "hub" && (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <Field
                  label={method === "connector" ? "LAN IP" : "Public host / DDNS"}
                  className="sm:col-span-2"
                >
                  <input
                    required
                    className="input"
                    value={form.host}
                    placeholder={
                      method === "connector" ? "192.168.88.1" : "abc123.sn.mynetname.net"
                    }
                    onChange={(e) => setForm({ ...form, host: e.target.value })}
                  />
                </Field>
                <Field label="Port">
                  <input
                    required
                    type="number"
                    className="input"
                    value={form.port}
                    onChange={(e) => setForm({ ...form, port: Number(e.target.value) })}
                  />
                </Field>
              </div>
            )}
            {method === "hub" && (
              <p className="rounded-md border border-border bg-muted/40 p-2 text-[11px] text-muted-foreground">
                {t.copy(
                  "Nothing to fill in — the board dials out to Magic Hub itself, so no address or port is needed. A name label is generated from the router name automatically.",
                )}
              </p>
            )}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="Username">
                <input
                  required
                  className="input"
                  value={form.username}
                  onChange={(e) => setForm({ ...form, username: e.target.value })}
                />
              </Field>
              <Field label={form.id ? "Password (leave blank to keep)" : "Password"}>
                <input
                  type="password"
                  className="input"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                />
              </Field>
            </div>
            <Field label="Site (optional)">
              <select
                className="input"
                value={form.siteId}
                onChange={(e) => setForm({ ...form, siteId: e.target.value })}
              >
                <option value="">No site</option>
                {(sites.data ?? []).map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </Field>
            <fieldset>
              <legend className="mb-1 block text-xs text-muted-foreground">
                {t.label("Connection method")}
              </legend>
              <div
                className={`grid gap-2 ${
                  methodOptions.length >= 3 ? "sm:grid-cols-3" : "sm:grid-cols-2"
                }`}
              >
                {methodOptions.map((opt) => {
                  const selected = method === opt.id;
                  return (
                    <button
                      key={opt.id}
                      type="button"
                      aria-pressed={selected}
                      onClick={() => {
                        setMethod(opt.id);
                        if (opt.id === "hub") {
                          setForm((f) => ({
                            ...f,
                            connectorId: "",
                            useTls: true,
                            port: f.port === 80 ? 443 : f.port,
                          }));
                        }
                      }}
                      className={`rounded-xl border p-3 text-left text-xs transition ${
                        opt.featured ? "magic-hub-method " : ""
                      }${
                        selected
                          ? opt.featured
                            ? "is-selected border-primary/70 bg-primary/15 text-primary"
                            : "border-primary/60 bg-primary/10 text-primary"
                          : "border-[color:var(--glass-border)] bg-white/5 hover:border-primary/40"
                      }`}
                    >
                      {opt.featured ? <MagicHubSparkles /> : null}
                      <div
                        className={`text-sm font-semibold ${opt.featured ? "relative z-[1]" : ""}`}
                      >
                        {t.label(opt.title)}
                      </div>
                      <p
                        className={`mt-1 leading-snug text-muted-foreground ${
                          opt.featured ? "relative z-[1]" : ""
                        }`}
                      >
                        {t.copy(opt.body)}
                      </p>
                    </button>
                  );
                })}
              </div>
            </fieldset>

            {method === "connector" && (
              <Field label="Paired connector">
                <select
                  className="input"
                  value={form.connectorId}
                  onChange={(e) => setForm({ ...form, connectorId: e.target.value })}
                >
                  <option value="">{t.label("Select a connector")}</option>
                  {(connectors.data ?? []).map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                      {c.online ? "" : " (offline)"}
                    </option>
                  ))}
                </select>
              </Field>
            )}
            {method !== "hub" && (
              <div className="flex flex-wrap gap-4 text-xs">
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={form.useTls}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        useTls: e.target.checked,
                        port: e.target.checked
                          ? form.port === 80
                            ? 443
                            : form.port
                          : form.port === 443
                            ? 80
                            : form.port,
                      })
                    }
                  />
                  Use HTTPS (TLS)
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={form.allowInsecureTls}
                    disabled={!form.useTls}
                    onChange={(e) => setForm({ ...form, allowInsecureTls: e.target.checked })}
                  />
                  Allow self-signed certificate (not verified)
                </label>
                {form.allowInsecureTls ? (
                  <div className="col-span-full space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs">
                    <p className="font-medium text-amber-200">
                      Warning: certificate checks are disabled for this router only. Traffic can be
                      intercepted on an untrusted network. Use a verified certificate whenever
                      possible.
                    </p>
                    <input
                      className="w-full rounded-md border border-white/15 bg-black/20 px-2 py-1"
                      placeholder="Reason for the exception (recorded in the audit trail)"
                      maxLength={300}
                      value={form.insecureTlsReason}
                      onChange={(e) => setForm({ ...form, insecureTlsReason: e.target.value })}
                    />
                  </div>
                ) : null}
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={form.isDefault}
                    onChange={(e) => setForm({ ...form, isDefault: e.target.checked })}
                  />
                  Default router
                </label>
              </div>
            )}
            {method === "hub" && (
              <label className="flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={form.isDefault}
                  onChange={(e) => setForm({ ...form, isDefault: e.target.checked })}
                />
                Default router
              </label>
            )}
            <p className="rounded-md border border-border bg-muted/40 p-2 text-xs text-muted-foreground">
              {method === "hub"
                ? t.copy(
                    "Magic Hub is Cloud Remote. Tap Add router (not only the Magic Hub card). A window then shows the paste script — it is not on the Scripts page.",
                  )
                : method === "connector"
                  ? t.copy(
                      "Local Connector reaches the LAN IP through a paired agent. Pair one under Connectors if the list is empty.",
                    )
                  : t.copy(
                      "Legacy direct connection — prefer Magic Hub or Local Connector for new boards.",
                    )}
            </p>

            {saveMut.error && (
              <p className="text-xs text-danger">{(saveMut.error as Error).message}</p>
            )}
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={saveMut.isPending}
                aria-busy={saveMut.isPending || undefined}
                className="inline-flex items-center justify-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-all duration-200 disabled:opacity-60"
              >
                {saveMut.isPending && <ButtonSpinner />}
                {form.id
                  ? saveMut.isPending
                    ? "Saving…"
                    : "Save changes"
                  : saveMut.isPending
                    ? "Adding…"
                    : "Add router"}
              </button>
              {form.id && (
                <button
                  type="button"
                  onClick={() => {
                    setForm(blankForm());
                    setMethod(search.method ?? "hub");
                  }}
                  className="rounded-md border border-border px-3 py-2 text-sm"
                >
                  Cancel
                </button>
              )}
            </div>
          </form>
        </section>

        <section className="panel p-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-lg font-semibold">{t.label("Your routers")}</h2>
            <div className="text-xs text-muted-foreground">
              {filtered.length} shown
              {selectedSite ? ` · Site: ${selectedSite.name}` : ""}
              {query && filtered.length !== (list.data?.length ?? 0)
                ? ` · filtered from ${list.data?.length ?? 0}`
                : ""}
            </div>
          </div>

          {(list.data?.length ?? 0) > 0 && (
            <div className="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-[color:var(--glass-border)] bg-white/5 p-3 text-xs">
              <span className="chip">
                {status.data ? `${status.data.online} online` : "Checking…"}
              </span>
              <span className="chip">{status.data ? `${status.data.offline} offline` : "—"}</span>
              {status.data?.observedAt && (
                <span className="text-[11px] text-muted-foreground">
                  Checked {new Date(status.data.observedAt).toLocaleTimeString()}
                </span>
              )}
              <button
                className="ml-auto min-h-[36px] shrink-0 whitespace-nowrap rounded-full border border-[color:var(--glass-border)] bg-white/5 px-3 py-1 transition-[border-color,color,opacity,background-color] hover:border-primary/50 hover:bg-primary/10 hover:text-primary disabled:opacity-60"
                onClick={() => {
                  setManualStatusRefresh(true);
                  void status.refetch().finally(() => setManualStatusRefresh(false));
                }}
                disabled={manualStatusRefresh}
              >
                {manualStatusRefresh ? "Refreshing…" : "Refresh status"}
              </button>
              <button
                className="min-h-[36px] rounded-full border border-[color:var(--glass-border)] bg-white/5 px-3 py-1 transition hover:border-primary/50 hover:bg-primary/10 hover:text-primary disabled:opacity-60"
                onClick={() => testAllMut.mutate()}
                disabled={testAllMut.isPending || filtered.length === 0}
              >
                {testAllMut.isPending ? "Testing all…" : "Test all"}
              </button>
            </div>
          )}

          {(list.data?.length ?? 0) > 0 && (
            <input
              className="input mt-3 min-h-[44px] w-full"
              placeholder="Search by name, host, or user"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              aria-label="Filter routers"
            />
          )}
          {list.isLoading && (
            <DelayedFallback
              loading
              label="Loading routers"
              fallback={<SkeletonList rows={4} className="mt-3" />}
            />
          )}
          {list.data?.length === 0 && (
            <p className="mt-2 text-sm text-muted-foreground">
              {isStaff
                ? t.copy(
                    "No routers yet. Add one with Magic Hub for Starlink or CGNAT, or Quick Setup if the WAN has a public IP.",
                  )
                : t.copy("No routers yet. Add one with Magic Hub for Starlink or CGNAT.")}
            </p>
          )}

          {list.data && list.data.length > 0 && filtered.length === 0 && (
            <p className="mt-3 text-sm text-muted-foreground">
              {query.trim()
                ? `No routers match "${query}".`
                : selectedSite
                  ? `No routers on site “${selectedSite.name}”. Clear the site filter or assign a router on Sites.`
                  : "No routers match the current filter."}
            </p>
          )}
          <ul className="mt-3 space-y-3">
            {filtered.map((r) => {
              const st = statusById.get(r.id);
              const lastTest = tests.data?.[r.id];
              const mode = isHubMode(r.connection_mode)
                ? "Magic Hub"
                : r.connector_id
                  ? "Local Connector"
                  : "Direct (legacy)";
              const routerLocked = r.router_unlock?.locked === true;
              return (
                <li key={r.id} className="airai-row animate-rise">
                  <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2 text-sm font-semibold">
                        <span className="relative inline-flex h-2 w-2">
                          {st?.online !== false && (
                            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-60" />
                          )}
                          <span
                            className={`relative inline-flex h-2 w-2 rounded-full ${
                              st === undefined
                                ? "bg-muted-foreground"
                                : st.online
                                  ? "bg-success shadow-[0_0_10px_var(--color-success)]"
                                  : "bg-danger shadow-[0_0_10px_var(--color-danger)]"
                            }`}
                          />
                        </span>
                        <span className="min-w-0 break-words">{r.name}</span>
                        <span className="chip">{mode}</span>
                        {r.site_id && siteName.get(r.site_id) && (
                          <span className="chip max-w-[160px] truncate">
                            {siteName.get(r.site_id)}
                          </span>
                        )}
                        {r.is_default && <span className="chip">default</span>}
                        {routerLocked && (
                          <span className="chip border-amber-400/50 text-amber-200">LOCKED</span>
                        )}
                      </div>
                      {(lastTest || st?.error) && (
                        <div className="mt-1 text-[11px] text-muted-foreground">
                          {lastTest && (
                            <>
                              Last Router Access (Ping): {lastTest.success ? "passed" : "failed"} ·{" "}
                              {new Date(lastTest.at).toLocaleString()}
                            </>
                          )}
                          {st?.error && (
                            <div className="text-danger break-words">
                              {toErrorMessage(st.error)}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                    <div className="grid w-full grid-cols-2 gap-1.5 text-xs sm:flex sm:w-auto sm:shrink-0 sm:flex-wrap sm:items-center sm:gap-2">
                      {routerLocked && (
                        <button
                          className="col-span-2 inline-flex min-h-11 items-center justify-center rounded-full border border-amber-400/50 bg-amber-400/10 px-3 py-1 text-amber-100 sm:min-h-9"
                          onClick={() => reactivateMut.mutate(r.id)}
                          disabled={reactivateMut.isPending}
                        >
                          {reactivateMut.isPending
                            ? "Unlocking…"
                            : "Unlock router · use Router key"}
                        </button>
                      )}
                      <div className="col-span-2 sm:col-span-1 sm:contents">
                        <VerifyIdentityButton routerId={r.id} name={r.name} locked={routerLocked} />
                      </div>
                      <button
                        className="min-h-11 rounded-full border border-primary/40 bg-primary/10 px-3 py-1 font-medium text-primary transition hover:border-primary/60 hover:bg-primary/20 sm:min-h-9"
                        onClick={() => setActionsOpen(r.id)}
                        aria-haspopup="dialog"
                      >
                        Actions
                      </button>
                      <button
                        className="min-h-11 rounded-full border border-[color:var(--glass-border)] bg-white/5 px-3 py-1 transition hover:border-primary/50 hover:bg-primary/10 hover:text-primary sm:min-h-9"
                        onClick={() => {
                          if (routerLocked) return;
                          setMethod(
                            isHubMode(r.connection_mode)
                              ? "hub"
                              : r.connector_id
                                ? "connector"
                                : "remote",
                          );
                          setForm({
                            id: r.id,
                            name: r.name,
                            host: r.host,
                            port: r.port,
                            username: r.username,
                            password: "",
                            useTls: r.use_tls,
                            allowInsecureTls: r.allow_insecure_tls,
                            insecureTlsReason: r.allow_insecure_tls
                              ? "Existing self-signed certificate exception"
                              : "",
                            isDefault: r.is_default,
                            siteId: r.site_id ?? "",
                            connectorId: r.connector_id ?? "",
                          });
                        }}
                        disabled={routerLocked}
                      >
                        Edit
                      </button>
                    </div>
                  </div>
                  {testResult[r.id] && <TestReport result={testResult[r.id]} />}
                  {routerLocked ? (
                    <p className="mt-3 rounded-md border border-amber-400/40 bg-amber-400/10 p-3 text-xs text-amber-100">
                      Router key expired. Router actions, Hub controls, hotspot setup and telemetry
                      are locked until you use a new Router key.
                    </p>
                  ) : null}
                  <ClientOnly
                    fallback={<div />}
                  >
                    <Suspense fallback={<div />}>
                      <RouterActionsDrawerLazy
                        open={actionsOpen === r.id}
                        onOpenChange={(open) => setActionsOpen(open ? r.id : null)}
                        router={r}
                        status={st ?? null}
                        privileged={privileged}
                        canConfigureRouter={canConfigureRouter}
                        hubArtifacts={hubArtifacts[r.id] ?? null}
                        setPagePaste={(art) => {
                          if (art) {
                            setPagePaste({
                              routerId: r.id,
                              script: art.routerScript,
                              rollbackScript: art.rollbackScript ?? "",
                            });
                          }
                        }}
                        onEdit={() => {
                          setMethod(
                            isHubMode(r.connection_mode)
                              ? "hub"
                              : r.connector_id
                                ? "connector"
                                : "remote",
                          );
                          setForm({
                            id: r.id,
                            name: r.name,
                            host: r.host,
                            port: r.port,
                            username: r.username,
                            password: "",
                            useTls: r.use_tls,
                            allowInsecureTls: r.allow_insecure_tls,
                            insecureTlsReason: r.allow_insecure_tls
                              ? "Existing self-signed certificate exception"
                              : "",
                            isDefault: r.is_default,
                            siteId: r.site_id ?? "",
                        connectorId: r.connector_id ?? "",
                      });
                      window.scrollTo({ top: 0, behavior: "smooth" });
                    }}
                    onDelete={(id) => {
                      if (!id) {
                        setDeleteTarget(null);
                        return;
                      }
                      const target = list.data?.find((x) => x.id === id);
                      if (target) setDeleteTarget({ id: target.id, name: target.name });
                    }}
                    deleteTarget={deleteTarget?.id ?? null}
                    delMut={{
                      isPending: delMut.isPending,
                      mutate: (vars) => delMut.mutate(vars),
                    }}
                  />
                    </Suspense>
                  </ClientOnly>
                </li>
              );
            })}
          </ul>
        </section>
      </div>
      <MagicHubPasteDialog
        open={Boolean(pagePaste?.script)}
        onOpenChange={(open) => {
          if (!open) setPagePaste(null);
        }}
        script={pagePaste?.script}
        rollbackScript={pagePaste?.rollbackScript ?? ""}
      />
    </div>
  );
}

function Field({
  label,
  children,
  className = "",
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <label className={`block ${className}`}>
      <span className="mb-1 block text-xs text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

type TestResult = Awaited<ReturnType<typeof testRouter>>;

function TestReport({ result }: { result: TestResult }) {
  const icon = (s: string) => (s === "ok" ? "✓" : s === "fail" ? "✕" : s === "warn" ? "⚠" : "–");
  const color = (s: string) =>
    s === "ok"
      ? "text-success"
      : s === "fail"
        ? "text-danger"
        : s === "warn"
          ? "text-warning"
          : "text-muted-foreground";
  return (
    <div className="mt-3 space-y-2 rounded-md border border-border bg-muted/30 p-3 text-xs">
      <div className="flex items-center justify-between">
        <span className={`font-medium ${result.ok ? "text-success" : "text-danger"}`}>
          {result.ok ? "Reachable — REST OK" : "Test failed"}
        </span>
        <code className="text-[10px] text-muted-foreground">{result.endpoint}</code>
      </div>
      <ol className="space-y-1">
        {result.steps.map((s, i) => (
          <li key={i} className="flex items-start gap-2">
            <span className={`w-4 shrink-0 font-mono ${color(s.status)}`}>{icon(s.status)}</span>
            <div className="flex-1">
              <div>{s.name}</div>
              {s.detail && (
                <div className="text-[11px] text-muted-foreground break-all">{s.detail}</div>
              )}
            </div>
          </li>
        ))}
      </ol>
      {result.remediation.length > 0 && (
        <div className="mt-2 border-t border-border pt-2">
          <div className="mb-1 font-medium text-warning">Fix checklist</div>
          <ol className="list-decimal space-y-1 pl-4">
            {result.remediation.map((r, i) => (
              <li key={i} className="text-muted-foreground [&_code]:font-mono">
                {r}
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  );
}
