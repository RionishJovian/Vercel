/** Display text for the read-only link, sensor and software readings the router reports. */
export interface LinkInfo { link_rate?: string; full_duplex?: boolean; running?: boolean; disabled?: boolean }
export interface Health { name: string; value: number; unit?: string }
export interface Firmware { installed_version?: string; latest_version?: string; channel?: string; current_firmware?: string; upgrade_firmware?: string; update_available?: boolean; firmware_upgrade_available?: boolean }
export interface Words { full: string; half: string; updateAvail: string; noNewer: string; fwUpgrade: string }

/** "1Gbps" -> "1 Gbps". Only a rate the router reported is shown; a port with no link has none. */
export function linkText(i: LinkInfo, w: Words): string {
  if (i.disabled || !i.running || !i.link_rate) return "";
  const rate = i.link_rate.replace(/^([0-9.]+)([MGT]bps)$/, "$1 $2");
  const duplex = i.full_duplex === true ? w.full : i.full_duplex === false ? w.half : "";
  return duplex ? `${rate} · ${duplex}` : rate;
}

/** "cpu-temperature" -> "CPU temperature", "fan1-speed" -> "Fan1 speed". */
export function healthLabel(name: string): string {
  const spaced = name.replace(/[-_]+/g, " ").trim();
  const words = spaced.split(" ").map((p) => (/^(cpu|psu|poe|sfp)$/i.test(p) ? p.toUpperCase() : p));
  const text = words.join(" ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

export function healthValue(h: Health): string {
  const n = Number.isInteger(h.value) ? String(h.value) : h.value.toFixed(1);
  return h.unit ? `${n} ${h.unit}` : n;
}

/** RouterOS line: "7.12.2" plus what the router last learned about a newer version. Never says "up to date" without a latest version. */
export function softwareText(version: string | undefined, fw: Firmware | undefined, w: Words): string {
  const base = fw?.installed_version || version || "—";
  if (!fw) return base;
  if (fw.update_available && fw.latest_version) return `${base} · ${w.updateAvail}: ${fw.latest_version}${fw.channel ? ` (${fw.channel})` : ""}`;
  if (fw.latest_version) return `${base} · ${w.noNewer}`;
  return base;
}

export function firmwareText(fw: Firmware | undefined, w: Words): string {
  if (!fw?.current_firmware) return "";
  return fw.firmware_upgrade_available && fw.upgrade_firmware ? `${fw.current_firmware} · ${w.fwUpgrade}: ${fw.upgrade_firmware}` : fw.current_firmware;
}
