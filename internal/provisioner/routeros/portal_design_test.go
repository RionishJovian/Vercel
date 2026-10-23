package routeros

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRouterPortalOfflineArtworkAndReferenceDesign(t *testing.T) {
	page, err := RenderPortalLogin(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<h1>Wi-Fi Access</h1>`, `Enter voucher code to connect`, `Connect to Wi-Fi`, `class="wifi-mark"`, `background-image:url("data:image/webp;base64,`, `data-preview="true"`, `prefers-reduced-transparency`, `prefers-reduced-motion`} {
		if !strings.Contains(page, want) {
			t.Fatalf("reference portal missing %q", want)
		}
	}
	if strings.Count(page, "data:image/webp;base64,") != 2 || strings.Contains(page, "https://") || strings.Contains(page, "http://") || len(page) > 24000 {
		t.Fatalf("portal must contain both local assets within its router budget: bytes=%d", len(page))
	}
	t.Logf("Complete offline glass portal: %d bytes", len(page))
	if path := os.Getenv("MM_ROUTER_PORTAL_DESIGN_FILE"); path != "" {
		if err := os.WriteFile(path, []byte(page), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRouterPortalDesignEscapesOwnerTextAndKeepsMacrosOwned(t *testing.T) {
	design := &PortalLoginDesign{BusinessName: `Cafe <script>alert(1)</script> $(chap-id)`, WelcomeText: `Welcome & connect`, TermsText: `<img src=x onerror=alert(1)>`, PrimaryHex: "#00745e", GlassTintHex: "#edf4f7", ConsentRequired: true, ConsentText: `I agree <now>`}
	page, err := RenderPortalLogin(design, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`Cafe <script>`, `<img src=x`, `I agree <now>`, `Cafe &lt;script&gt;alert(1)&lt;/script&gt; $(chap-id)`} {
		if strings.Contains(page, banned) {
			t.Fatalf("owner input interpreted as markup/macro: %s", banned)
		}
	}
	for _, want := range []string{`&#36;(chap-id)`, `Welcome &amp; connect`, `id="voucher-consent"`, `Accept the internet access terms`, `hexMD5(chapId+code+'$(chap-challenge)')`, `--portal-on-brand:#fff`} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing %q", want)
		}
	}
	preview, err := RenderPortalLogin(design, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview, `$(`) || strings.Contains(preview, `src="md5.js"`) || !strings.Contains(preview, `data-preview="true"`) {
		t.Fatal("preview retains live macros/helper or lacks inert marker")
	}
	if _, err := RenderPortalLogin(&PortalLoginDesign{PrimaryHex: `red;}</style><script>bad()</script>`}, false); err == nil {
		t.Fatal("accepted CSS injection")
	}
	if _, err := RenderPortalLogin(&PortalLoginDesign{TermsText: strings.Repeat("界", 2001)}, false); err == nil {
		t.Fatal("unbounded router document")
	}
	for _, c := range []string{"#fff", "#ffffff", "#ffb547", "#fff0"} {
		if portalBrandText(c) != "#102c29" {
			t.Fatalf("light brand needs dark button text: %s", c)
		}
	}
}

func TestRouterPortalDeploymentBindsSavedDesignToReview(t *testing.T) {
	r := newPortalRunner()
	a := portalTestApplier(r)
	design := &PortalLoginDesign{BusinessName: "Magic Cafe", WelcomeText: "Saved welcome", PrimaryHex: "#00745e", GlassTintHex: "#edf4f7"}
	plan, err := a.PreviewPortal(context.Background(), "biz", "dev", PortalDeployment{Profile: "guest", Design: design})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.PreviewHTML, "Magic Cafe") || !strings.Contains(plan.PreviewHTML, `data-preview="true"`) {
		t.Fatal("review must show inert exact saved design")
	}
	changed := *design
	changed.WelcomeText = "Changed after review"
	_, err = a.ApplyPortal(context.Background(), "biz", "dev", PortalDeployment{Profile: plan.Profile, Version: plan.Version, Confirmation: plan.Confirmation, Design: &changed})
	if err == nil {
		t.Fatal("changed branding reused old confirmation")
	}
	if portalHasWrites(r.commands) {
		t.Fatal("changed design wrote router settings")
	}
	result, err := a.ApplyPortal(context.Background(), "biz", "dev", PortalDeployment{Profile: plan.Profile, Version: plan.Version, Confirmation: plan.Confirmation, Design: design})
	if err != nil || !result.Verified {
		t.Fatalf("unchanged saved design failed verified apply: %v %+v", err, result)
	}
	rendered, _ := RenderPortalLogin(design, false)
	if r.files[result.Plan.File] != rendered {
		t.Fatal("installed document differs from saved renderer")
	}
}
