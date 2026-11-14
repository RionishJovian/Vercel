package routeros

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const testTemplateDevice = "123e4567-e89b-12d3-a456-426614174000"

func templateDesign(id string) *PortalLoginDesign {
	return &PortalLoginDesign{
		BusinessName: "Magic Café", WelcomeText: "Welcome to our Wi-Fi", TermsText: "One voucher, one device.",
		PrimaryHex: "#38bdf8", GlassTintHex: "#121620", TemplateID: id, ConsentRequired: true,
		BankTransfer: true, DeviceID: testTemplateDevice,
		Plans: []PortalLoginPlan{{Label: "1 Hour WiFi", Minutes: 60, Amount: 1000}, {Label: "Day Pass", Minutes: 1440, Amount: 3500}},
	}
}

func relLum(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	n, _ := strconv.ParseUint(hex[:6], 16, 32)
	var l float64
	for i, w := range []float64{.2126, .7152, .0722} {
		c := float64((n>>uint(16-i*8))&255) / 255
		if c <= .04045 {
			c /= 12.92
		} else {
			c = math.Pow((c+.055)/1.055, 2.4)
		}
		l += c * w
	}
	return l
}

func contrast(a, b string) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + .05) / (lb + .05)
}

func TestPortalTemplateCatalogue(t *testing.T) {
	list := PortalTemplates()
	if len(list) != 9 {
		t.Fatalf("want 9 templates, got %d", len(list))
	}
	want := map[string]string{
		"starlink_mm": PortalGroupMyanmar, "cultural_mm": PortalGroupMyanmar, "mountain_mm": PortalGroupMyanmar, "village_mm": PortalGroupMyanmar,
		"obsidian": PortalGroupCommercial, "cafe": PortalGroupCommercial, "hotel": PortalGroupCommercial, "express": PortalGroupCommercial, "daylight": PortalGroupCommercial,
	}
	for _, tpl := range list {
		if want[tpl.ID] != tpl.Group || tpl.Name == "" {
			t.Fatalf("template %q has group %q name %q", tpl.ID, tpl.Group, tpl.Name)
		}
		delete(want, tpl.ID)
	}
	if len(want) != 0 {
		t.Fatalf("templates missing from the catalogue: %v", want)
	}
	if !ValidPortalTemplate("") || !ValidPortalTemplate("cafe") || ValidPortalTemplate("nope") || ValidPortalTemplate("cafe\"><script>") {
		t.Fatal("template id validation is wrong")
	}
	if _, err := RenderPortalLogin(&PortalLoginDesign{TemplateID: "nope"}, true); err == nil {
		t.Fatal("unknown template accepted")
	}
}

// An unset template must render exactly like the obsidian template, so portals saved before templates existed
// keep their look when the column default is applied.
func TestPortalTemplateDefaultsToObsidian(t *testing.T) {
	unset, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "A"}, true)
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "A", TemplateID: "obsidian"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if unset != chosen || !strings.Contains(unset, `class="card t-obsidian"`) {
		t.Fatal("an unset template must render as obsidian")
	}
}

var assetPattern = regexp.MustCompile(`(?i)<img\b|<link\b|<iframe\b|@import|@font-face|url\(\s*["']?(?:https?:|data:|/)|\bsrc\s*=\s*["']?(?:https?:|data:|//)`)

func TestPortalTemplatesRenderSelfContainedRouterLogin(t *testing.T) {
	required := []string{`$(link-login-only)`, `$(link-orig)`, `$(chap-id)`, `$(chap-challenge)`, `$(if error)`, `$(if chap-id)<script src="md5.js"></script>$(endif)`, `$(ip)`}
	for _, info := range PortalTemplates() {
		info := info
		t.Run(info.ID, func(t *testing.T) {
			page, err := RenderPortalLogin(templateDesign(info.ID), false)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) >= 24000 {
				t.Fatalf("%d bytes: over the router page budget", len(page))
			}
			if len(page) >= 32*1024 {
				t.Fatalf("%d bytes: does not fit the 32 KB limit", len(page))
			}
			for _, want := range required {
				if !strings.Contains(page, want) {
					t.Errorf("missing RouterOS variable %q", want)
				}
			}
			if m := assetPattern.FindString(page); m != "" {
				t.Errorf("external or non-inline asset: %q", m)
			}
			// The only outbound reference is the owner's bank transfer link to the hosted pay page.
			for _, u := range regexp.MustCompile(`https?://[^"'\s)]+`).FindAllString(page, -1) {
				if !strings.HasPrefix(u, "https://"+portalHost+"/portal.html?deviceId="+testTemplateDevice) {
					t.Errorf("unexpected URL %q", u)
				}
			}
			for _, srcURL := range regexp.MustCompile(`\bsrc="([^"]*)"`).FindAllStringSubmatch(page, -1) {
				if srcURL[1] != "md5.js" {
					t.Errorf("only the router-local md5.js may be loaded, found src=%q", srcURL[1])
				}
			}
			for _, tag := range []string{"main", "div", "svg", "g", "ul", "ol", "form", "details", "style", "script", "defs", "pattern"} {
				open := len(regexp.MustCompile(`<`+tag+`[\s>]`).FindAllString(page, -1))
				closed := strings.Count(page, "</"+tag+">")
				if open != closed {
					t.Errorf("<%s> opened %d times and closed %d", tag, open, closed)
				}
			}
			for _, want := range []string{`class="card t-` + info.ID + `"`, `id="lang-my"`, `id="lang-en"`, `မြန်မာ`, `>EN<`, `function setLang(`, `id="voucher"`, `id="voucher-consent"`, `id="bank-transfer"`} {
				if !strings.Contains(page, want) {
					t.Errorf("missing %q", want)
				}
			}
			preview, err := RenderPortalLogin(templateDesign(info.ID), true)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(preview, "$(") || !strings.Contains(preview, `data-preview="true"`) {
				t.Error("preview must have no live RouterOS macros and must be inert")
			}
		})
	}
}

