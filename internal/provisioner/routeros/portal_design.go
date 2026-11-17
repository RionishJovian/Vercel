package routeros

import (
	"fmt"
	"html"
	"math"
	"mega-magic/internal/model"
	"regexp"
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
	// Plans feeds the Internet Packages grid. It is display text only.
	Plans []PortalLoginPlan `json:"plans,omitempty"`
	// Support contacts for the bottom support bar. Each is validated again at render time and left out when it
	// is not a plain phone number or Telegram username, because it becomes part of a link.
	SupportPhone    string `json:"support_phone"`
	SupportTelegram string `json:"support_telegram"`
	SupportViber    string `json:"support_viber"`
}

// PortalLoginPlan is the display-only part of a guest plan (never a key, price rule or limit).
type PortalLoginPlan struct {
	Label         string `json:"label"`
	Minutes       int    `json:"minutes"`
	Amount        int    `json:"amount"`
	DataMB        int    `json:"data_mb,omitempty"`
	SpeedDownMbps int    `json:"speed_down_mbps,omitempty"`
}

var (
	portalPhonePattern    = regexp.MustCompile(`^\+?[0-9]{6,15}$`)
	portalTelegramPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)
)

// NormalizeSupportPhone strips spaces, dashes and brackets and reports whether what is left is a dialable number.
func NormalizeSupportPhone(raw string) (string, bool) {
	v := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(raw))
	return v, v == "" || portalPhonePattern.MatchString(v)
}

// NormalizeSupportTelegram drops a leading @ and reports whether what is left is a Telegram username.
func NormalizeSupportTelegram(raw string) (string, bool) {
	v := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	return v, v == "" || portalTelegramPattern.MatchString(v)
}

const portalLoginMaxPlans = 4

func RouterPortalDesign(c *model.PortalConfig) *PortalLoginDesign {
	if c == nil {
		return nil
	}
	d := &PortalLoginDesign{BusinessName: c.BusinessName, WelcomeText: c.WelcomeText, TermsText: c.TermsText, PrimaryHex: c.PrimaryHex, GlassTintHex: c.GlassTintHex, ConsentRequired: c.ConsentRequired, ConsentText: c.ConsentText, TemplateID: c.TemplateID,
		SupportPhone: c.SupportPhone, SupportTelegram: c.SupportTelegram, SupportViber: c.SupportViber}
	for _, p := range c.GuestPlans {
		if len(d.Plans) == portalLoginMaxPlans {
			break
		}
		d.Plans = append(d.Plans, PortalLoginPlan{Label: p.Label, Minutes: p.Minutes, Amount: p.Amount, DataMB: p.DataMB, SpeedDownMbps: p.SpeedDownMbps})
	}
	return d
}

