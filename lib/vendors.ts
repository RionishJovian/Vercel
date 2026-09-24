// Unified multi-vendor device registry model.
//
// Pure and browser-safe. One place decides what a given vendor + category +
// transport can actually do, so the UI can disable an action with a reason
// instead of letting it fail at runtime against the device.

export const DEVICE_VENDORS = [
  "mikrotik",
  "ruijie",
  "cisco",
  "tplink",
  "ubiquiti",
  "generic",
] as const;
export type DeviceVendor = (typeof DEVICE_VENDORS)[number];

export const VENDOR_LABEL: Record<DeviceVendor, string> = {
  mikrotik: "MikroTik",
  ruijie: "Ruijie / Reyee",
  cisco: "Cisco",
  tplink: "TP-Link",
  ubiquiti: "Ubiquiti UniFi",
  generic: "Other vendor",
};

export const DEVICE_CATEGORIES = ["router", "gateway", "switch", "ap", "controller"] as const;
export type DeviceCategory = (typeof DEVICE_CATEGORIES)[number];

export const CATEGORY_LABEL: Record<DeviceCategory, string> = {
  router: "Router",
  gateway: "Gateway",
  switch: "Switch",
  ap: "Access point",
  controller: "Controller",
};

/** How the app reaches the device. */
export const DEVICE_TRANSPORTS = ["direct", "connector", "controller"] as const;
export type DeviceTransport = (typeof DEVICE_TRANSPORTS)[number];

export const TRANSPORT_LABEL: Record<DeviceTransport, string> = {
  direct: "Direct (public host / DDNS)",
  connector: "Local connector",
  controller: "Through a controller",
};

export const DEVICE_ACTIONS = [
  "inventory",
  "health",
  "ports",
  "portToggle",
  "poeRead",
  "poeCycle",
  "vlanRead",
  "vlanWrite",
  "ssidRead",
  "ssidWrite",
  "reboot",
  "backup",
  "provision",
  "terminal",
] as const;
export type DeviceAction = (typeof DEVICE_ACTIONS)[number];

export const ACTION_LABEL: Record<DeviceAction, string> = {
  inventory: "Inventory & identity",
  health: "Health & reachability",
  ports: "Port list & link state",
  portToggle: "Enable / disable port",
  poeRead: "PoE state & power draw",
  poeCycle: "PoE power-cycle a port",
  vlanRead: "VLAN / port role",
  vlanWrite: "Change VLAN / port role",
  ssidRead: "SSID list",
  ssidWrite: "Change SSID",
  reboot: "Reboot device",
  backup: "Configuration backup",
  provision: "One-click provisioning",
  terminal: "Terminal / API console",
};

export type CapabilityState = { supported: true } | { supported: false; reason: string };

type Matrix = Partial<Record<DeviceVendor, Partial<Record<DeviceCategory, DeviceAction[]>>>>;

/**
 * What each vendor + category combination supports through this app today.
 * Anything absent from a list is unsupported and must be surfaced as disabled.
 */
const SUPPORTED: Matrix = {
  mikrotik: {
    router: [
      "inventory",
      "health",
      "ports",
      "portToggle",
      "vlanRead",
      "vlanWrite",
      "ssidRead",
      "ssidWrite",
      "reboot",
      "backup",
      "provision",
      "terminal",
    ],
    gateway: [
      "inventory",
      "health",
      "ports",
      "portToggle",
      "vlanRead",
      "vlanWrite",
      "reboot",
      "backup",
      "provision",
      "terminal",
    ],
    switch: [
      "inventory",
      "health",
      "ports",
      "portToggle",
      "poeRead",
      "poeCycle",
      "vlanRead",
      "vlanWrite",
      "reboot",
      "backup",
      "terminal",
    ],
    ap: ["inventory", "health", "ssidRead", "ssidWrite", "reboot", "terminal"],
  },
  ubiquiti: {
    ap: ["inventory", "health", "ssidRead", "ssidWrite", "reboot"],
    controller: ["inventory", "health", "ssidRead", "ssidWrite"],
    switch: ["inventory", "health", "ports", "poeRead", "poeCycle", "vlanRead", "reboot"],
  },
  ruijie: {
    gateway: ["inventory", "health", "ports", "vlanRead", "reboot"],
    switch: ["inventory", "health", "ports", "poeRead", "poeCycle", "vlanRead"],
    ap: ["inventory", "health", "ssidRead", "ssidWrite", "reboot"],
  },
  cisco: {
    switch: ["inventory", "health", "ports", "poeRead", "vlanRead"],
  },
  tplink: {
    switch: ["inventory", "health", "ports", "poeRead", "poeCycle", "vlanRead"],
    ap: ["inventory", "health", "ssidRead"],
  },
  generic: {
    router: ["inventory", "health"],
    gateway: ["inventory", "health"],
    switch: ["inventory", "health"],
    ap: ["inventory", "health"],
    controller: ["inventory", "health"],
  },
};

/** Actions a transport can never carry, regardless of vendor. */
const TRANSPORT_BLOCKS: Partial<Record<DeviceTransport, Partial<Record<DeviceAction, string>>>> = {
  controller: {
    terminal: "Devices adopted by a controller are managed through the controller, not directly.",
    backup: "Take the backup from the controller instead of the adopted device.",
  },
};

export type DeviceDescriptor = {
  vendor: DeviceVendor;
  category: DeviceCategory;
  transport: DeviceTransport;
  model?: string | null;
};

/** Whether one action is available, with a plain-language reason when not. */
export function capability(device: DeviceDescriptor, action: DeviceAction): CapabilityState {
  const byCategory = SUPPORTED[device.vendor];
  const allowed = byCategory?.[device.category];
  if (!allowed) {
    return {
      supported: false,
      reason: `${VENDOR_LABEL[device.vendor]} ${CATEGORY_LABEL[device.category].toLowerCase()}s are not supported yet.`,
    };
  }
  if (!allowed.includes(action)) {
    return {
      supported: false,
      reason: `${ACTION_LABEL[action]} is not available for ${VENDOR_LABEL[device.vendor]} ${CATEGORY_LABEL[
        device.category
      ].toLowerCase()}s.`,
    };
  }
  const blocked = TRANSPORT_BLOCKS[device.transport]?.[action];
  if (blocked) return { supported: false, reason: blocked };
  return { supported: true };
}

export function isSupported(device: DeviceDescriptor, action: DeviceAction): boolean {
  return capability(device, action).supported;
}

/** Full matrix row for one device — used by the UI to render the badge grid. */
export function capabilitiesOf(
  device: DeviceDescriptor,
): Array<{ action: DeviceAction; label: string; state: CapabilityState }> {
  return DEVICE_ACTIONS.map((action) => ({
    action,
    label: ACTION_LABEL[action],
    state: capability(device, action),
  }));
}

export function supportedActions(device: DeviceDescriptor): DeviceAction[] {
  return DEVICE_ACTIONS.filter((a) => isSupported(device, a));
}

/** Throws the same message the UI shows, for server-side enforcement. */
export function assertCapability(device: DeviceDescriptor, action: DeviceAction): void {
  const state = capability(device, action);
  if (!state.supported) throw new Error(state.reason);
}

export function isVendor(v: unknown): v is DeviceVendor {
  return typeof v === "string" && (DEVICE_VENDORS as readonly string[]).includes(v);
}

export function isCategory(v: unknown): v is DeviceCategory {
  return typeof v === "string" && (DEVICE_CATEGORIES as readonly string[]).includes(v);
}
