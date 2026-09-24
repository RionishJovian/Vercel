-- Performance indexes for the owner-scoped revenue and voucher activity reads.
-- These preserve the existing accounting semantics while avoiding repeated
-- full-table scans when the dashboard orders recent activity.

create index if not exists voucher_codes_owner_created_at_idx
  on public.voucher_codes (owner_id, created_at desc);

create index if not exists voucher_codes_owner_first_seen_expires_idx
  on public.voucher_codes (owner_id, first_seen_at, expires_at);

create index if not exists payment_orders_owner_settled_at_idx
  on public.payment_orders (owner_id, settled_at desc);
