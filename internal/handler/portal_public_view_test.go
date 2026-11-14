package handler

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"mega-magic/internal/model"
)

func fullPortalConfig() *model.PortalConfig {
	return &model.PortalConfig{
		BusinessID: "11111111-1111-4111-8111-111111111111", BusinessName: "Cafe", WelcomeText: "Hi", TermsText: "Terms",
		PrimaryHex: "#0a7", GlassTintHex: "#123", ConsentRequired: true, ConsentText: "OK",
		BankName: "Bank", BankAccount: "123", BankHolder: "Holder", MMQRURL: "https://example.test/qr.png",
		PaymentInstructions: "Pay", MediaHost: "192.168.88.20", MediaPort: "8096",
		GuestGraceMinutes: 5, GuestGraceDataMB: 50,
		GuestPlans:          []model.GuestPlan{{Key: "p1", Label: "1h", Minutes: 60, Amount: 500, DataMB: 1024, SpeedDownMbps: 10, SpeedUpMbps: 5}},
		VoucherLoginEnabled: true, MobilePaymentEnabled: true, UpdatedAt: time.Now(),
	}
}

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// A guest must never receive the tenant identifier or internal bookkeeping.
func TestPublicPortalViewOmitsTenantAndBookkeeping(t *testing.T) {
	keys := jsonKeys(t, publicPortalView(fullPortalConfig()))
	for _, k := range keys {
		if k == "business_id" || k == "updated_at" {
			t.Fatalf("public portal config exposes %q", k)
		}
	}
	// Fields the guest page really uses must survive.
	want := map[string]bool{"business_name": false, "guest_plans": false, "voucher_login_enabled": false, "media_host": false, "media_port": false, "bank_account": false}
	for _, k := range keys {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("public portal config lost %q needed by the guest page", k)
		}
	}
}

// Fails when a field is added to model.PortalConfig without deciding whether guests may see it: either copy it into
// publicPortalConfig, or add it to the withheld list here, deliberately.
func TestPublicPortalViewCoversEveryModelField(t *testing.T) {
	// template_id only styles the router login page; the hosted guest portal does not use it.
	withheld := map[string]bool{"business_id": true, "updated_at": true, "template_id": true}
	var wantKeys []string
	for _, k := range jsonKeys(t, fullPortalConfig()) {
		if !withheld[k] {
			wantKeys = append(wantKeys, k)
		}
	}
	got := jsonKeys(t, publicPortalView(fullPortalConfig()))
	if len(got) != len(wantKeys) {
		t.Fatalf("public view keys %v differ from model keys minus withheld %v", got, wantKeys)
	}
	for i := range got {
		if got[i] != wantKeys[i] {
			t.Fatalf("public view keys %v differ from model keys minus withheld %v", got, wantKeys)
		}
	}
}

func TestPublicPortalViewNil(t *testing.T) {
	if publicPortalView(nil) != nil {
		t.Fatal("nil config must stay nil")
	}
}
