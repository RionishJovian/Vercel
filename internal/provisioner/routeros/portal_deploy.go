package routeros

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The public origin is deployment-owned, never supplied by a browser or router.
const portalOrigin = "https://megamagic.tech"
const portalHost = "megamagic.tech"

// Jellyfin's default port on the owner's LAN. Guests reach it over the business
// RouterBOARD network only; the portal never proxies video bytes.
const defaultMediaPort = "8096"

// mediaTarget validates the owner's media server. It must be a private IPv4 host
// on the business network - a public address here would expose a LAN service to
// the internet through the walled garden.
func mediaTarget(request PortalDeployment) (host, port string, err error) {
	return MediaTarget(request.MediaHost, request.MediaPort)
}

// MediaTarget validates a media server address and port. It is exported so the
// portal settings handler applies exactly the same rule as the router step: a
// private IPv4 address on the business network, and a port in range. An empty
// host is valid and means "no media service offered".
func MediaTarget(rawHost, rawPort string) (host, port string, err error) {
	host = strings.TrimSpace(rawHost)
	if host == "" {
		return "", "", nil
	}
	if strings.TrimSpace(rawPort) == "" {
		rawPort = defaultMediaPort
	} else {
		rawPort = strings.TrimSpace(rawPort)
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
		return "", "", fmt.Errorf("media host must be a private IPv4 address on your network")
	}
	if !ip.IsPrivate() {
		return "", "", fmt.Errorf("media host must be a private address on the business network, not a public one")
	}
	number, e := strconv.Atoi(rawPort)
	if e != nil || number < 1 || number > 65535 {
		return "", "", fmt.Errorf("media port must be between 1 and 65535")
	}
	return ip.To4().String(), strconv.Itoa(number), nil
}

var portalVersionPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)

type PortalDeployment struct {
	// Design is resolved from tenant-scoped saved/published configuration by the handler.
	Design       *PortalLoginDesign `json:"-"`
	Profile      string             `json:"profile"`
	Version      string             `json:"version"`
	Confirmation string             `json:"confirmation"`
	// MediaHost is the owner's LAN media server (Jellyfin) that guests on this
	// HotSpot network must be able to reach. Empty means no media service is
	// offered and the walled garden stays portal-only. Video bytes travel only on
	// the local network; nothing here enables cloud media transport.
	MediaHost string `json:"media_host"`
	MediaPort string `json:"media_port"`
}
type PortalDeploymentPlan struct {
	PreviewHTML  string   `json:"preview_html,omitempty"`
	Profile      string   `json:"profile"`
	Version      string   `json:"version"`
	Server       string   `json:"server"`
	File         string   `json:"file"`
	Steps        []string `json:"steps"`
	Confirmation string   `json:"confirmation"`
	// MediaHost and MediaPort echo the reviewed target so the operator's apply
	// call matches exactly what was previewed.
	MediaHost string `json:"media_host,omitempty"`
	MediaPort string `json:"media_port,omitempty"`
}
type PortalDeploymentResult struct {
	Verified   bool                 `json:"verified"`
	RolledBack bool                 `json:"rolled_back"`
	Backup     string               `json:"backup"`
	Plan       PortalDeploymentPlan `json:"plan"`
}

// Deployment can only target a profile used by exactly one enabled server.
const portalProfilesCommand = `:foreach id in=[/ip hotspot profile find] do={ :local profileName [/ip hotspot profile get $id name]; :if ([:len [/ip hotspot find where profile=$profileName disabled=no]]=1) do={ :put ("name: " . $profileName) } }`

// Read only fields used by deployment, avoiding script-bearing broad output.
func portalProfileCommand(selector string) string {
	return `:local id [/ip hotspot profile find where ` + selector + `]; :put ("login-by: " . [:tostr [/ip hotspot profile get $id login-by]]); :put ("html-directory: " . [:tostr [/ip hotspot profile get $id html-directory]]); :put ("html-directory-override: " . [:tostr [/ip hotspot profile get $id html-directory-override]])`
}

