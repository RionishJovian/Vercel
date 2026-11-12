import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { Loading, PageSkeleton, SkMetrics, SkRows, Skeleton } from "./Skeleton";

beforeEach(() => {
  const store: Record<string, string> = { mm2_lang: "en" };
  vi.stubGlobal("localStorage", { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; }, removeItem: () => {} });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const wrap = (ui: React.ReactNode) => render(<I18nProvider>{ui}</I18nProvider>);

describe("skeletons", () => {
  it("announce loading once and hide the decorative shapes", () => {
    wrap(<Loading><SkMetrics n={4} /><SkRows n={3} /></Loading>);
    const status = screen.getAllByRole("status");
    expect(status).toHaveLength(1);
    expect(status[0].getAttribute("aria-busy")).toBe("true");
    expect(status[0].textContent).toBe("Loading");
    expect(status[0].querySelector(".sk-shapes")?.getAttribute("aria-hidden")).toBe("true");
  });

  it("reuse the real component classes so loaded content takes the same space", () => {
    const { container } = wrap(<Loading><SkMetrics n={3} cols="three" /><SkRows n={2} /></Loading>);
    expect(container.querySelector(".metrics.three")?.querySelectorAll(".metric")).toHaveLength(3);
    expect(container.querySelectorAll(".linklist .linkrow")).toHaveLength(2);
  });

  it("give older pages an announced block too", () => {
    wrap(<Skeleton h={50} />);
    expect(screen.getByRole("status").textContent).toBe("Loading");
  });

  it("match the home layout for the route fallback", () => {
    const { container } = wrap(<PageSkeleton variant="home" />);
    expect(container.querySelectorAll(".svc-card").length).toBeGreaterThanOrEqual(1);
    expect(container.querySelector(".metrics.four")).not.toBeNull();
  });
});
