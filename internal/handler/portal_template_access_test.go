package handler

import (
	"testing"

	"mega-magic/internal/provisioner/routeros"
)

var premiumTemplates = []string{"cultural_mm", "mountain_mm", "village_mm", "obsidian", "cafe", "hotel", "express"}

func TestFreePortalTemplatesNeedNoAddOn(t *testing.T) {
	for _, id := range []string{"starlink_mm", "daylight", ""} {
		if !portalTemplateAllowed(id, "OWNER", nil) {
			t.Errorf("%q must be free", id)
		}
	}
}

func TestPremiumPortalTemplatesLockedUntilAddOnApproved(t *testing.T) {
	if len(premiumTemplates) != 7 {
		t.Fatal("seven premium templates expected")
	}
	for _, id := range premiumTemplates {
		if !portalTemplatePremium(id) || portalTemplateAllowed(id, "OWNER", map[string]bool{}) || portalTemplateAllowed(id, "ADMIN", nil) {
			t.Errorf("%s must be locked without an add-on", id)
		}
		// Another theme's add-on never unlocks this one.
		other := map[string]bool{portalThemeAddonKey("cafe"): true}
		if id != "cafe" && portalTemplateAllowed(id, "OWNER", other) {
			t.Errorf("%s unlocked by a different theme's add-on", id)
		}
		if !portalTemplateAllowed(id, "OWNER", map[string]bool{portalThemeAddonKey(id): true}) {
			t.Errorf("%s must unlock with its own add-on", id)
		}
		if !portalTemplateAllowed(id, "OWNER", map[string]bool{portalBundleAddonKey: true}) {
			t.Errorf("%s must unlock with the bundle", id)
		}
	}
}

func TestPlatformAdministratorIsExemptFromAllNine(t *testing.T) {
	n := 0
	for _, tpl := range routeros.PortalTemplates() {
		n++
		if !portalTemplateAllowed(tpl.ID, "DEVELOPER", nil) {
			t.Errorf("platform administrator blocked from %s", tpl.ID)
		}
	}
	if n != 9 {
		t.Fatalf("want 9 templates, got %d", n)
	}
}

func TestEveryPremiumTemplateHasAnAddOnKeyInTheMigration(t *testing.T) {
	// The seeded add-on keys must match what the entitlement rule looks for, and be valid store keys.
	for _, id := range premiumTemplates {
		if k := portalThemeAddonKey(id); !addonKeyPattern.MatchString(k) {
			t.Errorf("%s is not a valid add-on key", k)
		}
	}
	if !addonKeyPattern.MatchString(portalBundleAddonKey) {
		t.Fatal("bundle key invalid")
	}
	for _, tpl := range routeros.PortalTemplates() {
		if portalTemplatePremium(tpl.ID) != !portalFreeTemplates[tpl.ID] {
			t.Errorf("%s premium flag inconsistent", tpl.ID)
		}
	}
	if portalTemplatePremium("not_a_template") {
		t.Fatal("unknown template must not be treated as premium; validation rejects it instead")
	}
}
