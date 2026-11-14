package routeros

import (
	"strconv"
	"strings"
)

// DefaultPortalTemplate keeps every portal saved before templates existed looking exactly as it did: the
// obsidian template adds no palette override, so the base stylesheet (with its light-phone twin) is unchanged.
const DefaultPortalTemplate = "obsidian"

// portalPalette holds the template's literal colours so tests can check WCAG contrast without parsing CSS.
type portalPalette struct {
	Dark                                                            bool
	Bg, Surface, Sunken, Ink, Soft, Mute, Edge, Accent, Brand, Tint string
}

// portalTemplate is one selectable look for the router-served login.html. Everything is inline: the router serves
// the page before the guest has internet, so there are no images, fonts or CDNs, only inline CSS and inline SVG.
type portalTemplate struct {
	ID, Group, Name string
	Pal             *portalPalette // nil keeps the base stylesheet
	CSS             string
	Art             string // inline SVG (plus optional badge) shown above the title
	Foot            string // inline SVG shown below the card content
	BG              string // inline full-page background SVG
	Bilingual       bool   // voucher and pay buttons show both languages
	PreferMy        bool   // opens in Burmese unless the phone says otherwise
	Steps           bool   // 1-2-3 voucher instructions
	Tiers           bool   // guest-pass tier cards from the saved plans
	HeaderBrand     bool   // business name shown above the title
}

// PortalTemplateInfo is the public catalogue entry (the editor mirrors it for its picker).
type PortalTemplateInfo struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Name  string `json:"name"`
}

const (
	PortalGroupMyanmar    = "myanmar"
	PortalGroupCommercial = "commercial"
)

// PortalTemplates lists every template in picker order.
func PortalTemplates() []PortalTemplateInfo {
	out := make([]PortalTemplateInfo, 0, len(portalTemplateList))
	for _, t := range portalTemplateList {
		out = append(out, PortalTemplateInfo{ID: t.ID, Group: t.Group, Name: t.Name})
	}
	return out
}

// ValidPortalTemplate reports whether id is a known template. The empty string is valid and means the default.
func ValidPortalTemplate(id string) bool {
	if id == "" {
		return true
	}
	for _, t := range portalTemplateList {
		if t.ID == id {
			return true
		}
	}
	return false
}

func portalTemplateByID(id string) portalTemplate {
	for _, t := range portalTemplateList {
		if t.ID == id {
			return t
		}
	}
	for _, t := range portalTemplateList {
		if t.ID == DefaultPortalTemplate {
			return t
		}
	}
	return portalTemplateList[0]
}

func (p portalPalette) css() string {
	scheme, line, bad, badSoft, shadow := "light", "#00000024", "#b91c1c", "#b91c1c14", "0 1px 3px #0f172a0f,0 10px 28px -10px #0f172a1f"
	if p.Dark {
		scheme, line, bad, badSoft, shadow = "dark", "#ffffff17", "#f87171", "#f8717120", "inset 0 1px 0 #ffffff0f,0 20px 40px -15px #0009"
	}
	return ":root{color-scheme:" + scheme + ";--bg:" + p.Bg + ";--surface:" + p.Surface + ";--sunken:" + p.Sunken + ";--ink:" + p.Ink +
		";--ink-soft:" + p.Soft + ";--ink-mute:" + p.Mute + ";--line:" + line + ";--edge:" + p.Edge + ";--accent:" + p.Accent +
		";--bad:" + bad + ";--bad-soft:" + badSoft + ";--mesh:" + p.Accent + "1f;--glow:0 0 0 3px " + p.Accent + "40;--shadow:" + shadow + "}"
}

// Shared structure for every template except the default: a flex column so the header art, business name and
// language toggle can be ordered without changing the form markup the router and the tests rely on.
const portalTemplateBaseCSS = `.card{display:flex;flex-direction:column}.card>*{min-width:0}.card>.art{order:-2}.card>.brand{margin:24px 0 0}`

