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

const portalProfilesCommand = `:foreach id in=[/ip hotspot profile find] do={ :put ("name: " . [/ip hotspot profile get $id name]) }`

// Read only fields used by deployment, avoiding script-bearing broad output.
func portalProfileCommand(selector string) string {
	return `:local id [/ip hotspot profile find where ` + selector + `]; :put ("login-by: " . [/ip hotspot profile get $id login-by]); :put ("html-directory: " . [/ip hotspot profile get $id html-directory]); :put ("html-directory-override: " . [/ip hotspot profile get $id html-directory-override])`
}

// portalLoginHTML is the router-served voucher entry page. The RouterBOARD
// substitutes its $(...) macros and validates the code against its own
// hotspot user list (the app provisions each issued code as a local user), so
// the guest login never depends on the hosted site, the WAN or the tunnel.
func portalLoginHTML() string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect to WiFi</title>
<style>
:root{color-scheme:dark;--portal-brand:#075044;--portal-tint:#061f26;--portal-on-brand:#fff}
*{box-sizing:border-box}html{min-height:100%;background:#051b22}
body{margin:0;min-height:100vh;min-height:100svh;display:flex;align-items:center;justify-content:center;padding:32px 20px;background:#051b22;color:#edf7f5;font-family:-apple-system,BlinkMacSystemFont,"SF Pro Display","Inter","Segoe UI",sans-serif;-webkit-font-smoothing:antialiased;background-position:center;background-size:cover;background-repeat:no-repeat}
.card{position:relative;width:76%;max-width:340px;padding:34px 25px 32px;text-align:center;border-radius:33px;background:rgba(7,33,37,.46);border:1px solid #efffffb3;box-shadow:0 24px 65px #0008,0 0 0 2px #b3d3d336,inset 0 1px 2px #fff9,inset 2px 0 5px #e3f7ff55,inset -2px -2px 5px #efffde55;-webkit-backdrop-filter:blur(18px) saturate(125%);backdrop-filter:blur(18px) saturate(125%)}
.card:before{content:"";position:absolute;pointer-events:none;inset:3px;border:1px solid #e0f7f829;border-radius:29px}
.wifi-mark{display:block;width:98px;height:80px;object-fit:contain;margin:0 auto 10px}
h1{font-size:28px;line-height:1.2;font-weight:600;letter-spacing:-.035em;margin:0 0 10px;color:#f0f7f5}
p{color:#d4e3df;font-size:11.5px;line-height:1.6;margin:0 0 22px}
label{display:block}.field-label{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
input{display:block;width:100%;min-height:46px;padding:12px 14px;border-radius:13px;border:1px solid #dcf4f36b;background:#a1cecc0c;box-shadow:inset 0 1px 2px #ffffff12;color:#f1faf7;font:inherit;font-size:15px;letter-spacing:.1em;text-transform:uppercase}
input::placeholder{color:#a8cecb;letter-spacing:.28em;opacity:.9}
input:focus,button:focus-visible{outline:2px solid #bbfbe6;outline-offset:4px}
button{width:100%;min-height:50px;margin-top:23px;padding:12px 8px;border:1px solid #b4ffe0b3;border-radius:13px;background-color:var(--portal-brand);background-image:linear-gradient(150deg,#c2fff41f,#ffffff00 48%,#001c1a55);box-shadow:0 0 16px #48deb63b,inset 0 1px 3px #cafffca1,inset 0 -2px 4px #cafffb24;color:var(--portal-on-brand);font:inherit;font-size:15px;font-weight:500;cursor:pointer;transition:filter .15s,box-shadow .15s}
button:hover{filter:brightness(1.12);box-shadow:0 0 22px #48deb659,inset 0 1px 3px #cafffca1}button:disabled{cursor:wait;opacity:.7}
.brand{display:block;margin:17px 0 -10px;font-size:9px;line-height:1.5;letter-spacing:.12em;color:#c6d8d2;text-transform:uppercase;overflow-wrap:anywhere}
.err{margin:12px 0;padding:10px 12px;border-radius:11px;background:#431e2de8;border:1px solid #ffbdca66;color:#ffe1e8;font-size:12px;line-height:1.5;text-align:left}
.consent{display:flex;align-items:flex-start;gap:8px;text-align:left;font-size:11px;line-height:1.5;margin:18px 0 -8px;color:#d4e3df}.consent input{width:17px;min-height:17px;height:17px;padding:0;margin:1px 0;flex-shrink:0;accent-color:var(--portal-brand)}
details{margin:0 0 17px;font-size:11px;line-height:1.6;color:#d4e3df;text-align:left}details p{white-space:pre-wrap;margin:10px 0}
@media(max-width:360px){.card{width:100%;padding:28px 24px}h1{font-size:27px}}
@media(min-width:600px){.card{width:340px;padding:40px 36px}.wifi-mark{width:86px;height:72px}h1{font-size:32px}p{font-size:13px}button{font-size:16px}}
@media(max-height:560px){body{align-items:flex-start;padding-top:24px}.wifi-mark{height:44px;margin-bottom:12px}.card{padding-top:22px;padding-bottom:22px}}
@media(prefers-reduced-transparency:reduce){.card{background:#09272e;-webkit-backdrop-filter:none;backdrop-filter:none}}
@media(prefers-reduced-motion:reduce){button{transition:none}}
</style></head><body><main class="card">
<h1>Wi-Fi Access</h1>
<p>Enter voucher code to connect</p>
$(if error)<p class="err">$(error)</p>$(endif)
<form id="sendin" name="sendin" action="$(link-login-only)" method="post" hidden>
<input name="username" autocomplete="off">
<input name="password" type="hidden">
<input type="hidden" name="dst" value="$(link-orig)">
<input type="hidden" name="popup" value="true">
</form>
<form id="voucher-login" action="$(link-login-only)" method="post" onsubmit="return doLogin()" novalidate>
<label class="field-label" for="voucher">Voucher code</label>
<input id="voucher" name="voucher" type="text" placeholder="••••••••" autocapitalize="characters" autocorrect="off" autocomplete="off" spellcheck="false" aria-describedby="login-error">
<p id="login-error" class="err" role="alert" hidden></p>
<button id="connect" type="submit">Connect to Wi-Fi</button>
</form>
</main>
$(if chap-id)<script src="md5.js"></script>$(endif)
<script>
var loginPending=false;
var loginTimer;
function resetLogin(message){
  window.clearTimeout(loginTimer);
  loginPending=false;
  document.getElementById('voucher').disabled=false;
  var button=document.getElementById('connect');
  button.disabled=false;
  button.textContent='Connect to Wi-Fi';
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
    resetLogin('Enter the complete voucher code as printed.');
    input.focus();
    return false;
  }
  var consent=document.getElementById('voucher-consent');
  if(consent && !consent.checked){resetLogin('Accept the internet access terms before connecting.');return false;}
  if(document.body && document.body.getAttribute('data-preview')==='true'){resetLogin('Preview only. No voucher was used and no internet access was granted.');return false;}
  var chapId='$(chap-id)';
  var password=code;
  if(chapId!==''){
    if(typeof hexMD5!=='function'){
      resetLogin('WiFi login could not load. Reload this page and try again.');
      return false;
    }
    try{password=hexMD5(chapId+code+'$(chap-challenge)');}
    catch(error){
      resetLogin('WiFi login could not load. Reload this page and try again.');
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
  button.textContent='Connecting…';
  document.getElementById('login-error').hidden=true;
  loginTimer=window.setTimeout(function(){
    resetLogin('No login response received. Check your WiFi connection and try again.');
  },15000);
  try{sendin.submit();}
  catch(error){resetLogin('Could not send your code. Check your WiFi connection and try again.');}
  return false;
}
window.addEventListener('pageshow',function(){resetLogin();});
</script>
</body></html>`
}

func (a *DeviceApplier) PortalProfiles(ctx context.Context, businessID, deviceID string) ([]string, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return nil, err
	}
	out, err := r.Run(ctx, "/ip hotspot profile print value-list detail")
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
	s.garden = "comment=" + routerOSQuoted("mm-portal-"+request.Profile) + " dynamic=no"
	mediaHost, mediaPort, mediaErr := mediaTarget(request)
	if mediaErr != nil {
		return s, mediaErr
	}
	s.mediaHost, s.mediaPort = mediaHost, mediaPort
	s.plan.MediaHost, s.plan.MediaPort = mediaHost, mediaPort
	if mediaHost != "" {
		s.mediaGarden = "comment=" + routerOSQuoted("mm-media-"+request.Profile) + " dynamic=no"
	}
	out, err = r.Run(ctx, "/ip hotspot walled-garden ip print count-only where "+s.garden)
	if err != nil {
		return s, fmt.Errorf("could not inspect portal access rule")
	}
	s.hasGarden = strings.TrimSpace(out) == "1"
	if s.hasGarden {
		out, err = r.Run(ctx, "/ip hotspot walled-garden ip print value-list detail where "+s.garden)
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
	_, err = r.Run(ctx, "/file copy "+routerOSQuoted(s.directory)+" name="+routerOSQuoted(s.destination))
	var out string
	if err == nil {
		out, err = r.Run(ctx, ":put [/file get [find "+file+"] contents]")
	}
	if err != nil || strings.TrimSpace(out) != strings.TrimSpace(s.original) {
		return result, fmt.Errorf("HotSpot directory copy did not verify; active profile unchanged. RouterOS file copy support is required")
	}
	if s.needsCHAP {
		out, err = r.Run(ctx, "/file print count-only where name="+routerOSQuoted(s.destination+"/md5.js"))
		if err != nil || strings.TrimSpace(out) != "1" {
			return result, fmt.Errorf("copied HotSpot CHAP helper md5.js did not verify; active profile unchanged")
		}
	}
	// Keep an original login-page copy in persistent staging as well, because
	// the old directory can be RAM-backed on devices with a flash disk.
	_, err = r.Run(ctx, "/file copy "+routerOSQuoted(strings.Trim(s.directory, "/")+"/login.html")+" name="+routerOSQuoted(result.Backup))
	if err == nil {
		out, err = r.Run(ctx, ":put [/file get [find name="+routerOSQuoted(result.Backup)+"] contents]")
	}
	if err != nil || strings.TrimSpace(out) != strings.TrimSpace(s.original) {
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
		out, e := r.Run(ctx, "/ip hotspot walled-garden ip print value-list detail where "+s.mediaGarden)
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
	out, err = r.Run(ctx, "/ip hotspot walled-garden ip print value-list detail where "+s.garden)
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
