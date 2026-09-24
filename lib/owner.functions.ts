import { createServerFn } from "@tanstack/react-start";
import { z } from "zod";
import { getRequestHeader } from "@tanstack/react-start/server";
import { assertAuthAttemptAllowed } from "./auth-rate-limit.server";

/** Escape PostgREST ilike metacharacters so wildcards can't be used to enumerate accounts. */
function escapeIlike(value: string): string {
  return value.replace(/[\\%_]/g, "\\$&");
}

/**
 * Resolve a username → auth email so the client can sign in via Supabase.
 * For unknown identifiers this returns an unroutable placeholder address so the
 * response never reveals whether an account exists; sign-in then fails with the
 * same generic "invalid credentials" error as a wrong password.
 */
export const resolveLoginEmail = createServerFn({ method: "POST" })
  .inputValidator((raw: unknown) => z.object({ identifier: z.string().min(1).max(120) }).parse(raw))
  .handler(async ({ data }) => {
    const ip =
      getRequestHeader("cf-connecting-ip") ?? getRequestHeader("x-forwarded-for") ?? "unknown";
    await assertAuthAttemptAllowed({
      ip: ip.split(",")[0] ?? "unknown",
      identifier: data.identifier,
    });

    const raw = data.identifier.trim();
    if (raw.includes("@")) return { email: raw };
    const id = escapeIlike(raw);

    const unknown = { email: "unknown-account@invalid.local" };

    const { supabaseAdmin } = await import("@/integrations/supabase/client.server");
    const { data: row } = await supabaseAdmin
      .from("owner_accounts")
      .select("auth_email")
      .ilike("username", id)
      .maybeSingle();
    if (row?.auth_email) return { email: row.auth_email };

    const { data: prof } = await supabaseAdmin
      .from("profiles")
      .select("id")
      .ilike("username", id)
      .maybeSingle();
    if (prof?.id) {
      const { data: u } = await supabaseAdmin.auth.admin.getUserById(prof.id);
      if (u?.user?.email) return { email: u.user.email };
    }
    return unknown;
  });
