import type { IconName } from "../components/icons";

/**
 * One navigation model for every screen size.
 * Phone: five bottom destinations. Tablet/desktop: a grouped left rail.
 * Every existing hash route keeps working; this only decides which destination is highlighted.
 */
export type Tab = "home" | "net" | "vouchers" | "activity" | "more";

export const TABS: Array<{ key: Tab; icon: IconName; label: string }> = [
  { key: "home", icon: "home", label: "nav.home" },
  { key: "net", icon: "nodes", label: "nav.network" },
  { key: "vouchers", icon: "ticket", label: "nav.vouchers" },
  { key: "activity", icon: "activity", label: "nav.activity" },
  { key: "more", icon: "dots", label: "nav.more" },
];

const TAB_OF: Record<string, Tab> = {
  home: "home", hotspot: "home", isp: "home", pppoe: "home", customers: "home", guests: "home", revenue: "home", setup: "home",
  net: "net", router: "net", network: "net", sites: "net", hub: "net", topology: "net", devices: "net", "managed-devices": "net", approvals: "net", backups: "net",
  vouchers: "vouchers",
  activity: "activity", incidents: "activity", events: "activity", fleet: "activity", security: "activity",
};
export const tabFor = (route: string): Tab => TAB_OF[route] ?? "more";

export interface RailItem { href: string; icon: IconName; label: string; routes: string[]; staff?: boolean }
export interface RailGroup { label: string; items: RailItem[] }

export const RAIL: RailGroup[] = [
  { label: "nav.g.overview", items: [
    { href: "#/home", icon: "home", label: "nav.home", routes: ["home"] },
    { href: "#/setup", icon: "rocket", label: "nav.setup", routes: ["setup"] },
  ] },
  { label: "nav.g.services", items: [
    { href: "#/hotspot", icon: "wifi", label: "nav.hotspot", routes: ["hotspot", "guests"] },
    { href: "#/vouchers", icon: "ticket", label: "nav.vouchers", routes: ["vouchers"] },
    { href: "#/isp", icon: "isp", label: "nav.isp", routes: ["isp", "pppoe"] },
    { href: "#/customers", icon: "users", label: "nav.customers", routes: ["customers"] },
  ] },
  { label: "nav.g.network", items: [
    { href: "#/net", icon: "nodes", label: "nav.network", routes: ["net", "router", "network", "sites", "hub", "backups"] },
    { href: "#/topology", icon: "grid", label: "nav.topology", routes: ["topology"] },
    { href: "#/managed-devices", icon: "device", label: "nav.devices", routes: ["managed-devices", "devices"] },
    { href: "#/approvals", icon: "approve", label: "nav.approvals", routes: ["approvals"] },
  ] },
  { label: "nav.g.monitor", items: [
    { href: "#/activity", icon: "activity", label: "nav.activity", routes: ["activity", "events", "fleet"] },
    { href: "#/incidents", icon: "alert", label: "nav.alerts", routes: ["incidents"] },
  ] },
  { label: "nav.g.business", items: [
    { href: "#/revenue", icon: "cash", label: "nav.revenue", routes: ["revenue"] },
    { href: "#/more", icon: "gear", label: "nav.settings", routes: ["more", "profile", "security", "store", "guide", "jellyfin", "platform"] },
  ] },
];
