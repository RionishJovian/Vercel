import { createFileRoute, Navigate } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/app/magic-dude")({
  head: () => ({
    meta: [
      { title: "Magic Dude — MikroTik Magic" },
      {
        name: "description",
        content: "Open Magic Dude from the floating chat button inside MikroTik Magic.",
      },
    ],
  }),
  component: MagicDudeRedirect,
});

/**
 * Magic Dude is a floating assistant, not a separate dashboard workspace.
 * Keep the old URL as a backwards-compatible redirect for saved links.
 */
function MagicDudeRedirect() {
  return <Navigate to="/app" replace />;
}
