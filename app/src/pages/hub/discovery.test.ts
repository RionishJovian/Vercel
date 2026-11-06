import { describe, expect, it } from "vitest";
import { DEFAULT_ROUTER_IP, candidateSource, parsePrivateIPv4 } from "./discovery";

describe("parsePrivateIPv4", () => {
  it("accepts private addresses and trims them", () => {
    expect(parsePrivateIPv4("192.168.88.1")).toBe("192.168.88.1");
    expect(parsePrivateIPv4(" 10.0.0.1 ")).toBe("10.0.0.1");
    expect(parsePrivateIPv4("172.16.5.254")).toBe("172.16.5.254");
    expect(parsePrivateIPv4("172.31.0.1")).toBe("172.31.0.1");
  });
  it("rejects public, loopback, link-local and special addresses", () => {
    for (const bad of ["8.8.8.8", "127.0.0.1", "169.254.1.1", "172.32.0.1", "172.15.0.1", "192.169.1.1", "0.0.0.0", "192.168.88.0", "192.168.88.255"]) expect(parsePrivateIPv4(bad)).toBeNull();
  });
  it("rejects malformed input, including values that could change the link target", () => {
    for (const bad of ["", "192.168.88", "192.168.88.1.5", "192.168.088.1", "192.168.88.256", "abc", "192.168.88.1/admin", "192.168.88.1:8080", "192.168.88.1@evil.test", "http://192.168.88.1"]) expect(parsePrivateIPv4(bad)).toBeNull();
  });
});

describe("candidateSource", () => {
  it("calls only the factory default a suggestion", () => {
    expect(candidateSource(DEFAULT_ROUTER_IP)).toBe("suggested");
    expect(candidateSource("192.168.1.1")).toBe("manual");
  });
});