var portalTemplateList = []portalTemplate{
	{
		ID: "starlink_mm", Group: PortalGroupMyanmar, Name: "Starlink Community Hub", Bilingual: true,
		Pal: &portalPalette{Dark: true, Bg: "#070d19", Surface: "#0f1a2e", Sunken: "#0a1424", Ink: "#f1f5fb", Soft: "#a9b8d0", Mute: "#8fa0bb", Edge: "#6a7fa3", Accent: "#5cc8ff", Brand: "#5cc8ff", Tint: "#1b3a66"},
		CSS: `.t-starlink_mm{padding-top:20px;border-color:#5cc8ff40}.t-starlink_mm .art{width:100%;height:auto;margin:0 0 14px;border-radius:8px}` +
			`.badge{display:inline-flex;align-items:center;gap:8px;align-self:flex-start;order:-2;min-height:28px;margin:0 0 14px;padding:4px 12px;border-radius:999px;border:1px solid var(--accent);color:var(--accent);font-size:12px;font-weight:650;letter-spacing:.04em}` +
			`.badge i{width:8px;height:8px;border-radius:50%;background:var(--accent);box-shadow:0 0 0 3px #5cc8ff33}` +
			`.t-starlink_mm h1{font-size:26px}.t-starlink_mm button,.t-starlink_mm a.alt{min-height:56px;font-size:17px}.t-starlink_mm .field-label+input{border-color:var(--accent)}`,
		Art: `<svg class="art" viewBox="0 0 340 110" aria-hidden="true" focusable="false"><rect width="340" height="110" rx="8" fill="#0a1424"/><g fill="#a9b8d0"><circle cx="30" cy="20" r="1.2"/><circle cx="90" cy="12" r="1"/><circle cx="150" cy="30" r="1.3"/><circle cx="262" cy="14" r="1"/><circle cx="310" cy="30" r="1.2"/><circle cx="60" cy="54" r="1"/></g><g class="sb" stroke-width="1.4" stroke-linecap="round"><ellipse cx="170" cy="118" rx="170" ry="52" opacity=".55"/><ellipse cx="170" cy="118" rx="128" ry="38" opacity=".35"/><path d="M0 100Q170 60 340 100"/></g><g transform="translate(222 36) rotate(-24)"><rect x="-6" y="-4" width="12" height="8" rx="1.5" fill="#f1f5fb"/><rect x="-25" y="-3" width="16" height="6" class="fb"/><rect x="9" y="-3" width="16" height="6" class="fb"/></g><g class="sb" stroke-width="1.6" stroke-linecap="round"><path d="M214 54a14 14 0 0 1-8 10"/><path d="M210 60a24 24 0 0 1-12 16" opacity=".6"/></g></svg>` +
			`<span class="badge"><i></i><span data-t="link">Fast satellite link</span></span>`,
	},
	{
		ID: "cultural_mm", Group: PortalGroupMyanmar, Name: "Myanmar Cultural Heritage", HeaderBrand: true,
		Pal: &portalPalette{Dark: true, Bg: "#2a0e12", Surface: "#38141a", Sunken: "#240b0f", Ink: "#fbf3e4", Soft: "#e4cfaf", Mute: "#cdb48d", Edge: "#b8955a", Accent: "#d4a338", Brand: "#d4a338", Tint: "#7a1f2b"},
		CSS: `.t-cultural_mm{border-radius:10px;border-color:#d4a33866;box-shadow:inset 0 0 0 5px var(--surface),inset 0 0 0 6px #d4a33899,var(--shadow);padding:36px 32px}` +
			`.t-cultural_mm h1{font-family:"Noto Serif Myanmar",Georgia,"Times New Roman",serif;font-weight:600;text-align:center}.t-cultural_mm>p{text-align:center}` +
			`.t-cultural_mm .brand{order:-1;text-align:center;margin:0 0 10px;letter-spacing:.14em;text-transform:uppercase;font-size:12px;color:var(--accent)}` +
			`.t-cultural_mm .art,.t-cultural_mm .foot{width:100%;height:16px;color:var(--accent)}.t-cultural_mm .art{margin:0 0 14px}.t-cultural_mm .foot{order:9;margin:22px 0 0}`,
		Art:  `<svg class="art" aria-hidden="true" focusable="false"><defs><pattern id="mmp" width="24" height="16" patternUnits="userSpaceOnUse"><path d="M12 1L22 8L12 15L2 8Z" fill="none" stroke="currentColor" stroke-width="1.2"/><circle cx="12" cy="8" r="1.7" fill="currentColor"/></pattern></defs><rect width="100%" height="16" fill="url(#mmp)"/></svg>`,
		Foot: `<svg class="foot" aria-hidden="true" focusable="false"><rect width="100%" height="16" fill="url(#mmp)"/></svg>`,
	},
	{
		ID: "mountain_mm", Group: PortalGroupMyanmar, Name: "Highland & Mountain WISP",
		Pal: &portalPalette{Dark: true, Bg: "#0a221e", Surface: "#13332c", Sunken: "#0d2a25", Ink: "#ecf6f2", Soft: "#b3cfc6", Mute: "#94b8ad", Edge: "#6a9b8d", Accent: "#6ee7b7", Brand: "#34d399", Tint: "#0f766e"},
		CSS: `.t-mountain_mm{padding-top:20px}.t-mountain_mm .art{width:100%;height:auto;margin:0 0 14px;border-radius:8px}.t-mountain_mm h1{font-size:26px}`,
		Art: `<svg class="art" viewBox="0 0 340 110" aria-hidden="true" focusable="false"><defs><linearGradient id="mmg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0d2a25"/><stop offset="1" stop-color="#1c4a40"/></linearGradient></defs><rect width="340" height="110" rx="8" fill="url(#mmg)"/><path d="M0 78L38 46L62 64L104 30L150 70L196 40L246 74L292 48L340 76V110H0Z" fill="#2c6a5a" opacity=".55"/><path d="M0 92L46 62L86 82L130 54L184 90L232 64L288 92L340 70V110H0Z" fill="#0a221e" opacity=".85"/><g class="sb" stroke-width="1.6" stroke-linecap="round"><path d="M292 18V50M284 50H300"/><path d="M280 24a14 14 0 0 1 0-12M304 12a14 14 0 0 1 0 12" opacity=".8"/><path d="M274 28a22 22 0 0 1 0-20M310 8a22 22 0 0 1 0 20" opacity=".45"/></g><circle cx="292" cy="18" r="2.6" class="fb"/></svg>`,
	},
	{
		ID: "village_mm", Group: PortalGroupMyanmar, Name: "Village & Community Wi-Fi", PreferMy: true, Steps: true,
		Pal: &portalPalette{Dark: false, Bg: "#fff1dc", Surface: "#ffffff", Sunken: "#fff8ee", Ink: "#3b1f0e", Soft: "#5e3a1f", Mute: "#6f4726", Edge: "#8a5a2b", Accent: "#b45309", Brand: "#c2562a", Tint: "#f59e0b"},
		CSS: `body{font-size:18px}.t-village_mm{max-width:440px;padding:28px 24px;border-width:2px;border-color:#e9c9a0}.t-village_mm h1{font-size:34px;line-height:1.6}.t-village_mm>p{font-size:19px;line-height:1.75}` +
			`.t-village_mm input{min-height:68px;font-size:26px}.t-village_mm button{min-height:64px;font-size:20px;font-weight:700}.t-village_mm a.alt{min-height:56px;font-size:18px}.t-village_mm .lang button{min-height:48px;font-size:16px}` +
			`.t-village_mm .art{width:64px;height:64px;margin:0 0 12px}.steps{list-style:none;counter-reset:s;margin:0 0 20px;padding:0;display:grid;gap:10px}` +
			`.steps li{counter-increment:s;display:flex;align-items:center;gap:14px;min-height:48px;font-size:19px;line-height:1.75;color:var(--ink)}` +
			`.steps li::before{content:counter(s);flex:none;width:40px;height:40px;border-radius:50%;display:grid;place-items:center;background:var(--portal-brand);color:var(--portal-on-brand);font-weight:700;font-size:20px}`,
		Art: `<svg class="art" viewBox="0 0 64 64" aria-hidden="true" focusable="false"><circle cx="32" cy="32" r="30" fill="#fde7c4"/><path d="M10 44L32 24L54 44" fill="none" stroke="#8a5a2b" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/><rect x="18" y="42" width="28" height="12" fill="#c2562a"/><g class="sb" stroke-width="3" stroke-linecap="round"><path d="M22 18a14 14 0 0 1 20 0"/><path d="M27 23a7 7 0 0 1 10 0"/></g></svg>`,
	},
	{
		ID: "obsidian", Group: PortalGroupCommercial, Name: "Obsidian Telemetry",
		CSS: `.t-obsidian input{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}`,
		Art: portalWiFiMarkSVG,
		BG:  `<svg class="bg-art" viewBox="0 0 400 800" preserveAspectRatio="xMidYMid slice" aria-hidden="true" focusable="false"><g class="sb" stroke-width="1" fill="none"><path d="M-20 120C60 80 120 160 200 120S340 60 420 110"/><path d="M-20 170C70 130 130 210 200 170S330 110 420 160"/><path d="M-20 220C80 180 140 260 200 220S320 160 420 210"/><path d="M-20 560C70 520 150 600 220 560S350 500 420 550"/><path d="M-20 610C80 570 160 650 230 610S340 550 420 600"/><path d="M-20 660C90 620 170 700 240 660S330 600 420 650"/></g></svg>`,
	},
	{
		ID: "cafe", Group: PortalGroupCommercial, Name: "Warm Café & Bistro", HeaderBrand: true,
		Pal: &portalPalette{Dark: false, Bg: "#f3e6d3", Surface: "#fffaf2", Sunken: "#f8eddc", Ink: "#2b1a12", Soft: "#5a4232", Mute: "#6b5240", Edge: "#8a6a50", Accent: "#8a4b24", Brand: "#6f3b1e", Tint: "#c98b4b"},
		CSS: `.t-cafe{border-radius:28px;padding:36px 28px;border-color:#e2cdb0}.t-cafe h1{font-family:Georgia,"Times New Roman","Noto Serif Myanmar",serif;font-size:30px;font-weight:600}.t-cafe input,.t-cafe button,.t-cafe a.alt{border-radius:16px}` +
			`.t-cafe .brand{order:-1;margin:0 0 6px;font-size:13px;letter-spacing:.12em;text-transform:uppercase;color:var(--accent)}.t-cafe .art{width:56px;height:56px;margin:0 0 12px;color:var(--accent)}`,
		Art: `<svg class="art" viewBox="0 0 56 56" aria-hidden="true" focusable="false"><g fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M10 24h30v10a12 12 0 0 1-12 12h-6A12 12 0 0 1 10 34Z"/><path d="M40 28h4a5 5 0 0 1 0 10h-5"/><path d="M18 8c-3 3 3 5 0 9M26 8c-3 3 3 5 0 9M34 8c-3 3 3 5 0 9" opacity=".8"/></g></svg>`,
	},
	{
		ID: "hotel", Group: PortalGroupCommercial, Name: "Boutique Hotel & Lounge", HeaderBrand: true, Tiers: true,
		Pal: &portalPalette{Dark: false, Bg: "#ece7dc", Surface: "#fbfaf6", Sunken: "#f3f0e8", Ink: "#1a1a18", Soft: "#46453f", Mute: "#58574f", Edge: "#77746a", Accent: "#7a5c1f", Brand: "#1f2a44", Tint: "#b08d3c"},
		CSS: `.t-hotel{border-radius:2px;border-top:6px solid var(--portal-brand);padding:32px 30px}.t-hotel h1{font-family:Georgia,"Times New Roman","Noto Serif Myanmar",serif;font-size:32px;font-weight:500;letter-spacing:-.01em}` +
			`.t-hotel input,.t-hotel button,.t-hotel a.alt{border-radius:2px}.t-hotel .brand{order:-1;margin:0 0 10px;font-size:12px;letter-spacing:.2em;text-transform:uppercase;color:var(--accent)}` +
			`.t-hotel .art{width:100%;height:14px;margin:0 0 16px;color:var(--accent)}` +
			`.tiers{list-style:none;margin:0 0 20px;padding:0;display:grid;gap:8px}.tiers li{display:grid;grid-template-columns:1fr auto;gap:2px 12px;padding:12px 14px;border:1px solid var(--edge);border-radius:2px;background:var(--sunken)}` +
			`.tiers b{font-weight:650;font-size:15px;color:var(--ink)}.tiers .amt{font-weight:650;font-size:15px;color:var(--ink);text-align:right}.tiers span{font-size:13px;color:var(--ink-soft)}`,
		Art: `<svg class="art" viewBox="0 0 300 14" preserveAspectRatio="none" aria-hidden="true" focusable="false"><path d="M0 7H128M172 7H300" stroke="currentColor" stroke-width="1" fill="none"/><path d="M150 1L157 7L150 13L143 7Z" fill="currentColor"/></svg>`,
	},
	{
		ID: "express", Group: PortalGroupCommercial, Name: "Express Retail & Transit",
		Pal: &portalPalette{Dark: true, Bg: "#0a0a0a", Surface: "#171717", Sunken: "#0a0a0a", Ink: "#ffffff", Soft: "#d4d4d4", Mute: "#a8a8a8", Edge: "#8a8a8a", Accent: "#facc15", Brand: "#facc15", Tint: "#171717"},
		CSS: `.t-express{max-width:440px;padding:26px 22px;border:2px solid var(--accent);border-radius:20px}.t-express h1{font-size:30px;font-weight:800;text-transform:uppercase;letter-spacing:-.01em}` +
			`.t-express .art{width:44px;height:44px;margin:0 0 10px;color:var(--accent)}` +
			`.t-express #voucher{min-height:76px;padding:0 6px;font-size:28px;font-weight:800;letter-spacing:.55ch;border-width:2px;border-radius:12px;background-image:repeating-linear-gradient(90deg,transparent 0,transparent calc(1.55ch - 2px),var(--edge) calc(1.55ch - 2px),var(--edge) 1.55ch);background-size:calc(8*1.55ch) 100%;background-repeat:no-repeat;background-origin:content-box}` +
			`.t-express button{min-height:64px;font-size:19px;font-weight:800;text-transform:uppercase;letter-spacing:.04em;border-radius:12px}.t-express a.alt{min-height:56px;border-radius:12px;font-weight:700}`,
		Art: `<svg class="art" viewBox="0 0 44 44" aria-hidden="true" focusable="false"><path d="M25 3L8 25h11l-3 16 20-24H24Z" fill="currentColor"/></svg>`,
	},
	{
		ID: "daylight", Group: PortalGroupCommercial, Name: "Clean Daylight",
		Pal: &portalPalette{Dark: false, Bg: "#f5f6f8", Surface: "#ffffff", Sunken: "#ffffff", Ink: "#0b1220", Soft: "#374151", Mute: "#4b5563", Edge: "#6b7280", Accent: "#0a58ca", Brand: "#0a58ca", Tint: "#0a58ca"},
		CSS: `.t-daylight{border-radius:4px;border:1px solid #cfd4dc;box-shadow:none;padding:36px 32px}.t-daylight h1{font-weight:600;letter-spacing:-.02em}.t-daylight input,.t-daylight button,.t-daylight a.alt{border-radius:2px}` +
			`.t-daylight .art{width:44px;height:44px;margin:0 0 16px;color:var(--accent)}`,
		Art: `<svg class="art" viewBox="0 0 44 44" aria-hidden="true" focusable="false"><rect x=".5" y=".5" width="43" height="43" fill="none" stroke="currentColor" stroke-width="1"/><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M10 18a17 17 0 0 1 24 0M15 23a10 10 0 0 1 14 0M19.5 28a3.6 3.6 0 0 1 5 0"/></g><circle cx="22" cy="33" r="1.4" fill="currentColor"/></svg>`,
	},
}

// portalAmount formats whole kyat with thousands separators ("1,000").
func portalAmount(n int) string {
	s := strconv.Itoa(n)
	if n < 1000 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// portalDuration is a compact unit-only length ("30 min", "2 h", "7 d") that reads the same in both languages.
func portalDuration(minutes int) string {
	switch {
	case minutes <= 0:
		return ""
	case minutes%1440 == 0:
		return strconv.Itoa(minutes/1440) + " d"
	case minutes%60 == 0:
		return strconv.Itoa(minutes/60) + " h"
	default:
		return strconv.Itoa(minutes) + " min"
	}
}
