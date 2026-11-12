import { afterEach, describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { hashToPath, navigate, normalizeLocation, parsePath, parseRoute, useRoute } from "./route";

afterEach(() => { history.replaceState(null, "", "/"); });

describe("console routes", () => {
  it("reads /app/ paths", () => {
    expect(parsePath("/app/vouchers/create", "?x=1")).toMatchObject({ name: "vouchers", sub: "create" });
    expect(parsePath("/app/vouchers/create", "?x=1").params.get("x")).toBe("1");
    expect(parsePath("/app/").name).toBe("home");
    expect(parsePath("/elsewhere").name).toBe("home");
  });

  it("reads legacy hash routes", () => {
    expect(parseRoute("#/router/map?id=7")).toMatchObject({ name: "router", sub: "map" });
    expect(parseRoute("#/router/map?id=7").params.get("id")).toBe("7");
  });

  it("turns a hash route into its /app/ path with the query kept", () => {
    expect(hashToPath("#/router/map?id=1")).toBe("/app/router/map?id=1");
    expect(hashToPath("#/home")).toBe("/app/home");
    expect(hashToPath("#//evil.example")).toBe("/app/evil.example");
  });

  it("rewrites an old /v2/#/page bookmark to /app/page without adding history", () => {
    history.replaceState(null, "", "/app/#/vouchers?tab=1");
    const before = history.length;
    normalizeLocation();
    expect(location.pathname + location.search + location.hash).toBe("/app/vouchers?tab=1");
    expect(history.length).toBe(before);
  });

  it("follows navigation and the Back button", () => {
    history.replaceState(null, "", "/app/home");
    const { result } = renderHook(() => useRoute());
    expect(result.current.name).toBe("home");
    act(() => navigate("/app/net?router=r1"));
    expect(result.current.name).toBe("net");
    expect(result.current.params.get("router")).toBe("r1");
    act(() => navigate("#/vouchers/create"));
    expect(location.pathname).toBe("/app/vouchers/create");
    act(() => { history.replaceState(null, "", "/app/net"); window.dispatchEvent(new PopStateEvent("popstate")); });
    expect(result.current.name).toBe("net");
  });

  it("handles plain clicks on console links in place and leaves other links alone", () => {
    history.replaceState(null, "", "/app/home");
    renderHook(() => useRoute());
    const link = (href: string, extra: (a: HTMLAnchorElement) => void = () => {}) => {
      const a = document.createElement("a"); a.href = href; a.setAttribute("href", href); extra(a); document.body.append(a); return a;
    };
    const click = (a: HTMLAnchorElement, init: MouseEventInit = {}) => {
      const e = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0, ...init });
      a.dispatchEvent(e); a.remove(); return e.defaultPrevented;
    };
    expect(click(link("/app/activity"))).toBe(true);
    expect(location.pathname).toBe("/app/activity");
    expect(click(link("#/more"))).toBe(true);
    expect(location.pathname).toBe("/app/more");
    expect(click(link("/app/net"), { ctrlKey: true })).toBe(false);
    expect(click(link("/app/net", (a) => { a.target = "_blank"; }))).toBe(false);
    expect(click(link("/app/assets/report.pdf"))).toBe(false);
    expect(click(link("https://example.com/app/x"))).toBe(false);
    expect(click(link("//example.com/app/x"))).toBe(false);
    expect(location.pathname).toBe("/app/more");
  });
});
