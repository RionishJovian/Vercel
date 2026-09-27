package model

import "time"

// I18nOverride is a single translated UI string.
//
// ⚠️ THIS TABLE IS GLOBAL, NOT TENANT-SCOPED. `i18n_overrides` has no
// `business_id` column and RLS is DISABLED on it (verified: relrowsecurity=false,
// 0 policies), so every business reads the same rows. A tenant-scoped write would
// silently rewrite the UI for every other tenant — which is why the write routes
// are gated to the DEVELOPER platform role and not to tenant admins.
type I18nOverride struct {
	ID        string    `json:"id"`
	Locale    string    `json:"locale"`
	NS        string    `json:"namespace"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedBy *string   `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// I18nLocales mirrors the `i18n_overrides_locale_check` constraint exactly.
// A closed set, so an unknown locale is a 400 rather than an empty page.
var I18nLocales = []string{"en", "zh", "my"}

// I18nNamespaces mirrors the `i18n_overrides_ns_check` constraint exactly.
var I18nNamespaces = []string{"ui", "copy"}

// IsI18nLocale reports whether v is a locale the database accepts.
func IsI18nLocale(v string) bool { return containsStr(I18nLocales, v) }

// IsI18nNamespace reports whether v is a namespace the database accepts.
func IsI18nNamespace(v string) bool { return containsStr(I18nNamespaces, v) }

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
