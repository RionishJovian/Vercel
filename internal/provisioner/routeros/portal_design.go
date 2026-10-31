package routeros

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"mega-magic/internal/model"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Generated reference-matched artwork is bundled in the document itself. The
// guest therefore needs neither WAN access nor an external asset allowlist.
//
//go:embed assets/jade-portal-wallpaper.webp
var portalWallpaper []byte

//go:embed assets/jade-portal-wifi.webp
var portalWiFiMark []byte

var portalWallpaperURL = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(portalWallpaper)
var portalWiFiMarkURL = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(portalWiFiMark)

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
}

func RouterPortalDesign(c *model.PortalConfig) *PortalLoginDesign {
	if c == nil {
		return nil
	}
	return &PortalLoginDesign{BusinessName: c.BusinessName, WelcomeText: c.WelcomeText, TermsText: c.TermsText, PrimaryHex: c.PrimaryHex, GlassTintHex: c.GlassTintHex, ConsentRequired: c.ConsentRequired, ConsentText: c.ConsentText}
}

// Escape both HTML and RouterOS macro openers: owner text must never turn into
// a RouterOS servlet expression when the generated file is served later.
func portalDesignText(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "$(", "&#36;(")
}

// RenderPortalLogin supplies the same document for owner preview and deployment.
// Preview removes all RouterOS macros/resources and makes login explicitly inert.
func RenderPortalLogin(design *PortalLoginDesign, preview bool) (string, error) {
	d := PortalLoginDesign{BusinessName: "WiFi Voucher", WelcomeText: "Enter voucher code to connect", PrimaryHex: "#075044", GlassTintHex: "#061f26"}
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
		d.PrimaryHex = "#075044"
	}
	if d.GlassTintHex == "" {
		d.GlassTintHex = "#061f26"
	}
	if !model.IsHexColor(d.PrimaryHex) || !model.IsHexColor(d.GlassTintHex) {
		return "", fmt.Errorf("portal colours must be literal hex colours")
	}
	if utf8.RuneCountInString(d.BusinessName) > 200 || utf8.RuneCountInString(d.WelcomeText) > 1000 || utf8.RuneCountInString(d.TermsText) > 2000 || utf8.RuneCountInString(d.ConsentText) > 300 {
		return "", fmt.Errorf("portal branding is too long for a router login page")
	}
	page := portalLoginHTML()
	page = strings.Replace(page, `<main class="card">`, `<main class="card"><img class="wifi-mark" src="`+portalWiFiMarkURL+`" alt="" width="78" height="66">`, 1)
	page = strings.Replace(page, "<p>Enter voucher code to connect</p>", "<p>"+portalDesignText(d.WelcomeText)+"</p>", 1)
	if d.BankTransfer {
		href := "#"
		if !preview {
			if !guestRequestIDPattern.MatchString(d.DeviceID) {
				return "", fmt.Errorf("a valid gateway id is required for the bank transfer link")
			}
			href = "https://" + portalHost + "/portal.html?deviceId=" + d.DeviceID + "&amp;start=pay&amp;ip=$(ip)"
		}
		page = strings.Replace(page, "</form>\n</main>", "</form>\n<a class=\"alt\" id=\"bank-transfer\" href=\""+href+"\">Pay by bank transfer</a>\n</main>", 1)
	}
	if d.BusinessName != "WiFi Voucher" {
		page = strings.Replace(page, "</main>", `<span class="brand">`+portalDesignText(d.BusinessName)+`</span></main>`, 1)
	}
	css := `<style>:root{--portal-brand:` + d.PrimaryHex + `;--portal-tint:` + d.GlassTintHex + `;--portal-on-brand:` + portalBrandText(d.PrimaryHex) + `}body{background-image:url("` + portalWallpaperURL + `")}.card{background-color:color-mix(in srgb,var(--portal-tint) 7%,#08252b77)}</style>`
	page = strings.Replace(page, "</head>", css+"</head>", 1)
	if d.TermsText != "" {
		page = strings.Replace(page, "<form id=\"voucher-login\"", `<details><summary>Internet access terms</summary><p>`+portalDesignText(d.TermsText)+`</p></details><form id="voucher-login"`, 1)
	}
	if d.ConsentRequired {
		text := d.ConsentText
		if text == "" {
			text = "I agree to the internet access terms."
		}
		page = strings.Replace(page, `<button id="connect"`, `<label class="consent"><input id="voucher-consent" type="checkbox" required><span>`+portalDesignText(text)+`</span></label><button id="connect"`, 1)
	}
	if preview {
		page = strings.Replace(page, "<body>", `<body data-preview="true">`, 1)
		page = strings.ReplaceAll(page, `$(if chap-id)<script src="md5.js"></script>$(endif)`, "")
		page = strings.ReplaceAll(page, `$(if error)<p class="err">$(error)</p>$(endif)`, "")
		page = strings.NewReplacer("$(link-login-only)", "#", "$(link-orig)", "#", "$(chap-id)", "", "$(chap-challenge)", "").Replace(page)
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
	if luminance > .22 {
		return "#102c29"
	}
	return "#fff"
}