func TestPortalTemplatesEscapeOwnerText(t *testing.T) {
	for _, info := range PortalTemplates() {
		d := templateDesign(info.ID)
		d.BusinessName = `Cafe <script>alert(1)</script> $(chap-id)`
		d.Plans[0].Label = `<b>$(ip)</b>`
		d.WelcomeText = `Hi $(mac) <i>`
		page, err := RenderPortalLogin(d, false)
		if err != nil {
			t.Fatalf("%s: %v", info.ID, err)
		}
		for _, banned := range []string{`<script>alert(1)`, `<b>$(ip)</b>`, `Hi $(mac)`, `Cafe <script>`} {
			if strings.Contains(page, banned) {
				t.Errorf("%s: owner text interpreted as markup or macro: %q", info.ID, banned)
			}
		}
	}
}

func TestPortalTemplateContrast(t *testing.T) {
	for _, info := range PortalTemplates() {
		tpl := portalTemplateByID(info.ID)
		if tpl.Pal == nil {
			continue // obsidian keeps the base palette, checked by the public-pages contrast test
		}
		p := tpl.Pal
		bad := "#b91c1c"
		if p.Dark {
			bad = "#f87171"
		}
		checks := []struct {
			name, fg, bg string
			min          float64
		}{
			{"ink on surface", p.Ink, p.Surface, 4.5}, {"soft on surface", p.Soft, p.Surface, 4.5}, {"muted on surface", p.Mute, p.Surface, 4.5},
			{"ink on page", p.Ink, p.Bg, 4.5}, {"soft on page", p.Soft, p.Bg, 4.5}, {"muted on page", p.Mute, p.Bg, 4.5},
			{"ink in field", p.Ink, p.Sunken, 4.5}, {"muted in field", p.Mute, p.Sunken, 4.5},
			{"accent text on surface", p.Accent, p.Surface, 4.5}, {"error on surface", bad, p.Surface, 4.5},
			{"field edge", p.Edge, p.Sunken, 3}, {"field edge on surface", p.Edge, p.Surface, 3},
			{"button text", portalBrandText(p.Brand), p.Brand, 4.5},
		}
		for _, c := range checks {
			if got := contrast(c.fg, c.bg); got < c.min {
				t.Errorf("%s: %s is %.2f:1, need %.1f:1 (%s on %s)", info.ID, c.name, got, c.min, c.fg, c.bg)
			}
		}
	}
}

func TestPortalTemplateSpecifics(t *testing.T) {
	render := func(id string, mutate func(*PortalLoginDesign)) string {
		d := templateDesign(id)
		if mutate != nil {
			mutate(d)
		}
		page, err := RenderPortalLogin(d, true)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	if p := render("starlink_mm", nil); !strings.Contains(p, "var BI=true") || !strings.Contains(p, `class="badge"`) {
		t.Error("starlink needs bilingual buttons and the link badge")
	}
	if p := render("cafe", nil); strings.Contains(p, "var BI=true") || strings.Contains(p, `class="tiers"`) {
		t.Error("only starlink is bilingual and only hotel shows tiers")
	}
	village := render("village_mm", nil)
	for _, want := range []string{`class="steps"`, "PREF='my'", "min-height:68px", "line-height:1.75", "min-height:64px"} {
		if !strings.Contains(village, want) {
			t.Errorf("village missing %q", want)
		}
	}
	hotel := render("hotel", nil)
	for _, want := range []string{`class="tiers"`, `1,000 Ks`, `3,500 Ks`, `1 h`, `1 d`, `border-top:6px solid var(--portal-brand)`} {
		if !strings.Contains(hotel, want) {
			t.Errorf("hotel missing %q", want)
		}
	}
	if p := render("hotel", func(d *PortalLoginDesign) { d.BankTransfer = false }); strings.Contains(p, `class="tiers"`) {
		t.Error("tier cards must not appear when bank transfer is off")
	}
	if p := render("hotel", func(d *PortalLoginDesign) { d.Plans = nil }); strings.Contains(p, `class="tiers"`) {
		t.Error("no plans means no tier cards")
	}
	if p := render("express", nil); !strings.Contains(p, "calc(8*1.55ch)") {
		t.Error("express needs the 8-box segmented voucher field")
	}
	if p := render("obsidian", nil); !strings.Contains(p, `class="bg-art"`) {
		t.Error("obsidian needs its contour background")
	}
	if p := render("cultural_mm", nil); !strings.Contains(p, "#2a0e12") || !strings.Contains(p, "#d4a338") {
		t.Error("cultural template lost its lacquer and gold")
	}
	if p := render("mountain_mm", nil); !strings.Contains(p, "#0a221e") || !strings.Contains(p, "#13332c") {
		t.Error("mountain template lost its pine palette")
	}
	if p := render("starlink_mm", nil); !strings.Contains(p, "#070d19") || !strings.Contains(p, "#0f1a2e") {
		t.Error("starlink template lost its navy palette")
	}
}

// MM_PORTAL_TEMPLATES_DIR makes this test write every deployed page so scripts/test-router-local-portal-ui.mjs can
// run its login-behaviour suite against each of the nine templates.
func TestPortalTemplatesWriteForBrowserSuite(t *testing.T) {
	dir := os.Getenv("MM_PORTAL_TEMPLATES_DIR")
	if dir == "" {
		t.Skip("MM_PORTAL_TEMPLATES_DIR not set")
	}
	for _, info := range PortalTemplates() {
		page, err := RenderPortalLogin(templateDesign(info.ID), false)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/"+info.ID+".html", []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
