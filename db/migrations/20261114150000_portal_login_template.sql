-- Captive-portal login page templates (additive, forward-only).
--
-- template_id '' means "never chose one": the page keeps the look it had before templates existed, and no add-on is
-- needed for it. The id is validated by the application (routeros.ValidPortalTemplate), not by a CHECK, so adding a
-- template later never needs a migration.
ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS template_id text NOT NULL DEFAULT '';

-- Store add-ons: seven premium login page themes and a discounted bundle of all seven. starlink_mm and daylight are
-- free and have no add-on. An approved store_addon_requests row is the entitlement (see portal_template_access.go).
-- Prices are starting values the platform administrator can change in Platform > Add-ons; DO NOTHING keeps any
-- price or text already edited there.
INSERT INTO public.store_addons (addon_key, kind, name_en, name_my, description_en, description_my, price_mmk, period, active, sort_order) VALUES
 ('portal_theme_cultural_mm', 'feature', 'Portal theme: Myanmar Cultural Heritage', 'ပေါ်တယ် ပုံစံ: မြန်မာ ယဉ်ကျေးမှု အမွေအနှစ်', 'Lacquer crimson and Burmese gold login page for your router.', 'သင့် router အတွက် ကြာဖန်နီနှင့် မြန်မာ့ရွှေရောင် login စာမျက်နှာ။', 5000, 'one_time', true, 210),
 ('portal_theme_mountain_mm', 'feature', 'Portal theme: Highland & Mountain WISP', 'ပေါ်တယ် ပုံစံ: တောင်ပေါ် WISP', 'Pine-green mountain ridge login page with a tower signal mark.', 'တောင်တန်းနှင့် signal တာဝါပုံ ပါသော အစိမ်းရင့်ရောင် login စာမျက်နှာ။', 5000, 'one_time', true, 220),
 ('portal_theme_village_mm', 'feature', 'Portal theme: Village & Community Wi-Fi', 'ပေါ်တယ် ပုံစံ: ရွာနှင့် ရပ်ကွက် Wi-Fi', 'Large Burmese type, big tap targets and 1-2-3 voucher steps.', 'ကြီးသော မြန်မာစာလုံး၊ ကြီးသော ခလုတ်များနှင့် ၁-၂-၃ အဆင့်များ။', 5000, 'one_time', true, 230),
 ('portal_theme_obsidian', 'feature', 'Portal theme: Obsidian Telemetry', 'ပေါ်တယ် ပုံစံ: Obsidian', 'Dark slate login page with contour lines and a monospace voucher field.', 'ကွန်တိုးမျဉ်းများနှင့် monospace ကုဒ်အကွက် ပါသော အမှောင်ရောင် login စာမျက်နှာ။', 5000, 'one_time', true, 240),
 ('portal_theme_cafe', 'feature', 'Portal theme: Warm Café & Bistro', 'ပေါ်တယ် ပုံစံ: ကော်ဖီဆိုင်', 'Warm cream and espresso login page with soft rounded corners.', 'နွေးထွေးသော ခရင်မ်နှင့် ကော်ဖီရောင် login စာမျက်နှာ။', 5000, 'one_time', true, 250),
 ('portal_theme_hotel', 'feature', 'Portal theme: Boutique Hotel & Lounge', 'ပေါ်တယ် ပုံစံ: ဟိုတယ်နှင့် လောင်းချ်', 'Editorial login page with guest-pass tier cards from your plans.', 'သင့်ပလန်များမှ ဧည့်သည် ပတ်စ်ကတ်များ ပါသော ဟိုတယ်ပုံစံ login စာမျက်နှာ။', 5000, 'one_time', true, 260),
 ('portal_theme_express', 'feature', 'Portal theme: Express Retail & Transit', 'ပေါ်တယ် ပုံစံ: Express', 'Bold high-contrast login page with an 8-box voucher field.', 'အကွက် ၈ ကွက် ကုဒ်ဖြည့်ရာ၊ အရောင်ကွဲပြားသော login စာမျက်နှာ။', 5000, 'one_time', true, 270),
 ('portal_themes_bundle', 'feature', 'All 7 Premium Portal Themes Bundle', 'ပရီမီယံ ပေါ်တယ်ပုံစံ ၇ မျိုးလုံး ပက်ကေ့ချ်', 'Every premium login page theme, now and in the same set later, at a bundle price.', 'ပရီမီယံ login စာမျက်နှာ ပုံစံ ၇ မျိုးလုံးကို ပက်ကေ့ချ်စျေးဖြင့်။', 25000, 'one_time', true, 200)
ON CONFLICT (addon_key) DO NOTHING;
