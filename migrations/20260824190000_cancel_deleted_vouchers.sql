-- A voucher removed by an operator is audit-retained as cancelled, not erased.
-- Cancelled codes must never contribute to voucher revenue or earnings.
UPDATE public.voucher_codes
   SET status = 'cancelled'
 WHERE status = 'deleted';
