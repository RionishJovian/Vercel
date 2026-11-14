package routeros

import (
	"fmt"
	"html"
	"math"
	"mega-magic/internal/model"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The router serves this page before the guest has internet, so it carries no image and no web font: the Wi-Fi
// mark is inline SVG and the type is the phone's own. Obsidian and Cyan, owner decision 2026-11-13.
const portalWiFiMarkSVG = `<svg class="wifi-mark" viewBox="0 0 24 24" width="40" height="40" aria-hidden="true" focusable="false"><path d="M4 9.5a11.5 11.5 0 0 1 16 0M7 12.6a7.2 7.2 0 0 1 10 0M9.8 15.6a3 3 0 0 1 4.4 0M12 18.6h.01" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`

// PortalLoginDesign is the router-safe subset of saved portal settings. It has
// no payment credentials, external resources, media access or arbitrary markup.
type PortalLoginDesign struct {
	BusinessName    string `json:"business_name"`
	WelcomeText     string `json:"welcome_text"`
	TermsText       string `json:"terms_text"`
	PrimaryHex      string `json:"primary_hex"`
	GlassTintHex    string `json:"glass_tint_hex"`
	ConsentRequired bool   `json:"consent_required"`
	ConsentText     string `json:"consent_text"`
	// BankTransfer adds a "Pay by bank transfer" link to the hosted page, which
	// holds the owner's own account details. DeviceID is set by the server from
	// the gateway being deployed (never from a request body) and must be a UUID.
	BankTransfer bool   `json:"mobile_payment_enabled"`
	DeviceID     string `json:"-"`
	// TemplateID picks one of the PortalTemplates. Empty means DefaultPortalTemplate.
	TemplateID string `json:"template_id"`
	// Plans feeds the guest-pass tier cards of the hotel template. It is display text only.
	Plans []PortalLoginPlan `json:"plans,omitempty"`
}

// PortalLoginPlan is the display-only part of a guest plan (never a key, price rule or limit).
type PortalLoginPlan struct {
	Label   string `json:"label"`
	Minutes int    `json:"minutes"`
	Amount  int    `json:"amount"`
}

const portalLoginMaxPlans = 4

func RouterPortalDesign(c *model.PortalConfig) *PortalLoginDesign {
	if c == nil {
		return nil
	}
	d := &PortalLoginDesign{BusinessName: c.BusinessName, WelcomeText: c.WelcomeText, TermsText: c.TermsText, PrimaryHex: c.PrimaryHex, GlassTintHex: c.GlassTintHex, ConsentRequired: c.ConsentRequired, ConsentText: c.ConsentText, TemplateID: c.TemplateID}
	for _, p := range c.GuestPlans {
		if len(d.Plans) == portalLoginMaxPlans {
			break
		}
		d.Plans = append(d.Plans, PortalLoginPlan{Label: p.Label, Minutes: p.Minutes, Amount: p.Amount})
	}
	return d
}

func portalTiersHTML(plans []PortalLoginPlan) string {
	var b strings.Builder
	b.WriteString(`<ul class="tiers" aria-label="Guest passes">`)
	for i, p := range plans {
		if i == portalLoginMaxPlans {
			break
		}
		label := strings.TrimSpace(p.Label)
		if label == "" || p.Amount < 0 || p.Minutes < 0 {
			continue
		}
		if r := []rune(label); len(r) > 40 {
			label = string(r[:40])
		}
		b.WriteString(`<li><b>` + portalDesignText(label) + `</b><span class="amt">` + portalAmount(p.Amount) + ` Ks</span><span>` + portalDuration(p.Minutes) + `</span></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// Escape both HTML and RouterOS macro openers: owner text must never turn into
// a RouterOS servlet expression when the generated file is served later.
func portalDesignText(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "$(", "&#36;(")
}

// RenderPortalLogin supplies the same document for owner preview and deployment.
// Preview removes all RouterOS macros/resources and makes login explicitly inert.
func RenderPortalLogin(design *PortalLoginDesign, preview bool) (string, error) {
	d := PortalLoginDesign{BusinessName: "WiFi Voucher", WelcomeText: "Enter voucher code to connect", PrimaryHex: "#38bdf8", GlassTintHex: "#121620"}
	if design != nil {
		d = *design
	}
	if strings.TrimSpace(d.BusinessName) == "" {
		d.BusinessName = "WiFi Voucher"
	}
	if d.WelcomeText == "" {
		d.WelcomeText = "Enter voucher code to connect"
	}
	if d.PrimaryHex == "" {
		d.PrimaryHex = "#38bdf8"
	}
	if d.GlassTintHex == "" {
		d.GlassTintHex = "#121620"
	}
	if !model.IsHexColor(d.PrimaryHex) || !model.IsHexColor(d.GlassTintHex) {
		return "", fmt.Errorf("portal colours must be literal hex colours")
	}
	if !ValidPortalTemplate(d.TemplateID) {
		return "", fmt.Errorf("unknown portal template")
	}
	if utf8.RuneCountInString(d.BusinessName) > 200 || utf8.RuneCountInString(d.WelcomeText) > 1000 || utf8.RuneCountInString(d.TermsText) > 2000 || utf8.RuneCountInString(d.ConsentText) > 300 {
		return "", fmt.Errorf("portal branding is too long for a router login page")
	}
	tpl := portalTemplateByID(d.TemplateID)
	page := portalLoginHTML()
	toggle := `<div class="lang" role="group" aria-label="Language"><button type="button" id="lang-my" lang="my" aria-pressed="false" onclick="setLang('my')">မြန်မာ</button><button type="button" id="lang-en" lang="en" aria-pressed="true" onclick="setLang('en')">EN</button></div>`
	page = strings.Replace(page, `<main class="card">`, `<main class="card t-`+tpl.ID+`">`+toggle+tpl.Art, 1)
	if tpl.BG != "" {
		page = strings.Replace(page, "<body>", "<body>"+tpl.BG, 1)
	}
	tplCSS := ""
	if tpl.Pal != nil {
		tplCSS = tpl.Pal.css() + portalTemplateBaseCSS
	}
	page = strings.Replace(page, "</head>", "<style>"+tplCSS+tpl.CSS+"</style></head>", 1)
	if tpl.Bilingual {
		page = strings.Replace(page, "var BI=false", "var BI=true", 1)
	}
	if tpl.PreferMy {
		page = strings.Replace(page, "PREF=''", "PREF='my'", 1)
	}
	if d.WelcomeText != "Enter voucher code to connect" {
		page = strings.Replace(page, `<p data-t="welcome">Enter voucher code to connect</p>`, "<p>"+portalDesignText(d.WelcomeText)+"</p>", 1)
	}
	if tpl.Steps {
		page = strings.Replace(page, "<form id=\"voucher-login\"", `<ol class="steps"><li data-t="s1">Type your voucher code</li><li data-t="s2">Tap Connect</li><li data-t="s3">You are online</li></ol><form id="voucher-login"`, 1)
	}
	if tpl.Tiers && d.BankTransfer && len(d.Plans) > 0 {
		page = strings.Replace(page, "<form id=\"voucher-login\"", portalTiersHTML(d.Plans)+`<form id="voucher-login"`, 1)
	}
	if d.BankTransfer {
		href := "#"
		if !preview {
			if !guestRequestIDPattern.MatchString(d.DeviceID) {
				return "", fmt.Errorf("a valid gateway id is required for the bank transfer link")
			}
			href = "https://" + portalHost + "/portal.html?deviceId=" + d.DeviceID + "&amp;start=pay&amp;ip=$(ip)"
		}
		page = strings.Replace(page, "</form>\n</main>", "</form>\n<a class=\"alt\" id=\"bank-transfer\" href=\""+href+"\" data-t=\"pay\">Pay by bank transfer</a>\n</main>", 1)
	}
	if d.BusinessName != "WiFi Voucher" {
		page = strings.Replace(page, "</main>", `<span class="brand">`+portalDesignText(d.BusinessName)+`</span></main>`, 1)
	}
	if tpl.Foot != "" {
		page = strings.Replace(page, "</main>", tpl.Foot+"</main>", 1)
	}
	css := `<style>:root{--portal-brand:` + d.PrimaryHex + `;--portal-tint:` + d.GlassTintHex + `;--portal-on-brand:` + portalBrandText(d.PrimaryHex) + `}</style>`
	page = strings.Replace(page, "</head>", css+"</head>", 1)
	if d.TermsText != "" {
		page = strings.Replace(page, "<form id=\"voucher-login\"", `<details><summary data-t="terms">Internet access terms</summary><p>`+portalDesignText(d.TermsText)+`</p></details><form id="voucher-login"`, 1)
	}
	if d.ConsentRequired {
		text, attr := d.ConsentText, ""
		if text == "" {
			text, attr = "I agree to the internet access terms.", ` data-t="consent"`
		}
		page = strings.Replace(page, `<button id="connect"`, `<label class="consent"><input id="voucher-consent" type="checkbox" required><span`+attr+`>`+portalDesignText(text)+`</span></label><button id="connect"`, 1)
	}
	if preview {
		page = strings.Replace(page, "<body>", `<body data-preview="true">`, 1)
		page = strings.ReplaceAll(page, `$(if chap-id)<script src="md5.js"></script>$(endif)`, "")
		page = strings.ReplaceAll(page, `$(if error)<p class="err">$(error)</p>$(endif)`, "")
		page = strings.NewReplacer("$(link-login-only)", "#", "$(link-orig)", "#", "$(chap-id)", "", "$(chap-challenge)", "").Replace(page)
		if d.BankTransfer {
			// A "#" link inside the console's srcdoc preview frame would load the console itself in the phone.
			// Keep it inert and tell the console, which switches its preview to the bank transfer page.
			page = strings.Replace(page, "</body>", `<script>(function(){var a=document.getElementById("bank-transfer");if(a){a.addEventListener("click",function(e){e.preventDefault();parent.postMessage({type:"mm-portal-preview-pay"},"*");});}})();</script></body>`, 1)
		}
	}
	if len(page) > 24000 {
		return "", fmt.Errorf("portal login page exceeds router size limit")
	}
	return page, nil
}

func portalBrandText(hex string) string {
	value := strings.TrimPrefix(hex, "#")
	if len(value) == 3 || len(value) == 4 {
		var b strings.Builder
		for _, c := range value {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		value = b.String()
	}
	if len(value) == 6 {
		value += "ff"
	}
	n, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return "#fff"
	}
	alpha := float64(n&255) / 255
	var luminance float64
	for i, weight := range []float64{.2126, .7152, .0722} {
		channel := (float64((n>>uint(24-i*8))&255)*alpha + 255*(1-alpha)) / 255
		if channel <= .04045 {
			channel /= 12.92
		} else {
			channel = math.Pow((channel+.055)/1.055, 2.4)
		}
		luminance += channel * weight
	}
	if luminance > .19 {
		return "#06131f"
	}
	return "#fff"
}
