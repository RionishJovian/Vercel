import { createServerFn } from "@tanstack/react-start";
import { z } from "zod";
import { requireSupabaseAuth } from "@/integrations/supabase/auth-middleware";
import type { DatabaseClient } from "./database.types";
import {
  DEFAULT_VOUCHER_PRINT_LAYOUT,
  normalizeVoucherPrintLayout,
  type VoucherPrintLayout,
} from "./voucher-print-layout";

const layoutSchema = z.object({
  business_name: z.string().trim().min(1).max(80),
  wifi_name: z.string().trim().min(1).max(80),
  support_contact: z.string().trim().min(1).max(120),
  terms: z.string().trim().min(1).max(600),
  paper_width_mm: z.union([z.literal(58), z.literal(80)]),
  show_qr: z.boolean(),
  show_price: z.boolean(),
  show_expiry: z.boolean(),
  a4_columns: z.union([z.literal(2), z.literal(3), z.literal(4), z.literal(5)]),
  a4_rows_per_page: z.number().int().min(4).max(50),
  thermal_columns: z.union([z.literal(1), z.literal(2)]),
  thermal_rows_per_strip: z.number().int().min(4).max(40),
});

async function contextForLayout(
  context: { supabase: DatabaseClient; userId: string },
  write = false,
) {
  const { effectiveOwner, requireVoucherPrintLayoutManager } = await import("./guards.server");
  if (write) {
    await requireVoucherPrintLayoutManager(context.supabase, context.userId);
  }
  const ownerId = await effectiveOwner(context.supabase, context.userId);
  return { ownerId, db: context.supabase };
}

function portalDefaults(portal: Record<string, unknown> | null | undefined): VoucherPrintLayout {
  return normalizeVoucherPrintLayout({
    ...DEFAULT_VOUCHER_PRINT_LAYOUT,
    business_name: String(portal?.business_name ?? DEFAULT_VOUCHER_PRINT_LAYOUT.business_name),
    support_contact: String(portal?.seller_phone ?? DEFAULT_VOUCHER_PRINT_LAYOUT.support_contact),
    terms: String(portal?.terms ?? DEFAULT_VOUCHER_PRINT_LAYOUT.terms),
  });
}

/** All voucher operators may read the active print layout; only tenant staff may change it. */
export const getVoucherPrintLayout = createServerFn({ method: "GET" })
  .middleware([requireSupabaseAuth])
  .handler(async ({ context }) => {
    const { ownerId, db } = await contextForLayout(context);
    const [{ data: layout, error: layoutError }, { data: portal, error: portalError }] =
      await Promise.all([
        db
          .from("voucher_print_layouts")
          .select(
            "business_name, wifi_name, support_contact, terms, paper_width_mm, show_qr, show_price, show_expiry, a4_columns, a4_rows_per_page, thermal_columns, thermal_rows_per_strip",
          )
          .eq("owner_id", ownerId)
          .maybeSingle(),
        db
          .from("portal_settings")
          .select("business_name, seller_phone, terms")
          .eq("owner_id", ownerId)
          .maybeSingle(),
      ]);
    if (layoutError) throw new Error(layoutError.message);
    if (portalError) throw new Error(portalError.message);
    return normalizeVoucherPrintLayout(layout ?? portalDefaults(portal));
  });

export const saveVoucherPrintLayout = createServerFn({ method: "POST" })
  .middleware([requireSupabaseAuth])
  .inputValidator((raw: unknown) => layoutSchema.parse(raw))
  .handler(async ({ data, context }) => {
    const { ownerId, db } = await contextForLayout(context, true);
    const { error } = await db
      .from("voucher_print_layouts")
      .upsert({ owner_id: ownerId, ...data }, { onConflict: "owner_id" });
    if (error) throw new Error(error.message);
    return normalizeVoucherPrintLayout(data);
  });
