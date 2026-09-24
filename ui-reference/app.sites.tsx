import { createFileRoute } from "@tanstack/react-router";
import { DeviceLimitCard } from "@/components/DeviceLimitCard";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useServerFn } from "@tanstack/react-start";
import { Suspense, lazy, useState } from "react";
import { ClientOnly } from "@tanstack/react-router";
import { toast } from "sonner";
import {
  listSites,
  saveSite,
  deleteSite,
  assignRouterToSite,
  listRoutersWithSite,
} from "@/lib/sites.functions";
import { getMe } from "@/lib/auth.functions";
import { setSelectedSite, useSelectedSite } from "@/hooks/useSelectedSite";
import { listManagedDevices } from "@/lib/inventory.functions";
import { routersStatus } from "@/lib/routers.functions";
import { MagicGoLiveStrip } from "@/components/MagicGoLiveStrip";
import { TypedConfirmDialog } from "@/components/TypedConfirmDialog";
import { DELETE_SITE_PHRASE, siteNeedsTypedDeletion } from "@/lib/device-removal";
import { asCoord, roundCoord } from "@/lib/sites-coords";
import { toErrorMessage } from "@/lib/error-message";
import { useT } from "@/lib/i18n";
import { DelayedFallback, SkeletonList } from "@/components/ui/skeleton";
import { PlatformActiveSitesPanel } from "@/components/PlatformActiveSitesPanel";

const SitesMap = lazy(() => import("@/components/SitesMap"));

