// @lovable.dev/vite-tanstack-config already includes the following — do NOT add them manually
// or the app will break with duplicate plugins:
//   - TanStack devtools (dev-only, first), tanstackStart, viteReact, tailwindcss, tsConfigPaths,
//     nitro (build-only using cloudflare as a default target), VITE_* env injection, @ path alias,
//     React/TanStack dedupe, error logger plugins, and sandbox detection (port/host/strictPort).
// You can pass additional config via defineConfig({ vite: { ... }, etc... }) if needed.
import path from "node:path";
import { defineConfig } from "@lovable.dev/vite-tanstack-config";
import { mcpPlugin } from "@lovable.dev/mcp-js/stacks/tanstack/vite";
import { loadEnv } from "vite";

// Load all env vars into process.env for server-side routes.
const serverEnv = loadEnv(process.env["NODE_ENV"] ?? "development", process.cwd(), "");
Object.assign(process.env, serverEnv);

// Lovable Cloud provides SUPABASE_URL / SUPABASE_PUBLISHABLE_KEY (server secrets).
// Vite only inlines VITE_* into the browser bundle, so mirror the public pair.
// Without this, client navigations to /auth crash with "Missing Supabase…".
const supabaseUrl = process.env["VITE_SUPABASE_URL"] || process.env["SUPABASE_URL"];
const supabasePublishableKey =
  process.env["VITE_SUPABASE_PUBLISHABLE_KEY"] || process.env["SUPABASE_PUBLISHABLE_KEY"];
if (supabaseUrl) process.env["VITE_SUPABASE_URL"] = supabaseUrl;
if (supabasePublishableKey) process.env["VITE_SUPABASE_PUBLISHABLE_KEY"] = supabasePublishableKey;

export default defineConfig({
  tanstackStart: {
    server: { entry: "server" },
    nitro: { preset: "node-server" },
  },
  vite: {
    plugins: [mcpPlugin()],
    resolve: {
      alias: {
        "entities/lib/decode.js": path.resolve(
          process.cwd(),
          "node_modules/entities/lib/decode.js",
        ),
        "entities/lib/encode.js": path.resolve(
          process.cwd(),
          "node_modules/entities/lib/encode.js",
        ),
        entities: path.resolve(process.cwd(), "node_modules/entities"),
      },
    },
    build: {
      rollupOptions: {
        output: {
          manualChunks: (id: string) => {
            if (id.includes("node_modules")) {
              if (id.includes("@tanstack/react-router")) return "vendor-router";
              if (id.includes("motion/react")) return "vendor-motion";
              if (id.includes("@supabase/supabase-js")) return "vendor-supabase";
              if (id.includes("@radix-ui/react-dialog") ||
                  id.includes("@radix-ui/react-dropdown-menu") ||
                  id.includes("@radix-ui/react-popover") ||
                  id.includes("@radix-ui/react-sidebar")) return "vendor-ui";
              if (id.includes("react") || id.includes("react-dom")) return "vendor-react";
              if (id.includes("@tanstack/react-query") ||
                  id.includes("@tanstack/react-start")) return "vendor-tanstack";
              return "vendor";
            }
          },
        },
      },
      chunkSizeWarningLimit: 100,
    },
  },
});