// Read one owned rule with scalar getters. Presentation-oriented print output
// is not a stable key/value protocol on RouterOS.
func portalGardenCommand(selector string, media bool) string {
	fields := []string{"dst-host", "dst-port", "protocol", "server", "action", "disabled"}
	if media {
		fields[0] = "dst-address"
	}
	command := `:local id [/ip hotspot walled-garden ip find where ` + selector + `]; :if ([:len $id]!=1) do={ :error "failure: ambiguous portal access rule" };`
	for _, field := range fields {
		command += ` :put (` + routerOSQuoted(field+": ") + ` . [:tostr [/ip hotspot walled-garden ip get $id ` + field + `]]);`
	}
	return command
}

// portalLoginHTML is the router-served voucher entry page. The RouterBOARD
// substitutes its $(...) macros and validates the code against its own
// hotspot user list (the app provisions each issued code as a local user), so
// the guest login never depends on the hosted site, the WAN or the tunnel.
func portalLoginHTML() string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect to WiFi</title>
<style>
/* Layered deep-slate surfaces with a sky-cyan accent (owner direction 2026-11-13). The router serves this page before the guest has
   internet, so it uses system fonts and no image: the budget is 24 KB and nothing may be fetched. Night by default; light phones get the Day twin. */
:root{color-scheme:dark;--bg:#0b0e14;--surface:#121620;--sunken:#0e121a;--ink:#f8fafc;--ink-soft:#94a3b8;--ink-mute:#7b8ba3;--line:#ffffff17;--edge:#64748b;--accent:#38bdf8;--bad:#f87171;--bad-soft:#f8717120;--mesh:#38bdf817;--glow:0 0 0 3px #38bdf82e;--shadow:inset 0 1px 0 #ffffff0f,0 20px 40px -15px #0009;--portal-brand:#38bdf8;--portal-tint:#121620;--portal-on-brand:#06131f}
@media(prefers-color-scheme:light){:root{color-scheme:light;--bg:#f8fafc;--surface:#ffffff;--sunken:#f1f5f9;--ink:#0f172a;--ink-soft:#475569;--ink-mute:#5b6b80;--line:#e2e8f0;--edge:#64748b;--accent:#0369a1;--bad:#b91c1c;--bad-soft:#b91c1c14;--mesh:#38bdf81f;--glow:0 0 0 3px #0369a129;--shadow:0 1px 3px #0f172a0f,0 8px 24px -8px #0f172a14}}
*{box-sizing:border-box}html{min-height:100%;background:var(--bg)}
body{margin:0;min-height:100vh;min-height:100svh;display:flex;align-items:center;justify-content:center;padding:32px 20px;background:radial-gradient(ellipse 75% 45% at 50% -10%,var(--mesh),transparent) no-repeat,var(--bg);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Noto Sans Myanmar",system-ui,sans-serif;-webkit-font-smoothing:antialiased}
.card{width:100%;max-width:400px;padding:32px 28px;text-align:left;border-radius:12px;border:1px solid var(--line);background:color-mix(in srgb,var(--portal-tint) 6%,var(--surface));box-shadow:var(--shadow)}
.wifi-mark{display:block;width:40px;height:40px;margin:0 0 16px;color:var(--accent)}
h1{font-size:28px;line-height:1.15;font-weight:650;letter-spacing:-.028em;margin:0 0 8px;color:var(--ink)}
p{color:var(--ink-soft);font-size:15px;line-height:1.6;margin:0 0 24px}
label{display:block}.field-label{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
input{display:block;width:100%;min-height:60px;padding:0 16px;border-radius:8px;border:1px solid var(--edge);background:var(--sunken);color:var(--ink);font:600 22px ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.14em;text-transform:uppercase}
input::placeholder{color:var(--ink-mute);letter-spacing:.3em}
input:focus{outline:none;border-color:var(--accent);box-shadow:var(--glow)}
button:focus-visible,a.alt:focus-visible{outline:2px solid var(--accent);outline-offset:3px}
button{width:100%;min-height:52px;margin-top:16px;padding:0 16px;border:1px solid color-mix(in srgb,var(--portal-brand) 70%,var(--ink));border-radius:8px;background-color:var(--portal-brand);background-image:linear-gradient(180deg,#ffffff33,#fff0);box-shadow:inset 0 1px 0 #ffffff40;color:var(--portal-on-brand);font:inherit;font-size:16px;font-weight:650;cursor:pointer;transition:filter .16s}
button:hover{filter:brightness(1.07)}button:disabled{cursor:wait;opacity:.7}
.brand{display:block;margin:24px 0 0;font-size:13px;line-height:1.5;color:var(--ink-mute);overflow-wrap:anywhere}
a.alt{display:flex;align-items:center;justify-content:center;min-height:48px;margin-top:12px;border:1px solid var(--edge);border-radius:8px;color:var(--ink);font-size:15px;font-weight:600;text-decoration:none}
.err{margin:12px 0;padding:10px 12px;border-radius:8px;background:var(--bad-soft);border:1px solid var(--bad);color:var(--bad);font-size:14px;line-height:1.5;text-align:left}
.consent{display:flex;align-items:flex-start;gap:10px;text-align:left;font-size:14px;line-height:1.5;margin:16px 0 0;color:var(--ink-soft)}.consent input{width:22px;min-height:22px;height:22px;padding:0;margin:0;flex-shrink:0;accent-color:var(--accent)}
details{margin:0 0 16px;font-size:14px;line-height:1.6;color:var(--ink-soft);text-align:left}details p{white-space:pre-wrap;margin:10px 0;font-size:14px}summary{cursor:pointer;min-height:44px;display:flex;align-items:center}
.card{position:relative;z-index:1}.bg-art{position:fixed;inset:0;width:100%;height:100%;z-index:0;pointer-events:none;opacity:.22}
.sb{stroke:var(--portal-brand);fill:none}.fb{fill:var(--portal-brand);stroke:none}
.lang{display:flex;justify-content:flex-end;gap:6px;margin:0 0 14px;order:-3}
.lang button{width:auto;min-height:44px;margin:0;padding:0 14px;border:1px solid var(--edge);border-radius:999px;background:none;box-shadow:none;color:var(--ink);font-size:14px;font-weight:600}
.lang button[aria-pressed=true]{background:var(--portal-brand);border-color:var(--portal-brand);color:var(--portal-on-brand)}.lang button:hover{filter:none}
html[lang=my] p,html[lang=my] label,html[lang=my] summary,html[lang=my] button,html[lang=my] a.alt,html[lang=my] .consent{line-height:1.75}html[lang=my] h1{line-height:1.6}
@media(max-height:560px){body{align-items:flex-start;padding-top:24px}}
@media(prefers-reduced-motion:reduce){button{transition:none}}
</style></head><body><main class="card">
<h1 data-t="title">Wi-Fi Access</h1>
<p data-t="welcome">Enter voucher code to connect</p>
$(if error)<p class="err">$(error)</p>$(endif)
<form id="sendin" name="sendin" action="$(link-login-only)" method="post" hidden>
<input name="username" autocomplete="off">
<input name="password" type="hidden">
<input type="hidden" name="dst" value="$(link-orig)">
<input type="hidden" name="popup" value="true">
</form>
<form id="voucher-login" action="$(link-login-only)" method="post" onsubmit="return doLogin()" novalidate>
<label class="field-label" for="voucher" data-t="voucher">Voucher code</label>
<input id="voucher" name="voucher" type="text" placeholder="••••••••" autocapitalize="characters" autocorrect="off" autocomplete="off" spellcheck="false" aria-describedby="login-error">
<p id="login-error" class="err" role="alert" hidden></p>
<button id="connect" type="submit" data-t="connect">Connect to Wi-Fi</button>
</form>
</main>
$(if chap-id)<script src="md5.js"></script>$(endif)
<script>
var loginPending=false;
var loginTimer;
var BI=false,PREF='';
var T={en:{title:'Wi-Fi Access',welcome:'Enter voucher code to connect',voucher:'Voucher code',connect:'Connect to Wi-Fi',connecting:'Connecting\u2026',terms:'Internet access terms',pay:'Pay by bank transfer',consent:'I agree to the internet access terms.',s1:'Type your voucher code',s2:'Tap Connect',s3:'You are online',link:'Fast satellite link',passes:'Guest passes',badCode:'Enter the complete voucher code as printed.',needConsent:'Accept the internet access terms before connecting.',preview:'Preview only. No voucher was used and no internet access was granted.',noLoader:'WiFi login could not load. Reload this page and try again.',noResponse:'No login response received. Check your WiFi connection and try again.',sendFail:'Could not send your code. Check your WiFi connection and try again.'},
my:{title:'Wi-Fi အသုံးပြုရန်',welcome:'ဘောက်ချာကုဒ် ထည့်ပြီး ချိတ်ဆက်ပါ',voucher:'ဘောက်ချာကုဒ်',connect:'Wi-Fi ချိတ်ဆက်မည်',connecting:'ချိတ်ဆက်နေသည်\u2026',terms:'အင်တာနက် အသုံးပြုမှု စည်းကမ်းချက်များ',pay:'ဘဏ်လွှဲ၍ ပေးချေမည်',consent:'အင်တာနက် အသုံးပြုမှု စည်းကမ်းချက်များကို သဘောတူပါသည်။',s1:'ဘောက်ချာကုဒ် ရိုက်ထည့်ပါ',s2:'ချိတ်ဆက်မည် ကို နှိပ်ပါ',s3:'အင်တာနက် သုံးနိုင်ပါပြီ',link:'ဂြိုဟ်တုလင့်ခ် မြန်နှုန်းမြင့်',passes:'ဧည့်သည် အသုံးပြုခွင့်များ',badCode:'စာရွက်ပေါ်ပါအတိုင်း ကုဒ်အပြည့်အစုံ ထည့်ပါ။',needConsent:'ချိတ်ဆက်မီ စည်းကမ်းချက်များကို လက်ခံပါ။',preview:'နမူနာကြည့်ရန်သာ ဖြစ်သည်။ ဘောက်ချာ မသုံးရသေးပါ၊ အင်တာနက်လည်း မရပါ။',noLoader:'Wi-Fi ဝင်ရောက်မှု မတင်နိုင်ပါ။ စာမျက်နှာကို ပြန်ဖွင့်ပြီး ထပ်ကြိုးစားပါ။',noResponse:'ဖြေကြားချက် မရရှိပါ။ Wi-Fi ချိတ်ဆက်မှုကို စစ်ပြီး ထပ်ကြိုးစားပါ။',sendFail:'ကုဒ်ကို မပို့နိုင်ပါ။ Wi-Fi ချိတ်ဆက်မှုကို စစ်ပြီး ထပ်ကြိုးစားပါ။'}};
var lang=PREF||((typeof navigator!=='undefined'&&/^my/i.test(navigator.language||''))?'my':'en');
function tr(k){
  if(BI&&(k==='connect'||k==='pay')){return T.my[k]+' / '+T.en[k];}
  return (T[lang]&&T[lang][k])||T.en[k]||'';
}
function setLang(l){
  lang=l;
  if(!document.querySelectorAll){return;}
  if(document.documentElement&&document.documentElement.setAttribute){document.documentElement.setAttribute('lang',l);}
  var nodes=document.querySelectorAll('[data-t]');
  for(var i=0;i<nodes.length;i++){
    if(nodes[i].id==='connect'&&loginPending){continue;}
    nodes[i].textContent=tr(nodes[i].getAttribute('data-t'));
  }
  var my=document.getElementById('lang-my'),en=document.getElementById('lang-en');
  if(my){my.setAttribute('aria-pressed',l==='my'?'true':'false');}
  if(en){en.setAttribute('aria-pressed',l==='en'?'true':'false');}
}
function resetLogin(message){
  window.clearTimeout(loginTimer);
  loginPending=false;
  document.getElementById('voucher').disabled=false;
  var button=document.getElementById('connect');
  button.disabled=false;
  button.textContent=tr('connect');
  var error=document.getElementById('login-error');
  error.textContent=message||'';
  error.hidden=!message;
}
function doLogin(){
  if(loginPending){return false;}
  var input=document.getElementById('voucher');
  var code=input.value.trim().toUpperCase();
  input.value=code;
  if(!/^[A-Z0-9][A-Z0-9_-]{3,31}$/.test(code)){
    resetLogin(tr('badCode'));
    input.focus();
    return false;
  }
  var consent=document.getElementById('voucher-consent');
  if(consent && !consent.checked){resetLogin(tr('needConsent'));return false;}
  if(document.body && document.body.getAttribute('data-preview')==='true'){resetLogin(tr('preview'));return false;}
  var chapId='$(chap-id)';
  var password=code;
  if(chapId!==''){
    if(typeof hexMD5!=='function'){
      resetLogin(tr('noLoader'));
      return false;
    }
    try{password=hexMD5(chapId+code+'$(chap-challenge)');}
    catch(error){
      resetLogin(tr('noLoader'));
      return false;
    }
  }
  var sendin=document.getElementById('sendin');
  sendin.elements.username.value=code;
  sendin.elements.password.value=password;
  loginPending=true;
  input.disabled=true;
  var button=document.getElementById('connect');
  button.disabled=true;
  button.textContent=tr('connecting');
  document.getElementById('login-error').hidden=true;
  loginTimer=window.setTimeout(function(){
    resetLogin(tr('noResponse'));
  },15000);
  try{sendin.submit();}
  catch(error){resetLogin(tr('sendFail'));}
  return false;
}
window.addEventListener('pageshow',function(){resetLogin();});
setLang(lang);
</script>
</body></html>`
}

func (a *DeviceApplier) PortalProfiles(ctx context.Context, businessID, deviceID string) ([]string, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return nil, err
	}
	out, err := r.Run(ctx, portalProfilesCommand)
	if err != nil {
		return nil, fmt.Errorf("could not read HotSpot profiles")
	}
	profiles := []string{}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == "name" && gatewayInterfacePattern.MatchString(strings.TrimSpace(value)) {
			profiles = append(profiles, strings.TrimSpace(value))
		}
	}
	return profiles, nil
}

type portalState struct {
	plan                                    PortalDeploymentPlan
	original, replacement, selector, garden string
	mediaGarden                             string
	mediaHost, mediaPort                    string
	directory, destination                  string
	needsCHAP                               bool
	hasGarden                               bool
}

// Preview is read-only. Deployment clones the complete HTML directory so
// profiles sharing the original files are never changed as a side effect.
func portalPreflight(ctx context.Context, r CommandRunner, businessID, deviceID string, request PortalDeployment) (portalState, error) {
	s := portalState{}
	if !gatewayInterfacePattern.MatchString(request.Profile) || !portalVersionPattern.MatchString(request.Version) {
		return s, fmt.Errorf("select a HotSpot profile")
	}
	s.selector = "name=" + routerOSQuoted(request.Profile)
	count, err := r.Run(ctx, "/ip hotspot profile print count-only where "+s.selector)
	if err != nil || strings.TrimSpace(count) != "1" {
		return s, fmt.Errorf("HotSpot profile not found or ambiguous")
	}
	out, err := r.Run(ctx, portalProfileCommand(s.selector))
	if err != nil {
		return s, fmt.Errorf("could not read HotSpot profile")
	}
	p := parseValueList(out)
	p["login-by"] = strings.ReplaceAll(p["login-by"], ";", ",")
	if !strings.Contains(","+p["login-by"]+",", ",mac,") {
		return s, fmt.Errorf("configure MAC authentication for managed guests in gateway setup first")
	}
	// The deployed login page authenticates against the RouterBOARD's own user
	// list, so the profile must accept a form login (HTTP CHAP or PAP). Without
	// one, a guest's voucher code cannot be validated locally at all.
	if !strings.Contains(","+p["login-by"]+",", ",http-chap,") && !strings.Contains(","+p["login-by"]+",", ",http-pap,") {
		return s, fmt.Errorf("voucher login needs HTTP CHAP or PAP enabled on this HotSpot profile")
	}
	if v := p["html-directory-override"]; v != "" && v != "none" {
		return s, fmt.Errorf("external HTML overrides require operator-assisted deployment")
	}
	directory := strings.Trim(p["html-directory"], "/")
	if directory == "" || strings.Contains(directory, "..") {
		return s, fmt.Errorf("invalid HotSpot HTML directory")
	}
	s.directory = p["html-directory"]
	s.needsCHAP = strings.Contains(","+p["login-by"]+",", ",http-chap,")
	if s.needsCHAP {
		out, err = r.Run(ctx, "/file print count-only where name="+routerOSQuoted(directory+"/md5.js"))
		if err != nil || strings.TrimSpace(out) != "1" {
			return s, fmt.Errorf("HotSpot CHAP helper md5.js not found; restore the profile's local HotSpot files first")
		}
	}
	s.destination = "mm-portal-" + request.Version
	out, err = r.Run(ctx, "/file print count-only where name=\"flash\"")
	if err != nil || (strings.TrimSpace(out) != "0" && strings.TrimSpace(out) != "1") {
		return s, fmt.Errorf("could not inspect persistent storage")
	}
	if strings.TrimSpace(out) == "1" {
		s.destination = "flash/" + s.destination
	}
	out, err = r.Run(ctx, "/file print count-only where name="+routerOSQuoted(s.destination))
	if err != nil || strings.TrimSpace(out) != "0" {
		return s, fmt.Errorf("staging directory already exists; operator review required")
	}
	out, err = r.Run(ctx, "/ip hotspot print count-only where profile="+routerOSQuoted(request.Profile)+" disabled=no")
	if err != nil || strings.TrimSpace(out) != "1" {
		return s, fmt.Errorf("select a profile used by exactly one enabled HotSpot server")
	}
	out, err = r.Run(ctx, `:put ("name: " . [/ip hotspot get [find where profile=`+routerOSQuoted(request.Profile)+` disabled=no] name])`)
	if err != nil {
		return s, fmt.Errorf("could not read HotSpot server")
	}
	server := parseValueList(out)["name"]
	if !gatewayInterfacePattern.MatchString(server) {
		return s, fmt.Errorf("invalid HotSpot server")
	}
	s.plan = PortalDeploymentPlan{Profile: request.Profile, Version: request.Version, Server: server, File: s.destination + "/login.html", MediaHost: s.mediaHost, MediaPort: s.mediaPort}
	file := "name=" + routerOSQuoted(directory+"/login.html")
	out, err = r.Run(ctx, "/file print count-only where "+file)
	if err != nil || strings.TrimSpace(out) != "1" {
		return s, fmt.Errorf("existing login.html not found; initialize HotSpot HTML files first")
	}
	out, err = r.Run(ctx, ":put [/file get [find "+file+"] contents]")
	if err != nil || len(out) > 24000 || strings.TrimSpace(out) == "" {
		return s, fmt.Errorf("login page could not be read safely (maximum 24 KB)")
	}
	s.original = strings.TrimSuffix(strings.TrimSuffix(out, "\n"), "\r")
	var renderErr error
	s.replacement, renderErr = RenderPortalLogin(request.Design, false)
	if renderErr != nil {
		return s, renderErr
	}
	s.plan.PreviewHTML, renderErr = RenderPortalLogin(request.Design, true)
	if renderErr != nil {
		return s, renderErr
	}
	s.garden = "comment=" + routerOSQuoted("mm-portal-"+request.Profile)
	mediaHost, mediaPort, mediaErr := mediaTarget(request)
	if mediaErr != nil {
		return s, mediaErr
	}
	s.mediaHost, s.mediaPort = mediaHost, mediaPort
	s.plan.MediaHost, s.plan.MediaPort = mediaHost, mediaPort
	if mediaHost != "" {
		s.mediaGarden = "comment=" + routerOSQuoted("mm-media-"+request.Profile)
	}
	out, err = r.Run(ctx, "/ip hotspot walled-garden ip print count-only where "+s.garden)
	if err != nil {
		return s, fmt.Errorf("could not inspect portal access rule")
	}
	s.hasGarden = strings.TrimSpace(out) == "1"
	if s.hasGarden {
		out, err = r.Run(ctx, portalGardenCommand(s.garden, false))
		g := parseValueList(out)
		if err != nil || g["dst-host"] != portalHost || g["dst-port"] != "443" || g["protocol"] != "tcp" || g["server"] != server || g["action"] != "accept" || g["disabled"] == "yes" || g["disabled"] == "true" {
			return s, fmt.Errorf("existing managed portal rule differs; operator review required")
		}
	} else if strings.TrimSpace(out) != "0" {
		return s, fmt.Errorf("ambiguous managed portal access rule")
	}
	steps := []string{"Clone all existing HotSpot files into a separate persistent directory; retain the original directory for rollback", "Allow TCP 443 to megamagic.tech for HotSpot " + server + " (paid-plan continuation and hosted pages; voucher login itself stays router-local)"}
	if s.mediaHost != "" {
		steps = append(steps, "Allow guests on this HotSpot to reach the media server at "+s.mediaHost+":"+s.mediaPort+" on your local network")
	}
	steps = append(steps, "Install the router-served voucher login page and switch only profile "+request.Profile, "Read back and verify the login page and profile switch; the RouterBOARD validates issued voucher codes against its own user list")
	s.plan.Steps = steps
	encoded, _ := json.Marshal([]any{businessID, deviceID, s.plan, s.original, s.replacement, s.hasGarden})
	digest := sha256.Sum256(encoded)
	s.plan.Confirmation = hex.EncodeToString(digest[:])
	return s, nil
}

// How often, and how many times, to read a staged folder back before giving up on it.
var portalCopyWait = 2 * time.Second

const portalCopyChecks = 5

// portalCopyLanded reports whether the staging folder holds as many files as its source and the same login page.
func portalCopyLanded(ctx context.Context, r CommandRunner, s portalState) bool {
	count := func(folder string) string {
		out, err := r.Run(ctx, "/file print count-only where name~"+routerOSQuoted("^"+regexp.QuoteMeta(strings.Trim(folder, "/"))+"/"))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	source, copied := count(s.directory), count(s.destination)
	if source == "" || source == "0" || source != copied {
		return false
	}
	out, err := r.Run(ctx, ":put [/file get [find name="+routerOSQuoted(s.plan.File)+"] contents]")
	return err == nil && strings.TrimSpace(out) == strings.TrimSpace(s.original)
}

func (a *DeviceApplier) PreviewPortal(ctx context.Context, businessID, deviceID string, request PortalDeployment) (PortalDeploymentPlan, error) {
	version := make([]byte, 8)
	if _, err := rand.Read(version); err != nil {
		return PortalDeploymentPlan{}, fmt.Errorf("could not prepare deployment version")
	}
	request.Version = hex.EncodeToString(version)
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return PortalDeploymentPlan{}, err
	}
	s, err := portalPreflight(ctx, r, businessID, deviceID, request)
	return s.plan, err
}

func (a *DeviceApplier) ApplyPortal(ctx context.Context, businessID, deviceID string, request PortalDeployment) (PortalDeploymentResult, error) {
	result := PortalDeploymentResult{}
	lock, _ := gatewayLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return result, err
	}
	s, err := portalPreflight(ctx, r, businessID, deviceID, request)
	result.Plan = s.plan
	if err != nil {
		return result, err
	}
	if request.Confirmation == "" || request.Confirmation != s.plan.Confirmation {
		return result, fmt.Errorf("router state changed; preview again before deployment")
	}
	// Preserve the complete original directory; stage before switching the profile.
	result.Backup = s.destination + "/login-backup-" + request.Version + ".html"
	file := "name=" + routerOSQuoted(s.plan.File)
	// Nothing points at the staging folder yet, so a failed attempt can take it away rather than leave it on the router's storage.
	discard := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = r.Run(cleanup, "/file remove [find where name="+routerOSQuoted(s.destination)+"]")
	}
	// The copy's own result is not the answer: RouterOS 7.24.6 can finish the files and then hold the command past the SSH timeout. What is on the router decides.
	_, _ = r.Run(ctx, "/file copy "+routerOSQuoted(s.directory)+" name="+routerOSQuoted(s.destination))
	landed := portalCopyLanded(ctx, r, s)
	for i := 1; !landed && i < portalCopyChecks; i++ {
		select {
		case <-ctx.Done():
			discard()
			return result, fmt.Errorf("HotSpot directory copy did not verify; active profile unchanged")
		case <-time.After(portalCopyWait):
		}
		landed = portalCopyLanded(ctx, r, s)
	}
	if !landed {
		discard()
		return result, fmt.Errorf("HotSpot directory copy did not verify; active profile unchanged. Check the router's free storage and try again")
	}
	var out string
	if s.needsCHAP {
		out, err = r.Run(ctx, "/file print count-only where name="+routerOSQuoted(s.destination+"/md5.js"))
		if err != nil || strings.TrimSpace(out) != "1" {
			discard()
			return result, fmt.Errorf("copied HotSpot CHAP helper md5.js did not verify; active profile unchanged")
		}
	}
	// Older backups stay in their own folders. Left in this copy, every install would be bigger than the last until the copy above starts to stall. Best effort: a leftover backup harms nothing.
	_, _ = r.Run(ctx, "/file remove [find where name~"+routerOSQuoted("^"+regexp.QuoteMeta(s.destination)+"/login-backup-")+"]")
	// Keep an original login-page copy in persistent staging as well, because
	// the old directory can be RAM-backed on devices with a flash disk.
	_, err = r.Run(ctx, "/file copy "+routerOSQuoted(strings.Trim(s.directory, "/")+"/login.html")+" name="+routerOSQuoted(result.Backup))
	if err == nil {
		out, err = r.Run(ctx, ":put [/file get [find name="+routerOSQuoted(result.Backup)+"] contents]")
	}
	if err != nil || strings.TrimSpace(out) != strings.TrimSpace(s.original) {
		discard()
		return result, fmt.Errorf("persistent login backup did not verify; active profile unchanged")
	}
	addedGarden := false
	addedMediaGarden := false
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, e := r.Run(cleanup, "/ip hotspot profile set [find "+s.selector+"] html-directory="+routerOSQuoted(s.directory))
		out, readErr := r.Run(cleanup, portalProfileCommand(s.selector))
		ok := e == nil && readErr == nil && parseValueList(out)["html-directory"] == s.directory
		if addedMediaGarden {
			_, e = r.Run(cleanup, "/ip hotspot walled-garden ip remove [find "+s.mediaGarden+"]")
			out, readErr = r.Run(cleanup, "/ip hotspot walled-garden ip print count-only where "+s.mediaGarden)
			ok = ok && e == nil && readErr == nil && strings.TrimSpace(out) == "0"
		}
		if addedGarden {
			_, e = r.Run(cleanup, "/ip hotspot walled-garden ip remove [find "+s.garden+"]")
			out, readErr = r.Run(cleanup, "/ip hotspot walled-garden ip print count-only where "+s.garden)
			ok = ok && e == nil && readErr == nil && strings.TrimSpace(out) == "0"
		}
		result.RolledBack = ok
	}
	if s.mediaHost != "" {
		if out, e := r.Run(ctx, "/ip hotspot walled-garden ip print count-only where "+s.mediaGarden); e != nil || strings.TrimSpace(out) != "0" {
			rollback()
			return result, fmt.Errorf("managed media access rule already exists; operator review required")
		}
		addedMediaGarden = true // Contain a write even if its SSH response is lost.
		_, err = r.Run(ctx, "/ip hotspot walled-garden ip add action=accept disabled=no protocol=tcp dst-port="+s.mediaPort+" dst-address="+routerOSQuoted(s.mediaHost)+" server="+routerOSQuoted(s.plan.Server)+" comment="+routerOSQuoted("mm-media-"+s.plan.Profile))
		if err != nil {
			rollback()
			return result, fmt.Errorf("media access rule failed; check rollback status")
		}
		out, e := r.Run(ctx, portalGardenCommand(s.mediaGarden, true))
		g := parseValueList(out)
		if e != nil || g["dst-address"] != s.mediaHost || g["dst-port"] != s.mediaPort || g["protocol"] != "tcp" || g["action"] != "accept" {
			rollback()
			return result, fmt.Errorf("media access rule verification failed; check rollback status")
		}
	}
	if !s.hasGarden {
		addedGarden = true
		_, err = r.Run(ctx, "/ip hotspot walled-garden ip add action=accept disabled=no protocol=tcp dst-port=443 dst-host="+routerOSQuoted(portalHost)+" server="+routerOSQuoted(s.plan.Server)+" comment="+routerOSQuoted("mm-portal-"+request.Profile))
		if err != nil {
			rollback()
			return result, fmt.Errorf("portal access rule failed; check rollback status")
		}
	}
	_, err = r.Run(ctx, "/file set [find "+file+"] contents="+routerOSQuoted(s.replacement))
	if err != nil {
		rollback()
		return result, fmt.Errorf("login page write failed; check rollback status")
	}
	out, err = r.Run(ctx, ":put [/file get [find "+file+"] contents]")
	if err != nil || strings.TrimSpace(out) != s.replacement {
		rollback()
		return result, fmt.Errorf("login page verification failed; check rollback status")
	}
	out, err = r.Run(ctx, portalGardenCommand(s.garden, false))
	g := parseValueList(out)
	if err != nil || g["dst-host"] != portalHost || g["dst-port"] != "443" || g["protocol"] != "tcp" || g["server"] != s.plan.Server || g["action"] != "accept" || g["disabled"] == "yes" || g["disabled"] == "true" {
		rollback()
		return result, fmt.Errorf("portal rule verification failed; check rollback status")
	}
	_, err = r.Run(ctx, "/ip hotspot profile set [find "+s.selector+"] html-directory="+routerOSQuoted(s.destination))
	if err == nil {
		out, err = r.Run(ctx, portalProfileCommand(s.selector))
	}
	if err != nil || parseValueList(out)["html-directory"] != s.destination {
		rollback()
		return result, fmt.Errorf("portal profile switch did not verify; check rollback status")
	}
	result.Verified = true
	return result, nil
}
