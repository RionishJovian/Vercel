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

// portalTemplate is one selectable look for the router-served login.html and status.html. Everything is inline:
// the router serves the pages before the guest has internet, so there are no images, fonts or CDNs.
type portalTemplate struct {
	ID, Group, Name string
	Pal             *portalPalette // nil keeps the base stylesheet
	CSS             string
	Art             string // full-bleed inline SVG scene behind the hero header
	Hero            string // CSS background of the hero header; empty keeps the base mesh
	HeroInk, HeroBg string // hero text colour and the hero stop it must stay readable on (tests check the pair)
	Badges          string // pills shown in the hero
	En, My          string // extra strings for the page's translation table, as JS object members
	Foot            string // inline SVG shown below the card content
	BG              string // inline full-page background SVG
	Bilingual       bool   // voucher and pay buttons show both languages
	PreferMy        bool   // opens in Burmese unless the phone says otherwise
	Steps           bool   // 1-2-3 voucher instructions
	Welcome         bool   // "Welcome to" line above the business name
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

const portalScene = `<svg class="sc" viewBox="0 0 400 200" preserveAspectRatio="xMidYMid slice" aria-hidden="true" focusable="false">`

var portalTemplateList = []portalTemplate{
	{
		ID: "starlink_mm", Group: PortalGroupMyanmar, Name: "Starlink Community Hub", Bilingual: true,
		Pal:  &portalPalette{Dark: true, Bg: "#070d19", Surface: "#0f1a2e", Sunken: "#0a1424", Ink: "#f1f5fb", Soft: "#a9b8d0", Mute: "#8fa0bb", Edge: "#6a7fa3", Accent: "#5cc8ff", Brand: "#5cc8ff", Tint: "#1b3a66"},
		Hero: "radial-gradient(ellipse 80% 70% at 78% 8%,#17335f,transparent),linear-gradient(180deg,#030712,#0b1a33 62%,#070d19)", HeroInk: "#f1f5fb", HeroBg: "#17335f",
		CSS: `.t-starlink_mm{border-color:#5cc8ff40}.t-starlink_mm #voucher{border-color:var(--accent)}.t-starlink_mm #connect{min-height:56px;font-size:17px}`,
		Art: portalScene + `<g fill="#cfe3ff"><circle cx="28" cy="26" r="1.2"/><circle cx="92" cy="52" r="1"/><circle cx="146" cy="18" r="1.3"/><circle cx="208" cy="44" r="1"/><circle cx="318" cy="20" r="1.2"/><circle cx="374" cy="74" r="1"/><circle cx="58" cy="92" r="1"/><circle cx="180" cy="96" r=".9"/></g>` +
			`<g class="sb" stroke-width="1.4"><path d="M-10 142Q200 30 410 112" opacity=".75"/><path d="M-10 172Q200 76 410 146" opacity=".35"/></g>` +
			`<g transform="translate(236 60) rotate(-20)"><rect x="-7" y="-5" width="14" height="10" rx="2" fill="#f1f5fb"/><rect x="-31" y="-4" width="21" height="8" class="fb"/><rect x="10" y="-4" width="21" height="8" class="fb"/></g>` +
			`<g transform="translate(316 130)"><circle r="40" class="fb" opacity=".12"/><path d="M-22-6a24 24 0 0 0 36 16z" fill="#e6eefc"/><path d="M-2 4l-7 22h20" class="sb" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/><path d="M-5-10l-11-15" stroke="#e6eefc" stroke-width="2"/><g class="sb" stroke-width="1.8" stroke-linecap="round"><path d="M-25-26a13 13 0 0 1 11-9"/><path d="M-34-28a22 22 0 0 1 19-16" opacity=".6"/></g></g></svg>`,
		Badges: `<span data-t="link">High-Speed Satellite Internet</span><span><i></i><b data-t="st">Online · Stable Link</b></span>`,
		En:     `link:'High-Speed Satellite Internet',st:'Online \u00b7 Stable Link'`,
		My:     `link:'မြန်နှုန်းမြင့် ဂြိုဟ်တု အင်တာနက်',st:'အွန်လိုင်း \u00b7 လိုင်းတည်ငြိမ်'`,
	},
	{
		ID: "cultural_mm", Group: PortalGroupMyanmar, Name: "Myanmar Cultural Heritage",
		Pal:  &portalPalette{Dark: true, Bg: "#2a0e12", Surface: "#38141a", Sunken: "#240b0f", Ink: "#fbf3e4", Soft: "#e4cfaf", Mute: "#cdb48d", Edge: "#b8955a", Accent: "#d4a338", Brand: "#d4a338", Tint: "#7a1f2b"},
		Hero: "radial-gradient(circle at 74% 34%,#8a2a2f,transparent 46%),linear-gradient(180deg,#5a1820,#2a0e12)", HeroInk: "#fbf3e4", HeroBg: "#8a2a2f",
		CSS: `.t-cultural_mm{border-radius:6px;border-color:#d4a338;box-shadow:inset 0 0 0 4px var(--surface),inset 0 0 0 5px #d4a33899,var(--shadow);padding:30px 24px}` +
			`.t-cultural_mm::before,.t-cultural_mm::after{content:"";position:absolute;width:30px;height:30px;border:3px solid var(--accent)}.t-cultural_mm::before{top:-4px;left:-4px;border-width:3px 0 0 3px}.t-cultural_mm::after{right:-4px;bottom:-4px;border-width:0 3px 3px 0}` +
			`.t-cultural_mm h1{font-family:"Noto Serif Myanmar",Georgia,"Times New Roman",serif;font-weight:600;text-align:center}.t-cultural_mm>p{text-align:center}.t-cultural_mm .foot{display:block;width:100%;height:16px;margin:20px 0 0;color:var(--accent)}`,
		Art: portalScene + `<defs><pattern id="mmp" width="24" height="16" patternUnits="userSpaceOnUse"><path d="M12 1L22 8L12 15L2 8Z" fill="none" stroke="#d4a338" stroke-width="1.2"/><circle cx="12" cy="8" r="1.7" fill="#d4a338"/></pattern>` +
			`<path id="pgd" d="M-30 0h60l-8-9q-7-8-9-22l-4-9-3-17-2-9-4-22-4 22-2 9-3 17-4 9q-2 14-9 22z"/></defs>` +
			`<circle cx="296" cy="68" r="34" fill="#d4a338" opacity=".28"/><g fill="#d4a338"><use href="#pgd" transform="translate(296 150) scale(1.15)"/><use href="#pgd" transform="translate(214 150) scale(.7)" opacity=".8"/><use href="#pgd" transform="translate(364 150) scale(.62)" opacity=".7"/><use href="#pgd" transform="translate(152 150) scale(.42)" opacity=".55"/></g>` +
			`<path d="M0 150h400v50H0z" fill="#2a0e12"/><rect y="6" width="400" height="16" fill="url(#mmp)" opacity=".75"/></svg>`,
		Foot: `<svg class="foot" aria-hidden="true" focusable="false"><rect width="100%" height="16" fill="url(#mmp)"/></svg>`,
	},
	{
		ID: "mountain_mm", Group: PortalGroupMyanmar, Name: "Highland & Mountain WISP",
		Pal:  &portalPalette{Dark: true, Bg: "#0a221e", Surface: "#13332c", Sunken: "#0d2a25", Ink: "#ecf6f2", Soft: "#b3cfc6", Mute: "#94b8ad", Edge: "#6a9b8d", Accent: "#6ee7b7", Brand: "#34d399", Tint: "#0f766e"},
		Hero: "linear-gradient(180deg,#1d5a4e,#123a33 55%,#0a221e)", HeroInk: "#ecf6f2", HeroBg: "#1d5a4e",
		Art: portalScene + `<path d="M0 118L46 70L78 96L132 44L186 104L238 62L300 112L352 74L400 108V200H0Z" fill="#2c6a5a" opacity=".6"/><g fill="#ecf6f2" opacity=".16"><ellipse cx="96" cy="112" rx="120" ry="9"/><ellipse cx="310" cy="124" rx="130" ry="10"/></g>` +
			`<path d="M0 142L58 102L108 130L164 92L232 140L292 106L362 142L400 120V200H0Z" fill="#123a33"/><ellipse cx="190" cy="150" rx="210" ry="9" fill="#ecf6f2" opacity=".1"/><path d="M0 164L70 138L150 160L230 134L320 164L400 146V200H0Z" fill="#0a221e"/>` +
			`<g transform="translate(-34 26)"><g class="sb" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M320 40V104M308 104L320 62L332 104M312 86H328"/><path d="M304 50a18 18 0 0 1 0-20M336 30a18 18 0 0 1 0 20" opacity=".85"/><path d="M294 58a30 30 0 0 1 0-36M346 22a30 30 0 0 1 0 36" opacity=".45"/></g><circle cx="320" cy="40" r="9" class="fb" opacity=".25"/><circle cx="320" cy="40" r="3.4" class="fb"/></g></svg>`,
		Badges: `<span><i></i><b data-t="hl">Highland wireless link</b></span>`,
		En:     `hl:'Highland wireless link'`,
		My:     `hl:'တောင်ပေါ် ကြိုးမဲ့လိုင်း'`,
	},
	{
		ID: "village_mm", Group: PortalGroupMyanmar, Name: "Village & Community Wi-Fi", PreferMy: true, Steps: true,
		Pal:  &portalPalette{Dark: false, Bg: "#fff1dc", Surface: "#ffffff", Sunken: "#fff8ee", Ink: "#3b1f0e", Soft: "#5e3a1f", Mute: "#6f4726", Edge: "#8a5a2b", Accent: "#b45309", Brand: "#c2562a", Tint: "#f59e0b"},
		Hero: "linear-gradient(180deg,#f7a84a,#fbd08a 55%,#fff1dc)", HeroInk: "#3b1f0e", HeroBg: "#f7a84a",
		CSS: `body{font-size:18px}.t-village_mm{border-width:2px;border-color:#e9c9a0}.t-village_mm h1{font-size:32px;line-height:1.6}.t-village_mm>p{font-size:19px;line-height:1.75}` +
			`.t-village_mm #voucher{min-height:68px;font-size:26px}.t-village_mm #connect{min-height:64px;font-size:20px;font-weight:700}.t-village_mm .tabs button,.t-village_mm .gh,.t-village_mm a.alt{min-height:56px;font-size:17px}.hn{font-size:24px}` +
			`.steps{list-style:none;counter-reset:s;margin:0 0 18px;padding:0;display:grid;gap:6px}.steps li{counter-increment:s;position:relative;display:flex;align-items:center;gap:14px;min-height:52px;font-size:19px;line-height:1.75;color:var(--ink)}` +
			`.steps li::before{content:counter(s);flex:none;width:42px;height:42px;border-radius:50%;display:grid;place-items:center;background:var(--portal-brand);color:var(--portal-on-brand);font-weight:700;font-size:21px}` +
			`.steps li+li::after{content:"";position:absolute;left:20px;top:-10px;width:2px;height:14px;background:var(--edge)}`,
		Art: portalScene + `<circle cx="128" cy="52" r="30" fill="#fff3d6" opacity=".9"/><circle cx="128" cy="52" r="48" fill="#fff3d6" opacity=".3"/><path d="M0 150Q100 118 200 144T400 132V200H0Z" fill="#e58a3a" opacity=".55"/>` +
			`<g transform="translate(250 96)"><path d="M-44 30L0-8L44 30Z" fill="#8a4a1f"/><rect x="-32" y="30" width="64" height="40" fill="#c2562a"/><rect x="-8" y="42" width="16" height="28" fill="#5e3a1f"/><rect x="14" y="40" width="12" height="12" fill="#fff3d6"/></g>` +
			`<g stroke="#5e3a1f" stroke-width="4" stroke-linecap="round" fill="none"><path d="M352 170Q356 130 350 104"/><path d="M350 104q-18-8-30 4M350 104q-8-18-26-16M350 104q14-14 30-6M350 104q20-2 26 14"/></g>` +
			`<g stroke="#8a4a1f" stroke-width="3.4" stroke-linecap="round" fill="none"><path d="M226 74a34 34 0 0 1 48 0"/><path d="M236 84a20 20 0 0 1 28 0"/></g><path d="M0 172Q120 150 240 168T400 160V200H0Z" fill="#fff1dc"/></svg>`,
		En: `s1:'Type your voucher code',s2:'Tap Connect',s3:'You are online'`,
		My: `s1:'ဘောက်ချာကုဒ် ရိုက်ထည့်ပါ',s2:'ချိတ်ဆက်မည် ကို နှိပ်ပါ',s3:'အင်တာနက် သုံးနိုင်ပါပြီ'`,
	},
	{
		ID: "obsidian", Group: PortalGroupCommercial, Name: "Obsidian Telemetry",
		CSS: `.t-obsidian{border-radius:10px;border-top:2px solid var(--accent)}.t-obsidian #voucher{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}.hero .wifi-mark{filter:drop-shadow(0 0 10px var(--accent))}`,
		Art: portalScene + `<path class="sb" stroke-width="2" opacity=".1" d="M-80 200L120 0M-40 200L160 0M0 200L200 0M40 200L240 0M80 200L280 0M120 200L320 0M160 200L360 0M200 200L400 0M240 200L440 0M280 200L480 0"/>` +
			`<g transform="translate(-26 30)"><g class="sb" stroke-width="2" stroke-linejoin="round"><path d="M250 30h110l20 20v70l-20 20H270l-20-20z" opacity=".5"/><path d="M286 78a18 18 0 0 1 18-18h32a18 18 0 0 1 18 18l4 28a9 9 0 0 1-16 6l-8-10h-28l-8 10a9 9 0 0 1-16-6z" stroke-width="2.6"/><path d="M300 78v12M294 84h12" stroke-linecap="round"/></g><circle cx="338" cy="80" r="3" class="fb"/><circle cx="346" cy="90" r="3" class="fb"/></g>` +
			`<g class="sb" stroke-width="2" stroke-linecap="round" opacity=".6"><path d="M20 150h70l12-12h60M20 164h40"/></g></svg>` + portalWiFiMarkSVG,
		Badges: `<span><i></i><b data-t="lp">Low ping</b></span><span data-t="al">Anti-lag</span>`,
		En:     `lp:'Low ping',al:'Anti-lag'`,
		My:     `lp:'Ping နည်း',al:'Lag ကင်း'`,
		BG:     `<svg class="bg-art" viewBox="0 0 400 800" preserveAspectRatio="xMidYMid slice" aria-hidden="true" focusable="false"><g class="sb" stroke-width="1" fill="none"><path d="M-20 560C70 520 150 600 220 560S350 500 420 550"/><path d="M-20 610C80 570 160 650 230 610S340 550 420 600"/><path d="M-20 660C90 620 170 700 240 660S330 600 420 650"/></g></svg>`,
	},
	{
		ID: "cafe", Group: PortalGroupCommercial, Name: "Warm Café & Bistro",
		Pal:  &portalPalette{Dark: false, Bg: "#f3e6d3", Surface: "#fffaf2", Sunken: "#f8eddc", Ink: "#2b1a12", Soft: "#5a4232", Mute: "#6b5240", Edge: "#8a6a50", Accent: "#8a4b24", Brand: "#6f3b1e", Tint: "#c98b4b"},
		Hero: "radial-gradient(circle at 76% 40%,#f6dfba,transparent 52%),linear-gradient(180deg,#d9a566,#ecd0a4 60%,#f3e6d3)", HeroInk: "#2b1a12", HeroBg: "#d9a566",
		CSS: `.t-cafe{border-radius:28px;border:2px dashed #c9a57a;padding:28px 22px}.t-cafe h1{font-family:Georgia,"Times New Roman","Noto Serif Myanmar",serif;font-size:28px;font-weight:600}` +
			`.t-cafe input,.t-cafe button,.t-cafe a.alt{border-radius:16px}.t-cafe .tabs,.t-cafe .tabs button{border-radius:999px}.pg li,.sup a{border-radius:20px}`,
		Art: portalScene + `<g transform="translate(-34 22)"><g fill="none" stroke="#8a4b24" stroke-width="5" stroke-linecap="round" opacity=".75"><path d="M276 30c-12 12 12 20 0 34M306 22c-12 12 12 20 0 36M336 30c-12 12 12 20 0 34"/></g>` +
			`<ellipse cx="306" cy="160" rx="92" ry="12" fill="#6f3b1e" opacity=".3"/><path d="M246 80h120v34a52 52 0 0 1-52 52h-16a52 52 0 0 1-52-52z" fill="#fffaf2" stroke="#6f3b1e" stroke-width="5" stroke-linejoin="round"/><path d="M366 92h14a17 17 0 0 1 0 34h-18" fill="none" stroke="#6f3b1e" stroke-width="5" stroke-linecap="round"/>` +
			`<ellipse cx="306" cy="82" rx="54" ry="9" fill="#6f3b1e"/><path d="M294 112a14 14 0 0 1 24 0M286 104a26 26 0 0 1 40 0" fill="none" stroke="#8a4b24" stroke-width="4" stroke-linecap="round"/><circle cx="306" cy="124" r="4" fill="#8a4b24"/></g>` +
			`<g fill="#6f3b1e" opacity=".8"><ellipse cx="60" cy="54" rx="9" ry="13" transform="rotate(-30 60 54)"/><ellipse cx="96" cy="36" rx="7" ry="10" transform="rotate(24 96 36)"/><ellipse cx="34" cy="92" rx="6" ry="9" transform="rotate(50 34 92)"/></g></svg>`,
	},
	{
		ID: "hotel", Group: PortalGroupCommercial, Name: "Boutique Hotel & Lounge", Welcome: true,
		Pal:  &portalPalette{Dark: false, Bg: "#ece7dc", Surface: "#fbfaf6", Sunken: "#f3f0e8", Ink: "#1a1a18", Soft: "#46453f", Mute: "#58574f", Edge: "#77746a", Accent: "#7a5c1f", Brand: "#1f2a44", Tint: "#b08d3c"},
		Hero: "linear-gradient(180deg,#141b33,#3a2f5c 55%,#6d4660)", HeroInk: "#ffffff", HeroBg: "#6d4660",
		CSS: `.t-hotel{border-radius:4px;border:1px solid #b08d3c;box-shadow:0 0 0 4px #fbfaf6,0 0 0 5px #b08d3c,0 24px 44px -18px #141b3380;padding:30px 24px}.t-hotel h1{font-family:Georgia,"Times New Roman","Noto Serif Myanmar",serif;font-size:30px;font-weight:500;letter-spacing:-.01em}` +
			`.t-hotel input,.t-hotel button,.t-hotel a.alt,.pg li,.pg a,.sup a{border-radius:3px}.hw{letter-spacing:.18em;text-transform:uppercase;color:#f1d48a}.hn{font-family:Georgia,"Times New Roman","Noto Serif Myanmar",serif;font-size:26px;font-weight:500}`,
		Art: portalScene + `<g fill="#fff"><circle cx="40" cy="30" r="1.1"/><circle cx="120" cy="54" r="1"/><circle cx="190" cy="24" r="1.2"/><circle cx="262" cy="46" r="1"/></g><circle cx="92" cy="46" r="16" fill="#f1d48a"/><circle cx="99" cy="41" r="14" fill="#1c2140"/>` +
			`<g fill="#141b33"><path d="M214 150V84h36V66h52v18h36v66z"/><path d="M0 150h400v50H0z"/><path d="M150 150q4-44-2-70 4 26 10 70z"/></g><g fill="#f1d48a"><path d="M226 96h8v10h-8zM246 96h8v10h-8zM288 96h8v10h-8zM308 96h8v10h-8zM226 120h8v10h-8zM246 120h8v10h-8zM288 120h8v10h-8zM308 120h8v10h-8zM266 78h8v10h-8zM282 78h8v10h-8zM268 124h16v26h-16z"/></g>` +
			`<g stroke="#141b33" stroke-width="5" stroke-linecap="round" fill="none"><path d="M148 80q-22-10-34 6M148 80q-10-22-30-18M148 80q16-18 34-8M148 80q24-4 30 16"/></g></svg>`,
		En: `wel:'Welcome to'`,
		My: `wel:'ကြိုဆိုပါသည်'`,
	},
	{
		ID: "express", Group: PortalGroupCommercial, Name: "Express Retail & Transit",
		Pal:  &portalPalette{Dark: false, Bg: "#eef4ff", Surface: "#ffffff", Sunken: "#f5f8ff", Ink: "#0b1b3a", Soft: "#33456b", Mute: "#4a5b80", Edge: "#64779f", Accent: "#1d4ed8", Brand: "#1d4ed8", Tint: "#1d4ed8"},
		Hero: "linear-gradient(135deg,#1e3a8a,#1d4ed8 55%,#2563eb)", HeroInk: "#ffffff", HeroBg: "#2563eb",
		CSS: `.t-express{border-radius:20px;border:0;box-shadow:0 2px 4px #0b1b3a14,0 22px 44px -20px #1d4ed866}.t-express h1{font-size:28px;font-weight:800;letter-spacing:-.01em}` +
			`.t-express #voucher{min-height:72px;padding:0 6px;font-size:28px;font-weight:800;letter-spacing:.55ch;border-width:2px;border-radius:12px;background-image:repeating-linear-gradient(90deg,transparent 0,transparent calc(1.55ch - 2px),var(--edge) calc(1.55ch - 2px),var(--edge) 1.55ch);background-size:calc(8*1.55ch) 100%;background-repeat:no-repeat;background-origin:content-box}` +
			`.t-express #voucher::placeholder{letter-spacing:.55ch}.t-express #connect{min-height:60px;font-size:18px;font-weight:800;border-radius:12px}.bdg span{border-color:#fff}`,
		Art: portalScene + `<g fill="#fff" opacity=".1"><circle cx="330" cy="70" r="110"/><circle cx="330" cy="70" r="70"/></g><g stroke="#fff" stroke-width="5" stroke-linecap="round" fill="none"><path d="M270 62a84 84 0 0 1 120 0" opacity=".45"/><path d="M290 82a56 56 0 0 1 80 0" opacity=".7"/><path d="M310 102a28 28 0 0 1 40 0"/></g><circle cx="330" cy="124" r="8" fill="#fff"/>` +
			`<g stroke="#fff" stroke-width="4" stroke-linecap="round" opacity=".55"><path d="M20 60h90M46 84h120M20 108h64"/></g></svg>`,
		Badges: `<span data-t="fr">Fast &amp; Reliable Internet</span>`,
		En:     `fr:'Fast & Reliable Internet'`,
		My:     `fr:'မြန်ဆန် စိတ်ချရသော အင်တာနက်'`,
	},
	{
		ID: "daylight", Group: PortalGroupCommercial, Name: "Clean Daylight",
		Pal:  &portalPalette{Dark: false, Bg: "#f5f6f8", Surface: "#ffffff", Sunken: "#ffffff", Ink: "#0b1220", Soft: "#374151", Mute: "#4b5563", Edge: "#6b7280", Accent: "#0a58ca", Brand: "#0a58ca", Tint: "#0a58ca"},
		Hero: "linear-gradient(180deg,#bfdbfe,#e0edff 60%,#f5f6f8)", HeroInk: "#0b1220", HeroBg: "#bfdbfe",
		CSS: `.t-daylight{border-radius:10px;border:1px solid #d5dbe5;box-shadow:0 1px 2px #0b12201a,0 18px 40px -18px #0a58ca59}.t-daylight h1{font-weight:650;letter-spacing:-.02em}.t-daylight input,.t-daylight button,.t-daylight a.alt{border-radius:6px}.pg li{box-shadow:0 1px 2px #0b12201a,0 10px 24px -14px #0a58ca40}`,
		Art: portalScene + `<circle cx="236" cy="84" r="22" fill="#fff" opacity=".9"/><g fill="#fff"><path d="M40 70a18 18 0 0 1 34-6 14 14 0 0 1 22 12 12 12 0 0 1-4 23H44a15 15 0 0 1-4-29z"/><path d="M190 36a12 12 0 0 1 23-4 10 10 0 0 1 15 8 8 8 0 0 1-3 16h-33a10 10 0 0 1-2-20z" opacity=".8"/></g>` +
			`<g class="sb" stroke-width="6" stroke-linecap="round"><path d="M262 104a96 96 0 0 1 136 0" opacity=".3"/><path d="M284 126a64 64 0 0 1 92 0" opacity=".6"/><path d="M308 148a30 30 0 0 1 44 0"/></g><circle cx="330" cy="168" r="8" class="fb"/></svg>`,
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

// portalData is a compact data allowance ("500 MB", "2 GB", "1.5 GB").
func portalData(mb int) string {
	if mb < 1024 {
		return strconv.Itoa(mb) + " MB"
	}
	if mb%1024 == 0 {
		return strconv.Itoa(mb/1024) + " GB"
	}
	return strconv.FormatFloat(float64(mb)/1024, 'f', 1, 64) + " GB"
}