export const Route = createFileRoute("/_authenticated/app/sites")({
  head: () => ({
    meta: [
      { title: "Sites — MikroTik Hotspot Admin" },
      {
        name: "description",
        content:
          "Group routers by physical location — branch, hotel, site — and filter the dashboard by site.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: SitesPage,
});

function SitesPage() {
  const qc = useQueryClient();
  const t = useT();
  const fetchMe = useServerFn(getMe);
  const fetchSites = useServerFn(listSites);
  const fetchRouters = useServerFn(listRoutersWithSite);
  const save = useServerFn(saveSite);
  const remove = useServerFn(deleteSite);
  const assign = useServerFn(assignRouterToSite);

  const me = useQuery({ queryKey: ["me"], queryFn: () => fetchMe() });
  const sites = useQuery({ queryKey: ["sites"], queryFn: () => fetchSites() });
  const routers = useQuery({
    queryKey: ["routers-with-site"],
    queryFn: () => fetchRouters(),
  });
  // Map pins only need online/offline — not the deep Fleet probe (~12 REST
  // calls + syslog counts per router). Share the light routers-status cache.
  const fetchStatus = useServerFn(routersStatus);
  const fetchDevices = useServerFn(listManagedDevices);
  const status = useQuery({
    queryKey: ["routers-status"],
    queryFn: () => fetchStatus(),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    staleTime: 30_000,
  });
  const devices = useQuery({
    queryKey: ["managed-devices"],
    queryFn: () => fetchDevices(),
    staleTime: 60_000,
  });
  const { site: selected } = useSelectedSite();

  const canEdit = !(me.data?.roles?.includes("read_only") || me.data?.roles?.includes("expired"));

  const saveMutation = useMutation({
    mutationFn: (input: {
      id?: string;
      name: string;
      location: string | null;
      timezone: string | null;
      notes: string | null;
      latitude?: number | null;
      longitude?: number | null;
    }) => save({ data: input }),
    onSuccess: () => {
      toast.success("Site saved");
      qc.invalidateQueries({ queryKey: ["sites"] });
    },
    onError: (e) => toast.error(toErrorMessage(e, "Could not save site")),
  });

  const deleteMutation = useMutation({
    mutationFn: (input: { id: string; confirmation?: string }) => remove({ data: input }),
    onSuccess: (_r, input) => {
      toast.success("Site deleted");
      setDeleteTarget(null);
      if (selected?.id === input.id) setSelectedSite(null);
      qc.invalidateQueries({ queryKey: ["sites"] });
      qc.invalidateQueries({ queryKey: ["routers-with-site"] });
    },
    onError: (e) => toast.error(toErrorMessage(e, "Could not delete site")),
  });

  const assignMutation = useMutation({
    mutationFn: (v: { routerId: string; siteId: string | null }) => assign({ data: v }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["routers-with-site"] });
    },
    onError: (e) => toast.error(toErrorMessage(e, "Could not update router")),
  });

  const [draft, setDraft] = useState({
    name: "",
    location: "",
    timezone: "",
    notes: "",
    latitude: null as number | null,
    longitude: null as number | null,
  });

  // When set, the next click on the map saves coordinates for that site.
  const [placing, setPlacing] = useState<{ id: string; name: string } | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState({
    name: "",
    location: "",
    timezone: "",
  });
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string;
    name: string;
    bound: number;
  } | null>(null);

  const onlineRouterIds = new Set(
    (status.data?.routers ?? []).filter((r) => r.online).map((r) => r.id),
  );
  const mapSites = (sites.data ?? []).map((s) => {
    const attached = routers.data?.filter((r) => r.site_id === s.id) ?? [];
    return {
      id: s.id,
      name: s.name,
      location: s.location,
      latitude: asCoord(s.latitude),
      longitude: asCoord(s.longitude),
      routers: attached.length,
      online: attached.filter((r) => onlineRouterIds.has(r.id)).length,
    };
  });

  const draftPin =
    !placing && draft.latitude != null && draft.longitude != null
      ? { latitude: draft.latitude, longitude: draft.longitude }
      : null;

  const handleMapPick = (lat: number, lng: number) => {
    if (placing) {
      const site = sites.data?.find((x) => x.id === placing.id);
      if (!site) return;
      saveMutation.mutate({
        id: site.id,
        name: site.name,
        location: site.location,
        timezone: site.timezone,
        notes: site.notes,
        latitude: roundCoord(lat),
        longitude: roundCoord(lng),
      });
      setPlacing(null);
      return;
    }
    setDraft((d) => ({ ...d, latitude: roundCoord(lat), longitude: roundCoord(lng) }));
  };

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">{t.ui("Sites")}</h1>
          <p className="text-sm text-muted-foreground">
            Group routers by physical location. Use the site switcher in the header to scope Live
            users, Vouchers, and Fleet views.
          </p>
        </div>
        {selected && (
          <button
            onClick={() => setSelectedSite(null)}
            className="rounded-md border border-border px-3 py-1.5 text-xs hover:border-primary hover:text-primary"
          >
            Clear site filter ({selected.name})
          </button>
        )}
      </header>
      <DeviceLimitCard kind="sites" />

      {me.data?.isPlatformAdmin && <PlatformActiveSitesPanel />}

      {selected && <MagicGoLiveStrip siteId={selected.id} skipHotspotProbe compact={false} />}

      <section className="glass-panel rounded-2xl p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              Site map
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              {placing
                ? `${t.copy("Click the map to place")} “${placing.name}”.`
                : t.copy(
                    "Green pins have at least one device online, red pins are offline, grey pins have no device linked. Click the map to set coordinates for a new site.",
                  )}
            </p>
          </div>
          {placing && (
            <button
              onClick={() => setPlacing(null)}
              className="rounded-md border border-border px-3 py-1.5 text-xs hover:border-primary hover:text-primary"
            >
              Cancel
            </button>
          )}
        </div>
        <ClientOnly
          fallback={
            <div className="h-[320px] rounded-2xl border border-[color:var(--glass-border)] bg-surface/40" />
          }
        >
          <Suspense
            fallback={
              <div className="h-[320px] rounded-2xl border border-[color:var(--glass-border)] bg-surface/40" />
            }
          >
            <SitesMap
              sites={mapSites}
              draftPin={draftPin}
              {...(canEdit ? { onPick: handleMapPick } : {})}
            />
          </Suspense>
        </ClientOnly>
      </section>

      {canEdit && (
        <section className="glass-panel rounded-2xl p-4">
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Add a new site
          </h2>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <input
              className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
              placeholder="Name (e.g. Main site)"
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
            <input
              className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
              placeholder="Location"
              value={draft.location}
              onChange={(e) => setDraft({ ...draft, location: e.target.value })}
            />
            <input
              className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
              placeholder="Timezone (e.g. Asia/Yangon)"
              value={draft.timezone}
              onChange={(e) => setDraft({ ...draft, timezone: e.target.value })}
            />
            <button
              disabled={!draft.name || saveMutation.isPending}
              onClick={() =>
                saveMutation.mutate(
                  {
                    name: draft.name,
                    location: draft.location || null,
                    timezone: draft.timezone || null,
                    notes: draft.notes || null,
                    latitude: draft.latitude,
                    longitude: draft.longitude,
                  },
                  {
                    onSuccess: () =>
                      setDraft({
                        name: "",
                        location: "",
                        timezone: "",
                        notes: "",
                        latitude: null,
                        longitude: null,
                      }),
                  },
                )
              }
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {saveMutation.isPending ? "Saving…" : "Add site"}
            </button>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {draft.latitude != null && draft.longitude != null
              ? `${t.copy("Coordinates from map")}: ${draft.latitude}, ${draft.longitude}`
              : t.copy("Optional: click the map above to attach coordinates.")}{" "}
            Timezone for this site record (IANA, e.g. Asia/Yangon). Attached routers always sync to
            Asia/Yangon (UTC+06:30) with NTP so the platform and boards stay aligned for Hub linking
            and schedules.
          </p>
        </section>
      )}

      <section className="space-y-3">
        {sites.isLoading && (
          <DelayedFallback loading label="Loading sites" fallback={<SkeletonList rows={3} />} />
        )}
        {sites.data?.length === 0 && (
          <div className="glass-panel rounded-2xl p-8 text-center text-sm text-muted-foreground">
            No sites yet. Add your first location above.
          </div>
        )}
        {sites.data?.map((s) => {
          const attached = routers.data?.filter((r) => r.site_id === s.id) ?? [];
          const boundDevices = (devices.data ?? []).filter(
            (d) => "site_id" in d && d.site_id === s.id,
          ).length;
          const boundCount = attached.length + boundDevices;
          const unattached = routers.data?.filter((r) => !r.site_id) ?? [];
          const isSelected = selected?.id === s.id;
          const lat = asCoord(s.latitude);
          const lng = asCoord(s.longitude);
          return (
            <article
              key={s.id}
              className={`glass-panel rounded-2xl p-4 ${isSelected ? "ring-2 ring-primary" : ""}`}
            >
              <header className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-lg font-semibold">{s.name}</h3>
                  <p className="text-xs text-muted-foreground">
                    {[s.location, s.timezone].filter(Boolean).join(" · ") || "No location set"}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {lat != null && lng != null
                      ? `📍 ${lat}, ${lng}`
                      : t.copy("Not placed on the map yet")}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    onClick={() => setSelectedSite(isSelected ? null : { id: s.id, name: s.name })}
                    className="rounded-md border border-border px-3 py-1.5 text-xs hover:border-primary hover:text-primary"
                  >
                    {isSelected ? "Selected" : "View this site"}
                  </button>
                  {canEdit && (
                    <button
                      onClick={() => {
                        if (editingId === s.id) {
                          setEditingId(null);
                          return;
                        }
                        setEditingId(s.id);
                        setEditDraft({
                          name: s.name,
                          location: s.location ?? "",
                          timezone: s.timezone ?? "",
                        });
                      }}
                      className="rounded-md border border-border px-3 py-1.5 text-xs hover:border-primary hover:text-primary"
                    >
                      {editingId === s.id ? "Cancel edit" : "Edit"}
                    </button>
                  )}
                  {canEdit && (
                    <button
                      onClick={() => setPlacing({ id: s.id, name: s.name })}
                      className="rounded-md border border-border px-3 py-1.5 text-xs hover:border-primary hover:text-primary"
                    >
                      {lat != null ? "Move pin" : "Place on map"}
                    </button>
                  )}
                  {canEdit && (
                    <button
                      onClick={() => {
                        if (siteNeedsTypedDeletion(boundCount)) {
                          setDeleteTarget({ id: s.id, name: s.name, bound: boundCount });
                          return;
                        }
                        if (window.confirm(`Delete site "${s.name}"?`))
                          deleteMutation.mutate({ id: s.id });
                      }}
                      className="rounded-md border border-red-500/40 px-3 py-1.5 text-xs text-red-200 hover:bg-red-500/10"
                    >
                      Delete
                    </button>
                  )}
                </div>
              </header>

              {editingId === s.id && canEdit && (
                <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
                  <input
                    className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
                    value={editDraft.name}
                    onChange={(e) => setEditDraft({ ...editDraft, name: e.target.value })}
                    aria-label="Site name"
                  />
                  <input
                    className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
                    placeholder="Location / address"
                    value={editDraft.location}
                    onChange={(e) => setEditDraft({ ...editDraft, location: e.target.value })}
                    aria-label="Location"
                  />
                  <input
                    className="rounded-md border border-border bg-surface px-3 py-2 text-sm"
                    placeholder="Timezone (Asia/Yangon)"
                    value={editDraft.timezone}
                    onChange={(e) => setEditDraft({ ...editDraft, timezone: e.target.value })}
                    aria-label="Timezone"
                  />
                  <button
                    disabled={!editDraft.name.trim() || saveMutation.isPending}
                    onClick={() =>
                      saveMutation.mutate(
                        {
                          id: s.id,
                          name: editDraft.name.trim(),
                          location: editDraft.location || null,
                          timezone: editDraft.timezone || null,
                          notes: s.notes,
                          latitude: asCoord(s.latitude),
                          longitude: asCoord(s.longitude),
                        },
                        {
                          onSuccess: () => {
                            setEditingId(null);
                            if (selected?.id === s.id) {
                              setSelectedSite({ id: s.id, name: editDraft.name.trim() });
                            }
                          },
                        },
                      )
                    }
                    className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
                  >
                    {saveMutation.isPending ? "Saving…" : "Save changes"}
                  </button>
                </div>
              )}

              <div className="mt-4 space-y-2">
                <div className="text-xs uppercase tracking-wide text-muted-foreground">
                  Attached routers ({attached.length})
                </div>
                {attached.length === 0 && (
                  <p className="text-sm text-muted-foreground">No routers attached.</p>
                )}
                {attached.map((r) => (
                  <div
                    key={r.id}
                    className="flex items-center justify-between rounded-md border border-border/50 bg-surface/50 px-3 py-2 text-sm"
                  >
                    <span>
                      <strong>{r.name}</strong>{" "}
                      <span className="text-muted-foreground">· {r.host}</span>
                    </span>
                    {canEdit && (
                      <button
                        onClick={() =>
                          assignMutation.mutate({
                            routerId: r.id,
                            siteId: null,
                          })
                        }
                        className="text-xs text-muted-foreground hover:text-red-300"
                      >
                        Remove
                      </button>
                    )}
                  </div>
                ))}

                {canEdit && unattached.length > 0 && (
                  <div className="pt-2">
                    <label className="text-xs uppercase tracking-wide text-muted-foreground">
                      Attach a router
                    </label>
                    <select
                      className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm"
                      defaultValue=""
                      onChange={(e) => {
                        if (!e.target.value) return;
                        assignMutation.mutate({
                          routerId: e.target.value,
                          siteId: s.id,
                        });
                        e.target.value = "";
                      }}
                    >
                      <option value="">Select a router…</option>
                      {unattached.map((r) => (
                        <option key={r.id} value={r.id}>
                          {r.name} ({r.host})
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            </article>
          );
        })}
        {(() => {
          const unassigned = routers.data?.filter((r) => !r.site_id) ?? [];
          if (!unassigned.length) return null;
          return (
            <article className="glass-panel rounded-2xl border border-dashed border-border/80 p-4">
              <header className="mb-3">
                <h3 className="text-lg font-semibold">Unassigned routers</h3>
                <p className="text-xs text-muted-foreground">
                  Not attached to a site yet — pick a site above and use Attach, or set the site
                  when adding the router.
                </p>
              </header>
              <div className="space-y-2">
                {unassigned.map((r) => (
                  <div
                    key={r.id}
                    className="flex items-center justify-between rounded-md border border-border/50 bg-surface/50 px-3 py-2 text-sm"
                  >
                    <span>
                      <strong>{r.name}</strong>{" "}
                      <span className="text-muted-foreground">· {r.host}</span>
                    </span>
                  </div>
                ))}
              </div>
            </article>
          );
        })()}
      </section>
      <TypedConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t.copy("Delete site {name}?", { name: deleteTarget?.name ?? "" })}
        description={t.copy(
          "This site is already bound to {count} device(s). Continue, then type the confirmation phrase. Routers and devices will be unassigned.",
          { count: deleteTarget?.bound ?? 0 },
        )}
        typeHint={t.copy("Type the English phrase below exactly to confirm.")}
        phrase={DELETE_SITE_PHRASE}
        confirmLabel="Delete"
        pending={deleteMutation.isPending}
        pendingLabel="Deleting…"
        onConfirm={(typed) => {
          if (!deleteTarget) return;
          deleteMutation.mutate({ id: deleteTarget.id, confirmation: typed });
        }}
      />
    </div>
  );
}
