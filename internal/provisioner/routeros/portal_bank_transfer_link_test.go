package routeros

import (
	"strings"
	"testing"
)

const testGatewayID = "683a4cee-97fb-4a88-a4b4-f142fdbdc78c"

func TestBankTransferLinkIsAddedOnlyWhenEnabled(t *testing.T) {
	off, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "Cafe"}, false)
	if err != nil || strings.Contains(off, "bank-transfer") {
		t.Fatalf("link must be absent when bank transfer is off: %v", err)
	}
	on, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "Cafe", BankTransfer: true, DeviceID: testGatewayID}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := `href="https://megamagic.tech/portal.html?deviceId=` + testGatewayID + `&amp;start=pay&amp;ip=$(ip)"`
	if !strings.Contains(on, want) || !strings.Contains(on, "Pay by bank transfer") {
		t.Fatalf("deployed page lacks the hosted link: %q", want)
	}
	if strings.Index(on, `id="bank-transfer"`) < strings.Index(on, `id="voucher-login"`) || strings.Index(on, `id="bank-transfer"`) > strings.Index(on, "</main>") {
		t.Fatal("link must sit after the voucher form and inside the card")
	}
}

func TestBankTransferLinkRejectsUnsafeGatewayIDAndStaysInertInPreview(t *testing.T) {
	for _, id := range []string{"", "x", `"><script>alert(1)</script>`, testGatewayID + "&evil=1"} {
		if _, err := RenderPortalLogin(&PortalLoginDesign{BankTransfer: true, DeviceID: id}, false); err == nil {
			t.Fatalf("gateway id %q must be refused for a deployed page", id)
		}
	}
	preview, err := RenderPortalLogin(&PortalLoginDesign{BankTransfer: true, DeviceID: "ignored"}, true)
	if err != nil || !strings.Contains(preview, `id="bank-transfer" href="#"`) || strings.Contains(preview, "megamagic.tech/portal.html") {
		t.Fatalf("preview link must be inert and not need a gateway id: %v", err)
	}
}
