package hub

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// CallbackEnrollPath is the public, token-authenticated route a router calls
// during router-generated-key enrollment.
const CallbackEnrollPath = "/api/v1/hub-callback/enroll"

// CallbackTokenTTL bounds how long a pasted script can still enroll.
const CallbackTokenTTL = 15 * time.Minute

// callbackToken is 32 random bytes, base64url without padding.
var callbackToken = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidCallbackToken reports whether value has the shape of an issued token.
func ValidCallbackToken(value string) bool { return callbackToken.MatchString(value) }

// CallbackURL validates the app's public base URL and returns the full callback
// URL. Only HTTPS to a plain host (optional port) is accepted, so nothing but a
// fixed, validated string is ever interpolated into RouterOS.
func CallbackURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || !endpointName.MatchString(u.Hostname()) || strings.Contains(u.Hostname(), "..") {
		return "", fmt.Errorf("invalid callback base URL")
	}
	return "https://" + u.Host + CallbackEnrollPath, nil
}

// BuildCallbackBootstrapScript builds the one block a person pastes in WinBox
// when the router generates its own WireGuard key.
//
// The router creates the dedicated interface (RouterOS makes the private key
// itself), reads only the PUBLIC key and posts it with its RouterOS version to the
// app, authenticated by a single-use token. The app enrols the Hub peer and
// answers with the validated tunnel script, which the router then runs. The
// private key never leaves the router and is never sent, stored or logged.
//
// The router must verify the app's certificate (check-certificate=yes). The
// script never falls back to an unverified connection, because the reply is
// executed on the router. If the router cannot verify the certificate the paste
// stops with nothing changed beyond the empty interface.
func BuildCallbackBootstrapScript(deviceID, baseURL, token string) (string, error) {
	name, marker, err := scriptNames(deviceID)
	if err != nil {
		return "", err
	}
	if !ValidCallbackToken(token) {
		return "", fmt.Errorf("invalid callback token")
	}
	callback, err := CallbackURL(baseURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{
# Magic Hub one-paste setup. Paste the entire block in WinBox New Terminal.
# This router creates its own WireGuard key and keeps it. Only the PUBLIC key is sent to the app.
:local mmVer [/system resource get version]
:local mmMajor [:tonum [:pick $mmVer 0 [:find $mmVer "."]]]
:if ($mmMajor < 7) do={ :error ("STOP: Magic Hub needs RouterOS 7 or newer; this router runs " . $mmVer . ". Nothing changed.") }
:local mmIface [/interface/wireguard find where name="%[1]s"]
:if ([:len $mmIface] > 1) do={ :error "STOP: duplicate tunnel interface; nothing changed" }
:if ([:len $mmIface] = 0) do={
  /interface/wireguard add name="%[1]s" mtu=1280 comment="%[2]s"
  :set mmIface [/interface/wireguard find where name="%[1]s"]
}
:if ([/interface/wireguard get $mmIface comment] != "%[2]s") do={ :error "STOP: interface belongs to another setup; nothing changed" }
:local mmPub [/interface/wireguard get $mmIface public-key]
:local mmBody ("{\"token\":\"%[3]s\",\"public_key\":\"" . $mmPub . "\",\"routeros_version\":\"" . $mmVer . "\"}")
:local mmReply
:do {
  :set mmReply [/tool/fetch url="%[4]s" http-method=post http-header-field="Content-Type: application/json" http-data=$mmBody check-certificate=yes output=user as-value]
} on-error={ :error "STOP: the app did not accept this setup. Check the router can reach the app and trusts its certificate, then ask the app for a new script." }
:if ($mmReply->"status" != "finished") do={ :error "STOP: the app did not finish answering; run this block again" }
:local mmRun [:parse ($mmReply->"data")]
$mmRun
}`, name, marker, token, callback), nil
}
