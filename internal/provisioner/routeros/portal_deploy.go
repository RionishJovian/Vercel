package routeros

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The public origin is deployment-owned, never supplied by a browser or router.
const portalOrigin = "https://mikromagic.com"
const portalHost = "mikromagic.com"

// Jellyfin's default port on the owner's LAN. Guests reach it over the business
// RouterBOARD network only; the portal never proxies video bytes.
const defaultMediaPort = "8096"

// mediaTarget validates the owner's media server. It must be a private IPv4 host
// on the business network - a public address here would expose a LAN service to
// the internet through the walled garden.
func mediaTarget(request PortalDeployment) (host, port string, err error) {
	host = strings.TrimSpace(request.MediaHost)
	if host == "" {
		return "", "", nil
	}
	if request.MediaPort == "" {
		request.MediaPort = defaultMediaPort
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
		return "", "", fmt.Errorf("media host must be a private IPv4 address on your network")
	}
	if !ip.IsPrivate() {
		return "", "", fmt.Errorf("media host must be a private address on the business network, not a public one")
	}
	number, e := strconv.Atoi(request.MediaPort)
	if e != nil || number < 1 || number > 65535 {
		return "", "", fmt.Errorf("media port must be between 1 and 65535")
	}
	return ip.To4().String(), strconv.Itoa(number), nil
}

var portalVersionPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)

type PortalDeployment struct {
	Profile      string `json:"profile"`
	Version      string `json:"version"`
	Confirmation string `json:"confirmation"`
	// MediaHost is the owner's LAN media server (Jellyfin) that guests on this
	// HotSpot network must be able to reach. Empty means no media service is
	// offered and the walled garden stays portal-only. Video bytes travel only on
	// the local network; nothing here enables cloud media transport.
	MediaHost string `json:"media_host"`
	MediaPort string `json:"media_port"`
}
type PortalDeploymentPlan struct {
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

func portalLoginHTML(deviceID string) string {
	destination, _ := json.Marshal(portalOrigin + "/portal.html?deviceId=" + url.QueryEscape(deviceID))
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Connect to WiFi</title></head><body><p>Opening the WiFi portal…</p><a id="open">Open portal</a><script>var u=` + string(destination) + `+"&mac="+encodeURIComponent("$(mac)")+"&ip="+encodeURIComponent("$(ip)");document.getElementById("open").href=u;location.replace(u);</script><noscript>Enable JavaScript to connect to WiFi.</noscript></body></html>`
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
	mediaGarden                                                       string
	mediaHost, mediaPort                                              string
	hasMediaGarden                                                    bool
	directory, destination                  string
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
	out, err := r.Run(ctx, "/ip hotspot profile print value-list detail where "+s.selector)
	if err != nil {
		return s, fmt.Errorf("could not read HotSpot profile")
	}
	p := parseValueList(out)
	if !strings.Contains(","+p["login-by"]+",", ",mac,") {
		return s, fmt.Errorf("configure MAC authentication for managed guests in gateway setup first")
	}
	if v := p["html-directory-override"]; v != "" && v != "none" {
		return s, fmt.Errorf("external HTML overrides require operator-assisted deployment")
	}
	directory := strings.Trim(p["html-directory"], "/")
	if directory == "" || strings.Contains(directory, "..") {
		return s, fmt.Errorf("invalid HotSpot HTML directory")
	}
	s.directory = p["html-directory"]
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
	out, err = r.Run(ctx, "/ip hotspot print value-list detail where profile="+routerOSQuoted(request.Profile)+" disabled=no")
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
	s.replacement = portalLoginHTML(deviceID)
	s.garden = "comment=" + routerOSQuoted("mm-portal-"+request.Profile) + " dynamic=no"
	mediaHost, mediaPort, mediaErr := mediaTarget(request)
	if mediaErr != nil {
		return s, mediaErr
	}
	s.mediaHost, s.mediaPort = mediaHost, mediaPort
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
	steps := []string{"Clone all existing HotSpot files into a separate persistent directory; retain the original directory for rollback", "Allow TCP 443 to mikromagic.com for HotSpot " + server + " (hostname resolves to IPs, possibly shared CDN addresses)"}
	if s.mediaHost != "" {
		steps = append(steps, "Allow guests on this HotSpot to reach the media server at " + s.mediaHost + ":" + s.mediaPort + " on your local network")
	}
	steps = append(steps, "Install the HTTPS hosted portal entrypoint and switch only profile "+request.Profile, "Read back and verify the login page, profile and access rule; requires RouterOS file copy support")
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
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, e := r.Run(cleanup, "/ip hotspot profile set [find "+s.selector+"] html-directory="+routerOSQuoted(s.directory))
		out, readErr := r.Run(cleanup, "/ip hotspot profile print value-list detail where "+s.selector)
		ok := e == nil && readErr == nil && parseValueList(out)["html-directory"] == s.directory
		if addedGarden {
			if s.hasMediaGarden {
				_, _ = r.Run(cleanup, "/ip hotspot walled-garden ip remove [find "+s.mediaGarden+"]")
			}
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
		_, err = r.Run(ctx, "/ip hotspot walled-garden ip add action=accept disabled=no protocol=tcp dst-port="+s.mediaPort+" dst-address="+routerOSQuoted(s.mediaHost)+" server="+routerOSQuoted(s.plan.Server)+" comment="+routerOSQuoted("mm-media-"+s.plan.Profile))
		if err != nil {
			rollback()
			return result, fmt.Errorf("media access rule failed; check rollback status")
		}
		s.hasMediaGarden = true
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
		out, err = r.Run(ctx, "/ip hotspot profile print value-list detail where "+s.selector)
	}
	if err != nil || parseValueList(out)["html-directory"] != s.destination {
		rollback()
		return result, fmt.Errorf("portal profile switch did not verify; check rollback status")
	}
	result.Verified = true
	return result, nil
}
