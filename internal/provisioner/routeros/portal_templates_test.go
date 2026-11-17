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
		Plans:        []PortalLoginPlan{{Label: "1 Hour WiFi", Minutes: 60, Amount: 1000, DataMB: 1024}, {Label: "Day Pass", Minutes: 1440, Amount: 3500, SpeedDownMbps: 10}},
		SupportPhone: "+959 123 456 789", SupportTelegram: "@magiccafe", SupportViber: "+959123456789",
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

// An unset template renders as the obsidian template, so portals saved before templates existed keep their look
// when the column default is applied. Only the chosen obsidian (gaming) template carries its low-ping badges.
func TestPortalTemplateDefaultsToObsidian(t *testing.T) {
	unset, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "A"}, true)
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := RenderPortalLogin(&PortalLoginDesign{BusinessName: "A", TemplateID: "obsidian"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unset, `class="card t-obsidian"`) || strings.Contains(unset, `data-t="lp"`) || !strings.Contains(chosen, `data-t="lp"`) || !strings.Contains(chosen, `data-t="al"`) {
		t.Fatal("an unset template must render as obsidian without the gaming badges")
	}
	if strings.Replace(chosen, `<div class="bdg"><span><i></i><b data-t="lp">Low ping</b></span><span data-t="al">Anti-lag</span></div>`, "", 1) != unset {
		t.Fatal("the badges must be the only difference")
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
			if len(page) > portalLoginMaxBytes {
				t.Fatalf("%d bytes: over the router page budget", len(page))
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
			for _, tag := range []string{"main", "header", "nav", "div", "svg", "g", "ul", "ol", "li", "a", "span", "button", "form", "details", "style", "script", "defs", "pattern"} {
				open := len(regexp.MustCompile(`<`+tag+`[\s>]`).FindAllString(page, -1))
				closed := strings.Count(page, "</"+tag+">")
				if open != closed {
					t.Errorf("<%s> opened %d times and closed %d", tag, open, closed)
				}
			}
			for _, want := range []string{`class="card t-` + info.ID + `"`, `id="lang-my"`, `id="lang-en"`, `မြန်မာ`, `>EN<`, `function setLang(`, `id="voucher"`, `id="voucher-consent"`, `id="bank-transfer"`,
				`<header class="hero">`, `class="sc"`, `id="tab-v"`, `id="tab-m"`, `id="mu"`, `id="mp"`, `type="password"`, `id="qr"`, `Scan QR Voucher`,
				`<h2 data-t="pk">Internet Packages</h2>`, `class="pg"`, `<em data-t="bv">Best Value</em>`, `data-t="buy">Buy Now</a>`, `1,000 MMK`, `3,500 MMK`, `10 Mbps`, `1 GB`, `data-t="unl"`,
				`class="sup"`, `href="tel:+959123456789"`, `href="tg://resolve?domain=magiccafe"`, `href="viber://chat?number=%2B959123456789"`, `class="tr"`, `Family Safe`} {
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
	has := func(id, page string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(page, want) {
				t.Errorf("%s missing %q", id, want)
			}
		}
	}
	starlink := render("starlink_mm", nil)
	has("starlink_mm", starlink, "var BI=true", `data-t="link">High-Speed Satellite Internet`, `data-t="st">Online · Stable Link`, "#070d19", "#0f1a2e")
	if p := render("cafe", nil); strings.Contains(p, "var BI=true") {
		t.Error("only starlink is bilingual")
	}
	has("cultural_mm", render("cultural_mm", nil), "#2a0e12", "#d4a338", `id="pgd"`, `class="foot"`, "::before")
	has("mountain_mm", render("mountain_mm", nil), "#0a221e", "#13332c", `data-t="hl"`)
	has("village_mm", render("village_mm", nil), `class="steps"`, "PREF='my'", "min-height:68px", "line-height:1.75", "min-height:64px", `data-t="s1"`)
	has("obsidian", render("obsidian", nil), `class="bg-art"`, `data-t="lp">Low ping`, `data-t="al">Anti-lag`, `class="wifi-mark"`)
	has("cafe", render("cafe", nil), "border-radius:28px", "#6f3b1e")
	has("hotel", render("hotel", nil), `<span class="hw" data-t="wel">Welcome to</span><div class="hn brand">Magic Café</div>`, "#b08d3c", "#141b33")
	has("express", render("express", nil), "calc(8*1.55ch)", `data-t="fr">Fast &amp; Reliable Internet`, "#1d4ed8", `data-t="f1">Fast Speed`, `data-t="f2">24/7 Stable`, `data-t="f3">Family Safe`, `data-t="f4">Best Signal`)
	has("daylight", render("daylight", nil), "#bfdbfe", "#0a58ca")
	// The packages grid and its Buy Now buttons need the owner's payment flow; without it there is nothing to buy.
	if p := render("hotel", func(d *PortalLoginDesign) { d.BankTransfer = false }); strings.Contains(p, `class="pg"`) || strings.Contains(p, "Buy Now</a>") {
		t.Error("packages must not appear when bank transfer is off")
	}
	if p := render("hotel", func(d *PortalLoginDesign) { d.Plans = nil }); strings.Contains(p, `class="pg"`) {
		t.Error("no plans means no packages grid")
	}
	if p := render("daylight", func(d *PortalLoginDesign) { d.Plans = d.Plans[:1] }); strings.Contains(p, `data-t="bv"`) {
		t.Error("Best Value needs at least two priced plans to compare")
	}
	// Day Pass is 3,500 for 1,440 minutes, cheaper per minute than 1,000 for 60: it carries the tag.
	if p := render("daylight", nil); !strings.Contains(p, `<li class="bv"><em data-t="bv">Best Value</em><b>Day Pass</b>`) {
		t.Error("Best Value must mark the lowest price per minute")
	}
	// Contacts become links, so anything that is not a plain number or username is left out.
	bad := render("daylight", func(d *PortalLoginDesign) {
		d.SupportPhone, d.SupportTelegram, d.SupportViber = `1"><script>`, `a b$(ip)`, "javascript:alert(1)"
	})
	if strings.Contains(bad, `class="sup"`) || strings.Contains(bad, "javascript:") {
		t.Error("invalid support contacts must not be rendered")
	}
	if p := render("daylight", func(d *PortalLoginDesign) { d.SupportTelegram, d.SupportViber = "", "" }); !strings.Contains(p, `href="tel:+959123456789"`) || strings.Contains(p, "tg://") || strings.Contains(p, "viber://") {
		t.Error("only the contacts the owner filled in may show")
	}
}

// The hero text sits on the template's own gradient; HeroBg is the stop it is hardest to read on.
func TestPortalTemplateHeroContrast(t *testing.T) {
	for _, info := range PortalTemplates() {
		tpl := portalTemplateByID(info.ID)
		if tpl.Hero == "" {
			continue // obsidian keeps the base tokens: ink on bg, checked by the public-pages contrast test
		}
		if got := contrast(tpl.HeroInk, tpl.HeroBg); got < 4.5 {
			t.Errorf("%s: hero text is %.2f:1 on %s", info.ID, got, tpl.HeroBg)
		}
		if !strings.Contains(tpl.Hero, tpl.HeroBg) {
			t.Errorf("%s: HeroBg %s is not a stop of the hero background", info.ID, tpl.HeroBg)
		}
	}
}

func TestPortalStatusPageMatchesTheTemplate(t *testing.T) {
	required := []string{`$(username)`, `$(ip)`, `$(mac)`, `$(bytes-in-nice)`, `$(bytes-out-nice)`, `$(session-time-left)`, `$(if session-time-left)`, `action="$(link-logout)"`, `Logout / Disconnect`, `$(if refresh-timeout)`}
	for _, info := range PortalTemplates() {
		info := info
		t.Run(info.ID, func(t *testing.T) {
			d := templateDesign(info.ID)
			d.BusinessName = `Cafe <script>alert(1)</script> $(mac)`
			page, err := RenderPortalStatus(d, false)
			if err != nil {
				t.Fatal(err)
			}
			login, err := RenderPortalLogin(d, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) > portalStatusMaxBytes || len(page)+len(login) >= 45*1024 {
				t.Fatalf("status %d + login %d bytes: over the 45 KB pair budget", len(page), len(login))
			}
			for _, want := range append(required, `class="card t-`+info.ID+`"`, `<header class="hero">`, `class="tel"`, `id="lang-my"`, `class="tr"`, `href="tel:+959123456789"`) {
				if !strings.Contains(page, want) {
					t.Errorf("missing %q", want)
				}
			}
			if m := assetPattern.FindString(page); m != "" {
				t.Errorf("external or non-inline asset: %q", m)
			}
			if regexp.MustCompile(`https?://|\bsrc=`).MatchString(page) || strings.Contains(page, `Cafe <script>`) || strings.Contains(page, `</script> $(mac)`) {
				t.Error("status page must load nothing and must escape owner text")
			}
			// Same palette and hero as the login page.
			tpl := portalTemplateByID(info.ID)
			if !strings.Contains(page, tpl.Art) || (tpl.Pal != nil && !strings.Contains(page, tpl.Pal.css())) {
				t.Error("status page must use the login page's hero and palette")
			}
			for _, tag := range []string{"main", "header", "nav", "div", "svg", "g", "ul", "li", "dl", "dt", "dd", "form", "style", "script", "button"} {
				if open, closed := len(regexp.MustCompile(`<`+tag+`[\s>]`).FindAllString(page, -1)), strings.Count(page, "</"+tag+">"); open != closed {
					t.Errorf("<%s> opened %d times and closed %d", tag, open, closed)
				}
			}
			preview, err := RenderPortalStatus(d, true)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(preview, "$(") || !strings.Contains(preview, `data-t="smp"`) || !strings.Contains(preview, `onsubmit="return false"`) {
				t.Error("status preview must have no live macros, must label its sample session and must not log anyone out")
			}
		})
	}
}

// What the editor lets an owner type, all in Burmese (three bytes a letter), must still fit the router.
func TestPortalPagesFitWithTheLongestEditorText(t *testing.T) {
	my := func(n int) string { return strings.Repeat("က", n) }
	for _, info := range PortalTemplates() {
		d := templateDesign(info.ID)
		d.BusinessName, d.WelcomeText, d.TermsText, d.ConsentText = my(120), my(240), my(500), my(300)
		d.Plans = nil
		for i := 0; i < portalLoginMaxPlans; i++ {
			d.Plans = append(d.Plans, PortalLoginPlan{Label: my(40), Minutes: 60 * (i + 1), Amount: 1000 * (i + 1), DataMB: 1536, SpeedDownMbps: 10})
		}
		login, err := RenderPortalLogin(d, false)
		if err != nil {
			t.Fatalf("%s login: %v", info.ID, err)
		}
		status, err := RenderPortalStatus(d, false)
		if err != nil {
			t.Fatalf("%s status: %v", info.ID, err)
		}
		if len(login)+len(status) >= 45*1024 {
			t.Errorf("%s: %d + %d bytes is over 45 KB", info.ID, len(login), len(status))
		}
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
