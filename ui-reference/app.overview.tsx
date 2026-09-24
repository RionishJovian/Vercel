import { createFileRoute } from "@tanstack/react-router";
import { BusinessSummary } from "@/components/BusinessSummary";
import { MagicGoLiveStrip } from "@/components/MagicGoLiveStrip";
import { HomeSiteTopology } from "@/components/HomeSiteTopology";

export const Route = createFileRoute("/_authenticated/app/overview")({
  head: () => ({
    meta: [
      { title: "Overview — MikroTik Magic" },
      {
        name: "description",
        content: "Revenue, go-live checklist, site topology and fleet health.",
      },
      { name: "robots", content: "noindex" },
    ],
  }),
  component: OverviewPage,
});

/**
 * Overview — the "how are things?" page.
 *
 * Home ("what can I do?") is the launcher card grid; this page carries the
 * status surfaces that used to sit on top of it: revenue, go-live, topology.
 */
function OverviewPage() {
  return (
    <>
      <h1 className="sr-only">Overview</h1>

      <div className="mb-4">
        <BusinessSummary />
      </div>

      <div className="mb-4">
        <MagicGoLiveStrip compact skipHotspotProbe />
      </div>

      <div className="mb-4">
        <HomeSiteTopology />
      </div>

    </>
  );
}
