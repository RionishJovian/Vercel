package handler

import (
	"os"
	"strings"
	"testing"
)

func TestPortalThemeMigrationSeedsEveryPremiumAddOn(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/20261114150000_portal_login_template.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(raw)
	for _, id := range premiumTemplates {
		if !strings.Contains(sqlText, "'"+portalThemeAddonKey(id)+"'") {
			t.Errorf("migration does not seed %s", portalThemeAddonKey(id))
		}
	}
	for _, free := range []string{"portal_theme_starlink_mm", "portal_theme_daylight"} {
		if strings.Contains(sqlText, free) {
			t.Errorf("free template must have no add-on: %s", free)
		}
	}
	for _, want := range []string{"'" + portalBundleAddonKey + "'", "ON CONFLICT (addon_key) DO NOTHING", "ADD COLUMN IF NOT EXISTS template_id"} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("migration missing %q", want)
		}
	}
	if strings.Contains(strings.ToUpper(sqlText), "DROP ") || strings.Contains(strings.ToUpper(sqlText), "DELETE ") {
		t.Error("migration must be forward-only and non-destructive")
	}
}
