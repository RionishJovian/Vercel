import { createFileRoute, redirect } from "@tanstack/react-router";

/**
 * Public IP / DDNS Quick Setup was removed from the product.
 * Deep links land on Routers with Magic Hub selected.
 */
export const Route = createFileRoute("/_authenticated/app/quick-setup")({
  beforeLoad: () => {
    throw redirect({ to: "/app/routers", search: { method: "hub" } });
  },
});
