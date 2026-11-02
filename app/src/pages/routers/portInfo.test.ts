import { describe, expect, it } from "vitest";
import { firmwareText, healthLabel, healthValue, linkText, softwareText } from "./portInfo";

const w = { full: "Full duplex", half: "Half duplex", updateAvail: "Update available", noNewer: "No newer version known", fwUpgrade: "Upgrade available" };

describe("link text", () => {
  it("shows rate and duplex for a running port", () => {
    expect(linkText({ link_rate: "1Gbps", full_duplex: true, running: true }, w)).toBe("1 Gbps · Full duplex");
    expect(linkText({ link_rate: "100Mbps", full_duplex: false, running: true }, w)).toBe("100 Mbps · Half duplex");
    expect(linkText({ link_rate: "2.5Gbps", running: true }, w)).toBe("2.5 Gbps");
  });
  it("shows nothing for a port with no link, a disabled port, or no reported rate", () => {
    expect(linkText({ link_rate: "1Gbps", full_duplex: true, running: false }, w)).toBe("");
    expect(linkText({ link_rate: "1Gbps", running: true, disabled: true }, w)).toBe("");
    expect(linkText({ running: true }, w)).toBe("");
  });
});

describe("health text", () => {
  it("makes readable labels and keeps units", () => {
    expect(healthLabel("cpu-temperature")).toBe("CPU temperature");
    expect(healthLabel("fan1-speed")).toBe("Fan1 speed");
    expect(healthValue({ name: "temperature", value: 44, unit: "°C" })).toBe("44 °C");
    expect(healthValue({ name: "voltage", value: 24.12, unit: "V" })).toBe("24.1 V");
    expect(healthValue({ name: "x", value: 0 })).toBe("0");
  });
});

describe("software text", () => {
  it("claims an update only when the server says so, and never claims up to date without a latest version", () => {
    expect(softwareText("7.12.2", { installed_version: "7.12.2", latest_version: "7.16", channel: "stable", update_available: true }, w)).toBe("7.12.2 · Update available: 7.16 (stable)");
    expect(softwareText("7.12.2", { installed_version: "7.12.2", latest_version: "7.12.2", update_available: false }, w)).toBe("7.12.2 · No newer version known");
    expect(softwareText("7.12.2 (stable)", { installed_version: "7.12.2" }, w)).toBe("7.12.2");
    expect(softwareText("7.12.2", undefined, w)).toBe("7.12.2");
    expect(softwareText(undefined, undefined, w)).toBe("—");
  });
  it("shows the board firmware and a pending upgrade", () => {
    expect(firmwareText({ current_firmware: "7.12.2", upgrade_firmware: "7.16", firmware_upgrade_available: true }, w)).toBe("7.12.2 · Upgrade available: 7.16");
    expect(firmwareText({ current_firmware: "7.12.2", firmware_upgrade_available: false }, w)).toBe("7.12.2");
    expect(firmwareText(undefined, w)).toBe("");
  });
});
