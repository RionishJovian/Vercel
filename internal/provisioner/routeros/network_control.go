package routeros

// Shared guarded execution for explicitly typed business-network operations.
// No menu, RouterOS command or property name is accepted from the browser.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NetworkObject struct {
	ID     string            `json:"id"`
	Values map[string]string `json:"values"`
}
type NetworkInventory struct {
	Health           map[string]string          `json:"health,omitempty"`
	WriteEnabled     bool                       `json:"write_enabled"`
	WiFiWriteEnabled bool                       `json:"wifi_write_enabled"`
	Interfaces       []GatewayInterface         `json:"interfaces"`
	Objects          map[string][]NetworkObject `json:"objects"`
	Available        map[string]bool            `json:"available"`
}
type NetworkRequest struct {
	Kind         string            `json:"kind"`
	ID           string            `json:"id"`
	Values       map[string]string `json:"values"`
	Confirmation string            `json:"confirmation"`
}
type NetworkReview struct {
	Confirmation string    `json:"confirmation"`
	Steps        []string  `json:"steps"`
	Warnings     []string  `json:"warnings"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type NetworkResult struct {
	Backup     string `json:"backup"`
	Verified   bool   `json:"verified"`
	RolledBack bool   `json:"rolled_back"`
}
type networkMenu struct {
	path   string
	fields []string
}

var networkMenus = map[string]networkMenu{
	"filters":           {"/ip firewall filter", []string{"chain", "action", "connection-state", "in-interface", "comment", "disabled", "dynamic"}},
	"bindings":          {"/ip hotspot ip-binding", []string{"mac-address", "address", "server", "type", "comment", "disabled"}},
	"queue_types":       {"/queue type", []string{"name", "kind", "pcq-rate", "pcq-classifier"}},
	"addresses":         {"/ip address", []string{"address", "interface", "comment", "disabled", "dynamic"}},
	"pools":             {"/ip pool", []string{"name", "ranges"}},
	"dhcp":              {"/ip dhcp-server", []string{"name", "interface", "address-pool", "lease-time", "disabled"}},
	"dhcp_networks":     {"/ip dhcp-server network", []string{"address", "gateway", "dns-server", "comment"}},
	"leases":            {"/ip dhcp-server lease", []string{"address", "mac-address", "server", "comment", "disabled", "dynamic"}},
	"hotspots":          {"/ip hotspot", []string{"name", "interface", "profile", "address-pool", "disabled"}},
	"hotspot_profiles":  {"/ip hotspot profile", []string{"name", "dns-name", "login-by", "use-radius", "html-directory"}},
	"routes":            {"/ip route", []string{"dst-address", "gateway", "distance", "check-gateway", "scope", "target-scope", "routing-table", "comment", "disabled", "dynamic"}},
	"dhcp_clients":      {"/ip dhcp-client", []string{"interface", "gateway", "add-default-route", "disabled"}},
	"pppoe_clients":     {"/interface pppoe-client", []string{"name", "interface", "user", "password", "add-default-route", "default-route-distance", "use-peer-dns", "disabled"}},
	"ppp_profiles":      {"/ppp profile", []string{"name", "local-address", "remote-address", "dns-server", "rate-limit", "only-one"}},
	"pppoe_servers":     {"/interface pppoe-server server", []string{"service-name", "interface", "default-profile", "authentication", "one-session-per-host", "disabled"}},
	"subscribers":       {"/ppp secret", []string{"name", "password", "service", "profile", "disabled"}},
	"queues":            {"/queue simple", []string{"name", "target", "max-limit", "queue", "parent", "comment", "disabled"}},
	"wifi":              {"/interface wifi", []string{"name", "configuration.ssid", "configuration.country", "configuration.mode", "security.authentication-types", "security.passphrase", "disabled"}},
	"wireless":          {"/interface wireless", []string{"name", "ssid", "country", "mode", "security-profile", "disabled"}},
	"wireless_security": {"/interface wireless security-profiles", []string{"name", "mode", "authentication-types", "wpa2-pre-shared-key"}},
	"nat":               {"/ip firewall nat", []string{"chain", "action", "src-address", "out-interface", "comment", "disabled"}},
	"ssh_sessions":      {"/user active", []string{"address", "via"}},
	"wg_peers":          {"/interface wireguard peers", []string{"current-endpoint-address"}},
}
var networkIDPattern = regexp.MustCompile(`^\*[0-9A-Fa-f]+$`)

func networkSecret(field string) bool {
	return field == "password" || field == "security.passphrase" || field == "wpa2-pre-shared-key"
}
func networkReadCommand(menu networkMenu) string {
	filter := ""
	if menu.path == "/queue type" {
		filter = " where kind=pcq"
	}
	command := `:foreach id in=[` + menu.path + ` find` + filter + `] do={ :put ("id: " . $id);`
	for _, field := range menu.fields {
		command += ` :put (` + routerOSQuoted(field+": ") + ` . [:tostr [` + menu.path + ` get $id ` + field + `]]);`
	}
	return command + " }"
}
func readNetworkObjects(ctx context.Context, r CommandRunner, key string) ([]NetworkObject, error) {
	menu, ok := networkMenus[key]
	if !ok {
		return nil, fmt.Errorf("unsupported network menu")
	}
	out, err := r.Run(ctx, networkReadCommand(menu))
	if err != nil {
		return nil, fmt.Errorf("could not read %s safely", key)
	}
	if len(out) > 512*1024 {
		return nil, fmt.Errorf("network inventory exceeds the safe size limit")
	}
	return parseNetworkObjects(menu, out, key)
}
func parseNetworkObjects(menu networkMenu, out, key string) ([]NetworkObject, error) {
	objects := []NetworkObject{}
	var current *NetworkObject
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		field, value, ok := strings.Cut(line, ": ")
		if !ok {
			return nil, fmt.Errorf("unrecognized %s inventory", key)
		}
		if field == "id" {
			if !networkIDPattern.MatchString(value) {
				return nil, fmt.Errorf("invalid router record identity")
			}
			objects = append(objects, NetworkObject{ID: value, Values: map[string]string{}})
			current = &objects[len(objects)-1]
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("missing router record identity")
		}
		allowed := false
		for _, known := range menu.fields {
			if known == field {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unexpected router property")
		}
		if field == "dns-server" || field == "authentication" || field == "security.authentication-types" || field == "login-by" || field == "target" {
			value = strings.ReplaceAll(value, ";", ",")
		}
		current.Values[field] = value
	}
	for _, object := range objects {
		for _, field := range menu.fields {
			if _, ok := object.Values[field]; !ok {
				return nil, fmt.Errorf("incomplete %s inventory", key)
			}
		}
	}
	return objects, nil
}
func networkReadOne(ctx context.Context, r CommandRunner, key, id string, fields map[string]string) (map[string]string, error) {
	if !networkIDPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid record identity")
	}
	menu := networkMenus[key]
	menu.fields = networkSortedKeys(fields)
	command := networkReadCommand(menu)
	start := strings.Index(command, " find")
	end := strings.Index(command, "] do={")
	command = command[:start] + " find where .id=" + id + command[end:]
	text, err := r.Run(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("could not read selected %s fields safely", key)
	}
	objects, err := parseNetworkObjects(menu, text, key)
	if err != nil || len(objects) != 1 || objects[0].ID != id {
		return nil, fmt.Errorf("selected %s record is missing or unreadable", key)
	}
	return objects[0].Values, nil
}
func networkSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func networkHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func networkBool(v string) string {
	if v == "true" {
		return "yes"
	}
	if v == "false" {
		return "no"
	}
	return v
}
func networkEqual(field, a, b string) bool {
	if a == b {
		return true
	}
	if networkBool(a) == networkBool(b) {
		return true
	}
	if field == "lease-time" {
		x, ok := networkDurationSeconds(a)
		y, ok2 := networkDurationSeconds(b)
		return ok && ok2 && x == y
	}
	if field == "rate-limit" || field == "max-limit" {
		parse := func(value string) (string, bool) {
			parts := strings.Split(value, "/")
			if len(parts) != 2 {
				return "", false
			}
			for i, part := range parts {
				factor := uint64(1)
				if len(part) > 0 {
					switch part[len(part)-1] {
					case 'k', 'K':
						factor = 1000
						part = part[:len(part)-1]
					case 'M':
						factor = 1000000
						part = part[:len(part)-1]
					case 'G':
						factor = 1000000000
						part = part[:len(part)-1]
					}
				}
				n, e := strconv.ParseUint(part, 10, 64)
				if e != nil || n > 1000000000000/factor {
					return "", false
				}
				parts[i] = strconv.FormatUint(n*factor, 10)
			}
			return strings.Join(parts, "/"), true
		}
		x, ok := parse(a)
		y, ok2 := parse(b)
		return ok && ok2 && x == y
	}
	if field == "dns-server" || field == "authentication" || field == "security.authentication-types" || field == "login-by" {
		// RouterOS :tostr renders arrays with semicolons, while set accepts CSV.
		split := func(v string) []string {
			return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' })
		}
		x, y := split(a), split(b)
		sort.Strings(x)
		sort.Strings(y)
		return strings.Join(x, ",") == strings.Join(y, ",")
	}
	return false
}
func (a *DeviceApplier) ReadNetwork(ctx context.Context, businessID, deviceID string) (NetworkInventory, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return NetworkInventory{}, err
	}
	interfaces, err := gatewayInterfaces(ctx, r)
	if err != nil {
		return NetworkInventory{}, err
	}
	result := NetworkInventory{Health: readHubHealth(ctx, r), Interfaces: interfaces, Objects: map[string][]NetworkObject{}, Available: map[string]bool{}}
	for key := range networkMenus {
		if key == "ssh_sessions" || key == "wg_peers" {
			continue
		}
		objects, err := readNetworkObjects(ctx, r, key)
		result.Available[key] = err == nil
		if err != nil {
			continue
		}
		for _, object := range objects {
			for field := range object.Values {
				if networkSecret(field) {
					if object.Values[field] != "" {
						object.Values["has_secret"] = "yes"
					}
					delete(object.Values, field)
				}
			}
		}
		result.Objects[key] = objects
	}
	return result, nil
}

type networkStep struct {
	Key, ID, SelectorField, SelectorValue, Label string
	Values, Before                               map[string]string
	First                                        bool
	EmptyTable                                   bool
}
type networkApproval struct {
	Business, Device, RequestHash, StateHash string
	Expires                                  time.Time
}

var networkApprovals = struct {
	sync.Mutex
	entries map[string]networkApproval
}{entries: map[string]networkApproval{}}

// An unconfirmed timer must finish before another network write can reuse its
// owned selectors. This map is guarded by the per-device gateway lock.
var networkGuardUntil sync.Map

func (a *DeviceApplier) PreviewNetwork(ctx context.Context, businessID, deviceID string, request NetworkRequest) (NetworkReview, error) {
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return NetworkReview{}, err
	}
	steps, warnings, err := buildNetworkPlan(ctx, r, request)
	if err != nil {
		return NetworkReview{}, err
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return NetworkReview{}, fmt.Errorf("preview unavailable")
	}
	token := hex.EncodeToString(raw)
	request.Confirmation = ""
	expiry := time.Now().Add(5 * time.Minute)
	networkApprovals.Lock()
	defer networkApprovals.Unlock()
	for key, entry := range networkApprovals.entries {
		if time.Now().After(entry.Expires) {
			delete(networkApprovals.entries, key)
		}
	}
	if len(networkApprovals.entries) >= 512 {
		return NetworkReview{}, fmt.Errorf("too many active previews; retry shortly")
	}
	networkApprovals.entries[token] = networkApproval{businessID, deviceID, networkHash(request), networkHash(steps), expiry}
	labels := []string{}
	for _, step := range steps {
		details := []string{}
		for _, field := range networkSortedKeys(step.Values) {
			if networkSecret(field) {
				details = append(details, field+": (value hidden)")
				continue
			}
			value := step.Values[field]
			if old, exists := step.Before[field]; exists {
				details = append(details, field+": "+old+" → "+value)
			} else {
				details = append(details, field+": "+value)
			}
		}
		labels = append(labels, step.Label+" — "+strings.Join(details, "; "))
	}
	return NetworkReview{token, labels, warnings, expiry}, nil
}
func networkStepSelector(s networkStep) string {
	return s.SelectorField + "=" + routerOSQuoted(s.SelectorValue)
}
func networkStepID(ctx context.Context, r CommandRunner, s networkStep) (string, error) {
	if s.ID != "" {
		return s.ID, nil
	}
	out, err := r.Run(ctx, `:put [`+networkMenus[s.Key].path+` find where `+networkStepSelector(s)+`]`)
	id := strings.TrimSpace(out)
	if err != nil || !networkIDPattern.MatchString(id) {
		return "", fmt.Errorf("owned %s record is missing or ambiguous", s.Key)
	}
	return id, nil
}

// clearNATTracking drops the connection-tracking table after a NAT change.
// RouterOS matches NAT on the first packet only and remembers the result, so a
// modified masquerade rule does not affect already-tracked connections: the
// change silently appears not to work until those entries expire.
func clearNATTracking(ctx context.Context, r CommandRunner) {
	_, _ = r.Run(ctx, "/ip/firewall/connection tracking remove")
}

func networkWrite(ctx context.Context, r CommandRunner, s networkStep, restore bool) error {
	_, err := r.Run(ctx, networkWriteCommand(s, restore))
	return err
}
func networkWriteCommand(s networkStep, restore bool) string {
	menu := networkMenus[s.Key].path
	values := s.Values
	if restore && s.ID == "" {
		return menu + " remove [find where " + networkStepSelector(s) + "]"
	}
	cmd := menu + " add"
	if s.ID != "" {
		cmd = menu + " set " + s.ID
	}
	if restore {
		values = s.Before
	}
	for _, field := range networkSortedKeys(values) {
		cmd += " " + field + "=" + routerOSQuoted(values[field])
	}
	if s.First && !s.EmptyTable && s.ID == "" && !restore {
		cmd += " place-before=0"
	}
	return cmd
}

// natTouched reports whether any step changes NAT. Tracking must be flushed once
// after the last such step, not per-step, so the table is not wiped repeatedly.
func natTouched(steps []networkStep) bool {
	for _, s := range steps {
		if s.Key == "nat" {
			return true
		}
	}
	return false
}
func networkVerify(ctx context.Context, r CommandRunner, s networkStep, restore bool) error {
	if restore && s.ID == "" {
		out, err := r.Run(ctx, networkMenus[s.Key].path+" print count-only where "+networkStepSelector(s))
		if err != nil || strings.TrimSpace(out) != "0" {
			return fmt.Errorf("owned record cleanup not verified")
		}
		return nil
	}
	id, err := networkStepID(ctx, r, s)
	if err != nil {
		return err
	}
	want := s.Values
	if restore {
		want = s.Before
	}
	actual, err := networkReadOne(ctx, r, s.Key, id, want)
	if err != nil {
		return err
	}
	for field, value := range want {
		if !networkEqual(field, actual[field], value) {
			return fmt.Errorf("%s readback mismatch at %s", s.Key, field)
		}
	}
	if s.First && !restore {
		first, e := r.Run(ctx, `:put [:pick [`+networkMenus[s.Key].path+` find] 0]`)
		if e != nil || strings.TrimSpace(first) != id {
			return fmt.Errorf("protection rule order not verified")
		}
	}
	return nil
}
func (a *DeviceApplier) ApplyNetwork(ctx context.Context, businessID, deviceID string, request NetworkRequest) (NetworkResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result := NetworkResult{}
	networkApprovals.Lock()
	approved, ok := networkApprovals.entries[request.Confirmation]
	delete(networkApprovals.entries, request.Confirmation)
	networkApprovals.Unlock()
	request.Confirmation = ""
	if !ok || approved.Business != businessID || approved.Device != deviceID || time.Now().After(approved.Expires) || approved.RequestHash != networkHash(request) {
		return result, fmt.Errorf("preview expired or changed; review again before applying")
	}
	lock, _ := gatewayLocks.LoadOrStore(businessID+":"+deviceID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	guardKey := businessID + ":" + deviceID
	if until, ok := networkGuardUntil.Load(guardKey); ok && time.Now().Before(until.(time.Time)) {
		return result, fmt.Errorf("a previous router restoration timer may still be active; wait three minutes and reload before another change")
	}
	r, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return result, err
	}
	steps, _, err := buildNetworkPlan(ctx, r, request)
	if err != nil {
		return result, err
	}
	if networkHash(steps) != approved.StateHash {
		return result, fmt.Errorf("router settings changed since preview; reload and review again")
	}
	jobs, e := networkCount(ctx, r, "/system script job print count-only")
	// RouterOS includes the current SSH script itself in this count.
	if e != nil || jobs != 1 {
		return result, fmt.Errorf("router has an active script job; wait for it to finish before changing network settings")
	}
	result.Backup = snapshotName(deviceID+"-network", time.Now().UTC())
	for _, command := range []string{"/export file=" + result.Backup, "/system backup save name=" + result.Backup} {
		if _, err = r.Run(ctx, command); err != nil {
			return result, fmt.Errorf("network backup failed; nothing applied")
		}
	}
	// The timer lives on the router, not on the API host. It restores the exact
	// reviewed fields if transport disappears. Never log this source: previous
	// credentials can be part of a restoration. Each undo is isolated so one
	// absent addition does not prevent the remaining records from restoring.
	guard := `:put [:execute { :delay 3m;`
	for i := len(steps) - 1; i >= 0; i-- {
		guard += ` :do { ` + networkWriteCommand(steps[i], true) + ` } on-error={};`
	}
	guard += ` }]`
	networkGuardUntil.Store(guardKey, time.Now().Add(3*time.Minute+10*time.Second))
	out, guardErr := r.Run(ctx, guard)
	job := strings.TrimSpace(out)
	if guardErr != nil || !networkIDPattern.MatchString(job) {
		return result, fmt.Errorf("router-side restoration guard unavailable; nothing applied")
	}
	cancelGuard := func(cleanup context.Context) error {
		_, e := r.Run(cleanup, "/system script job remove "+job)
		if e != nil {
			return fmt.Errorf("restoration guard cancellation failed")
		}
		n, e := networkCount(cleanup, r, "/system script job print count-only where .id="+job)
		if e != nil || n != 0 {
			return fmt.Errorf("restoration guard cancellation not verified")
		}
		networkGuardUntil.Delete(guardKey)
		return nil
	}
	applied := []networkStep{}
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		verified := true
		for i := len(applied) - 1; i >= 0; i-- {
			step := applied[i]
			if networkWrite(cleanup, r, step, true) != nil || networkVerify(cleanup, r, step, true) != nil {
				verified = false
			}
		}
		result.RolledBack = verified
		if natTouched(applied) {
			clearNATTracking(cleanup, r)
		}
		if verified {
			_ = cancelGuard(cleanup)
		}
	}
	if request.Kind == "wan" {
		// Stage a coherent routing policy in one router script. Independent SSH
		// writes can lose their own return route halfway through a WAN change.
		applied = append(applied, steps...)
		commands := []string{}
		for _, step := range steps {
			commands = append(commands, networkWriteCommand(step, false))
		}
		if _, err = r.Run(ctx, "{ "+strings.Join(commands, "; ")+" }"); err != nil {
			rollback()
			return result, fmt.Errorf("WAN routing script failed; inspect rollback status")
		}
	}
	for _, step := range steps {
		if request.Kind != "wan" {
			applied = append(applied, step)
			err = networkWrite(ctx, r, step, false)
		}
		if err != nil {
			rollback()
			return result, fmt.Errorf("network write failed at %s; inspect rollback status", step.Key)
		}
		if err = networkVerify(ctx, r, step, false); err != nil {
			rollback()
			return result, fmt.Errorf("network verification failed: %s; inspect rollback status", err.Error())
		}
	}
	if natTouched(steps) {
		clearNATTracking(ctx, r)
	}
	if request.Kind == "wan" {
		for _, probe := range []string{"1.1.1.1", "8.8.8.8"} {
			count, e := networkProbe(ctx, r, probe)
			if e != nil || count < 1 {
				rollback()
				return result, fmt.Errorf("ISP health probe did not reply; WAN changes rolled back where reachable")
			}
		}
	}
	if err = cancelGuard(ctx); err != nil {
		rollback()
		return result, fmt.Errorf("guard cancellation failed; inspect rollback status")
	}
	result.Verified = true
	return result, nil
}

// RouterOS prints the ping table even when its return value is assigned. Read
// one explicitly labelled scalar, never interpret arbitrary table numbers.
func networkProbe(ctx context.Context, r CommandRunner, probe string) (int, error) {
	if probe != "1.1.1.1" && probe != "8.8.8.8" {
		return 0, fmt.Errorf("unsupported health probe")
	}
	out, err := r.Run(ctx, `:local received [/ping `+probe+` count=2 interval=500ms]; :put ("MM-PROBE-RESULT:" . $received)`)
	if err != nil || len(out) > 4096 {
		return 0, fmt.Errorf("health probe unreadable")
	}
	count, matches := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if value, ok := strings.CutPrefix(line, "MM-PROBE-RESULT:"); ok {
			matches++
			n, e := strconv.Atoi(value)
			if e != nil || n < 0 || n > 2 {
				return 0, fmt.Errorf("invalid health probe result")
			}
			count = n
		}
	}
	if matches != 1 {
		return 0, fmt.Errorf("ambiguous health probe result")
	}
	return count, nil
}
