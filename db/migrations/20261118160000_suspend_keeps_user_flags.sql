-- Suspending a business used to switch off every user in it, and unsuspending switched them all
-- back on, so a person the owner had disabled came back. Suspension now lives only in
-- businesses.is_active (login, sessions and RequireActiveTenant already read it). Users of a
-- business that is suspended right now were switched off by that old behaviour, so switch them
-- back on here; they stay locked out until the business itself is unsuspended.
UPDATE public.users u
   SET is_active = true, updated_at = now()
  FROM public.businesses b
 WHERE b.business_id = u.business_id AND NOT b.is_active AND NOT u.is_active;