// portalPlansHTML is the two-column Internet Packages grid. Every Buy Now button opens the hosted pay page (href),
// where the guest pays the owner's own KBZPay, Wave or bank account and waits for the owner's approval.
// "Best Value" marks the lowest price per minute and only appears when there are at least two priced plans.
func portalPlansHTML(plans []PortalLoginPlan, href string) string {
	shown := make([]PortalLoginPlan, 0, portalLoginMaxPlans)
	for _, p := range plans {
		p.Label = strings.TrimSpace(p.Label)
		if len(shown) == portalLoginMaxPlans || p.Label == "" || p.Amount < 0 || p.Minutes < 0 {
			continue
		}
		if r := []rune(p.Label); len(r) > 40 {
			p.Label = string(r[:40])
		}
		shown = append(shown, p)
	}
	if len(shown) == 0 {
		return ""
	}
	best, priced, rate := -1, 0, 0.0
	for i, p := range shown {
		if p.Minutes > 0 && p.Amount > 0 {
			priced++
			if r := float64(p.Amount) / float64(p.Minutes); best < 0 || r < rate {
				best, rate = i, r
			}
		}
	}
	if priced < 2 {
		best = -1
	}
	var b strings.Builder
	b.WriteString(`<h2 data-t="pk">Internet Packages</h2><ul class="pg">`)
	for i, p := range shown {
		if i == best {
			b.WriteString(`<li class="bv"><em data-t="bv">Best Value</em>`)
		} else {
			b.WriteString(`<li>`)
		}
		speed := `<span data-t="unl">Unlimited</span>`
		if p.SpeedDownMbps > 0 {
			speed = strconv.Itoa(p.SpeedDownMbps) + " Mbps"
		}
		if p.DataMB > 0 {
			speed += " · " + portalData(p.DataMB)
		}
		b.WriteString(`<b>` + portalDesignText(p.Label) + `</b><span class="d">` + portalDuration(p.Minutes) + `</span><span class="s">` + speed + `</span><strong>` + portalAmount(p.Amount) + ` MMK</strong><a class="pay" href="` + href + `" data-t="buy">Buy Now</a></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

const (
	portalIconPhone    = `<svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z"/></svg>`
	portalIconTelegram = `<svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M21 4 3 11l6 2 2 6 3-4 4 3zM9 13l7-5"/></svg>`
	portalIconViber    = `<svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M4 5h16v11H10l-5 4v-4H4zM9 9c0 3 2 5 5 5"/></svg>`
	// The four-icon feature strip under every template.
	portalTrustHTML = `<ul class="tr"><li><svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M13 2 4 14h7l-1 8 9-12h-7z"/></svg><span data-t="f1">Fast Speed</span></li>` +
		`<li><svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg><span data-t="f2">24/7 Stable</span></li>` +
		`<li><svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6zM9 12l2 2 4-4"/></svg><span data-t="f3">Family Safe</span></li>` +
		`<li><svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M4 20v-4M9 20v-8M14 20V8M19 20V4"/></svg><span data-t="f4">Best Signal</span></li></ul>`
)

// portalSupportHTML is the bottom support bar. Phone opens the dialler; Telegram and Viber open their apps by deep
// link, because a guest who has not logged in yet cannot load a web link.
func portalSupportHTML(d *PortalLoginDesign) string {
	var b strings.Builder
	if v, ok := NormalizeSupportPhone(d.SupportPhone); ok && v != "" {
		b.WriteString(`<a href="tel:` + v + `">` + portalIconPhone + `<span data-t="call">Call</span></a>`)
	}
	if v, ok := NormalizeSupportTelegram(d.SupportTelegram); ok && v != "" {
		b.WriteString(`<a href="tg://resolve?domain=` + v + `">` + portalIconTelegram + `Telegram</a>`)
	}
	if v, ok := NormalizeSupportViber(d.SupportViber); ok && v != "" {
		b.WriteString(`<a href="viber://chat?number=` + strings.Replace(v, "+", "%2B", 1) + `">` + portalIconViber + `Viber</a>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<nav class="sup" aria-label="Support">` + b.String() + `</nav>`
}

// NormalizeSupportViber accepts the same phone format; Viber chats are addressed by number.
func NormalizeSupportViber(raw string) (string, bool) { return NormalizeSupportPhone(raw) }

const portalLangToggle = `<div class="lang" role="group" aria-label="Language"><button type="button" id="lang-my" lang="my" aria-pressed="false" onclick="setLang('my')">မြန်မာ</button><button type="button" id="lang-en" lang="en" aria-pressed="true" onclick="setLang('en')">EN</button></div>`

// portalDefaults fills the unset fields and validates the design shared by the login and status pages.
func portalDefaults(design *PortalLoginDesign) (PortalLoginDesign, error) {
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
		return d, fmt.Errorf("portal colours must be literal hex colours")
	}
	if !ValidPortalTemplate(d.TemplateID) {
		return d, fmt.Errorf("unknown portal template")
	}
	if utf8.RuneCountInString(d.BusinessName) > 200 || utf8.RuneCountInString(d.WelcomeText) > 1000 || utf8.RuneCountInString(d.TermsText) > 2000 || utf8.RuneCountInString(d.ConsentText) > 300 {
		return d, fmt.Errorf("portal branding is too long for a router login page")
	}
	return d, nil
}

// portalThemed puts the template's hero, palette, extra strings and brand colours into a page built on the shared
// shell (<header class="hero"></header><main class="card">), so login.html and status.html always match.
func portalThemed(page string, d PortalLoginDesign, tpl portalTemplate) string {
	hero := portalLangToggle + tpl.Art
	// A portal that never chose a template renders as obsidian but without its gaming badges.
	if tpl.Badges != "" && (tpl.ID != DefaultPortalTemplate || d.TemplateID != "") {
		hero += `<div class="bdg">` + tpl.Badges + `</div>`
	}
	if d.BusinessName != "WiFi Voucher" {
		if tpl.Welcome {
			hero += `<span class="hw" data-t="wel">Welcome to</span>`
		}
		hero += `<div class="hn brand">` + portalDesignText(d.BusinessName) + `</div>`
	}
	page = strings.Replace(page, `<header class="hero"></header><main class="card">`, `<header class="hero">`+hero+`</header><main class="card t-`+tpl.ID+`">`, 1)
	if tpl.BG != "" {
		page = strings.Replace(page, "<body>", "<body>"+tpl.BG, 1)
	}
	css := ""
	if tpl.Pal != nil {
		css = tpl.Pal.css()
	}
	if tpl.Hero != "" {
		css += ".hero{background:" + tpl.Hero + ";color:" + tpl.HeroInk + "}"
	}
	css += tpl.CSS + `:root{--portal-brand:` + d.PrimaryHex + `;--portal-tint:` + d.GlassTintHex + `;--portal-on-brand:` + portalBrandText(d.PrimaryHex) + `}`
	page = strings.Replace(page, "</head>", "<style>"+css+"</style></head>", 1)
	if tpl.En != "" {
		page = strings.Replace(page, "var T={en:{", "var T={en:{"+tpl.En+",", 1)
		page = strings.Replace(page, "\nmy:{", "\nmy:{"+tpl.My+",", 1)
	}
	if tpl.PreferMy {
		page = strings.Replace(page, "PREF=''", "PREF='my'", 1)
	}
	if tpl.Foot != "" {
		page = strings.Replace(page, "</main>", tpl.Foot+"</main>", 1)
	}
	return strings.Replace(page, "</main>", "</main>"+portalSupportHTML(&d)+portalTrustHTML, 1)
}

// Inert links for the console's srcdoc preview frame: a "#" or app link there would navigate the phone frame.
// Pay links tell the console instead, which switches its preview to the pay page.
func portalPreviewLinks(pay bool) string {
	tell := ""
	if pay {
		tell = `if(/\bpay\b/.test(this.className)){parent.postMessage({type:"mm-portal-preview-pay"},"*");}`
	}
	return `<script>(function(){var a=document.querySelectorAll("a");for(var i=0;i<a.length;i++){a[i].addEventListener("click",function(e){e.preventDefault();` + tell + `});}})();</script>`
}

// Escape both HTML and RouterOS macro openers: owner text must never turn into
// a RouterOS servlet expression when the generated file is served later.
func portalDesignText(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "$(", "&#36;(")
}

// RenderPortalLogin supplies the same document for owner preview and deployment.
// Preview removes all RouterOS macros/resources and makes login explicitly inert.
func RenderPortalLogin(design *PortalLoginDesign, preview bool) (string, error) {
	d, err := portalDefaults(design)
	if err != nil {
		return "", err
	}
	tpl := portalTemplateByID(d.TemplateID)
	page := portalThemed(portalLoginHTML(), d, tpl)
	if tpl.Bilingual {
		page = strings.Replace(page, "var BI=false", "var BI=true", 1)
	}
	if d.WelcomeText != "Enter voucher code to connect" {
		page = strings.Replace(page, `<p data-t="welcome">Enter voucher code to connect</p>`, "<p>"+portalDesignText(d.WelcomeText)+"</p>", 1)
	}
	const tabs = `<div class="tabs"`
	if tpl.Steps {
		page = strings.Replace(page, tabs, `<ol class="steps"><li data-t="s1">Type your voucher code</li><li data-t="s2">Tap Connect</li><li data-t="s3">You are online</li></ol>`+tabs, 1)
	}
	if d.BankTransfer {
		href := "#"
		if !preview {
			if !guestRequestIDPattern.MatchString(d.DeviceID) {
				return "", fmt.Errorf("a valid gateway id is required for the bank transfer link")
			}
			href = "https://" + portalHost + "/portal.html?deviceId=" + d.DeviceID + "&amp;start=pay&amp;ip=$(ip)"
		}
		// After the visible login form, which is the last form in the card (a template's footer art may follow it).
		at := strings.LastIndex(page, "</form>\n") + len("</form>\n")
		page = page[:at] + "<a class=\"alt pay\" id=\"bank-transfer\" href=\"" + href + "\" data-t=\"pay\">Pay by bank transfer</a>\n" + page[at:]
		page = strings.Replace(page, "</main>", "</main>"+portalPlansHTML(d.Plans, href), 1)
	}
	if d.TermsText != "" {
		page = strings.Replace(page, tabs, `<details><summary data-t="terms">Internet access terms</summary><p>`+portalDesignText(d.TermsText)+`</p></details>`+tabs, 1)
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
		page = strings.Replace(page, "</body>", portalPreviewLinks(d.BankTransfer)+"</body>", 1)
	}
	if len(page) > portalLoginMaxBytes {
		return "", fmt.Errorf("portal login page exceeds router size limit")
	}
	return page, nil
}

// Router page budgets. RouterOS reads and writes a file's contents up to just under 60 KiB (61439 bytes, RouterOS
// manual, Files), and the deployment reads each page back to verify it. The pair stays under 45 KiB.
const (
	portalLoginMaxBytes  = 30000
	portalStatusMaxBytes = 16000
)

// portalStatusHTML is the body of the router-served status.html: the session the guest is in, read from RouterOS
// $(...) variables, and a logout form. It reuses the login page's stylesheet so the two always look alike.
const portalStatusBody = `<body><div class="wrap"><header class="hero"></header><main class="card">
<div class="bdg"><span><i></i><b data-t="on">Connected</b></span></div>
<h1 data-t="title">You are online</h1>
<dl class="tel">
<div><dt data-t="user">User</dt><dd>$(username)</dd></div>
<div><dt data-t="ip">IP address</dt><dd>$(ip)</dd></div>
<div class="w"><dt>MAC</dt><dd>$(mac)</dd></div>
<div><dt data-t="down">Download</dt><dd>$(bytes-out-nice)</dd></div>
<div><dt data-t="up">Upload</dt><dd>$(bytes-in-nice)</dd></div>
$(if session-time-left)<div><dt data-t="left">Time left</dt><dd>$(session-time-left)</dd></div>$(endif)
</dl>
<form action="$(link-logout)" name="logout" method="post">
<input type="hidden" name="erase-cookie" value="on">
<button id="logout" type="submit" data-t="out">Logout / Disconnect</button>
</form>
</main></div>
<script>
var PREF='';
var T={en:{on:'Connected',title:'You are online',user:'User',ip:'IP address',down:'Download',up:'Upload',left:'Time left',out:'Logout / Disconnect',smp:'Sample session. Your router fills in the real values.',f1:'Fast Speed',f2:'24/7 Stable',f3:'Family Safe',f4:'Best Signal',call:'Call'},
my:{on:'ချိတ်ဆက်ပြီး',title:'အင်တာနက် သုံးနိုင်ပါပြီ',user:'အသုံးပြုသူ',ip:'IP လိပ်စာ',down:'ဒေါင်းလုဒ်',up:'အပ်လုဒ်',left:'ကျန်ချိန်',out:'ထွက်မည် / ချိတ်ဆက်မှု ဖြတ်မည်',smp:'နမူနာ အချက်အလက်။ တကယ့်တန်ဖိုးများကို router က ဖြည့်ပေးသည်။',f1:'မြန်နှုန်းမြင့်',f2:'၂၄ နာရီ တည်ငြိမ်',f3:'မိသားစုအတွက် ဘေးကင်း',f4:'လိုင်းအကောင်းဆုံး',call:'ဖုန်းခေါ်မည်'}};
var lang=PREF||((typeof navigator!=='undefined'&&/^my/i.test(navigator.language||''))?'my':'en');
function setLang(l){
  lang=l;
  document.documentElement.setAttribute('lang',l);
  var nodes=document.querySelectorAll('[data-t]');
  for(var i=0;i<nodes.length;i++){var k=nodes[i].getAttribute('data-t');nodes[i].textContent=(T[l]&&T[l][k])||T.en[k]||nodes[i].textContent;}
  document.getElementById('lang-my').setAttribute('aria-pressed',l==='my'?'true':'false');
  document.getElementById('lang-en').setAttribute('aria-pressed',l==='en'?'true':'false');
}
setLang(lang);
</script>
</body></html>`

const portalStatusCSS = `.card h1{margin-top:14px}.tel{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin:16px 0 4px}.tel div{min-width:0;padding:10px 12px;border-radius:10px;border:1px solid var(--line);background:var(--sunken)}` +
	`.tel .w{grid-column:1/-1}.tel dt{font-size:12px;line-height:1.5;color:var(--ink-mute)}.tel dd{margin:2px 0 0;font:600 15px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--ink);overflow-wrap:anywhere}`

// RenderPortalStatus builds status.html, the connected-session screen, in the same template as the login page.
// Preview swaps the RouterOS variables for a labelled sample session and makes the logout button inert.
func RenderPortalStatus(design *PortalLoginDesign, preview bool) (string, error) {
	d, err := portalDefaults(design)
	if err != nil {
		return "", err
	}
	login := portalLoginHTML()
	head := login[:strings.Index(login, "/*login*/")] // the shared part of the stylesheet; the rest styles login-only controls
	head = strings.Replace(head, "<title>Connect to WiFi</title>", "<title>WiFi connected</title>\n$(if refresh-timeout)<meta http-equiv=\"refresh\" content=\"$(refresh-timeout-secs)\">$(endif)", 1)
	page := portalThemed(head+portalStatusCSS+"</style></head>"+portalStatusBody, d, portalTemplateByID(d.TemplateID))
	if preview {
		page = strings.Replace(page, "<body>", `<body data-preview="true">`, 1)
		page = strings.Replace(page, `$(if refresh-timeout)<meta http-equiv="refresh" content="$(refresh-timeout-secs)">$(endif)`, "", 1)
		page = strings.NewReplacer("$(if session-time-left)", "", "$(endif)", "", "$(username)", "ABCD2345", "$(ip)", "10.5.50.24", "$(mac)", "A4:5E:60:12:34:56",
			"$(bytes-out-nice)", "182.4 MiB", "$(bytes-in-nice)", "12.1 MiB", "$(session-time-left)", "1h42m10s",
			`<form action="$(link-logout)" name="logout" method="post">`, `<p class="hint" data-t="smp">Sample session. Your router fills in the real values.</p><form action="#" name="logout" method="post" onsubmit="return false">`).Replace(page)
		page = strings.Replace(page, "</body>", portalPreviewLinks(false)+"</body>", 1)
	}
	if len(page) > portalStatusMaxBytes {
		return "", fmt.Errorf("portal status page exceeds router size limit")
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
