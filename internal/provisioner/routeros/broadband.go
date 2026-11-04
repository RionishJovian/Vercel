package routeros

import (
	"context"
	"fmt"
	"mega-magic/internal/broadband"
	"strconv"
	"strings"
	"sync"
	"time"
)

const bbGuardName = "mega-magic-broadband-expiry-v1"
const bbMarker = "mega-magic:broadband:v1"

func bbProfile(id string) string { return "mm-bb-" + strings.ReplaceAll(id, "-", "") }

func validBBSpec(s broadband.Spec) bool {
	if !broadband.UUID.MatchString(s.AccountID) || !broadband.UUID.MatchString(s.OperationID) || !gatewayInterfacePattern.MatchString(s.Username) || len(s.Username) > 64 || !gatewayInterfacePattern.MatchString(s.Profile) {
		return false
	}
	switch s.Action {
	case "create":
		return s.ExpectedOperation == "" && networkText(s.Password, 128) && len(s.Password) >= 10 && broadband.ValidPeriod(s.PeriodSeconds)
	case "renew":
		return broadband.UUID.MatchString(s.ExpectedOperation) && broadband.ValidPeriod(s.PeriodSeconds)
	case "suspend", "resume":
		return broadband.UUID.MatchString(s.ExpectedOperation) && s.PeriodSeconds == 0
	}
	return false
}
func readBroadband(ctx context.Context, r CommandRunner, id, name string) (broadband.Observation, error) {
	return readBroadbandObservation(ctx, r, id, name, true)
}
func readBroadbandObservation(ctx context.Context, r CommandRunner, id, name string, strict bool) (broadband.Observation, error) {
	var o broadband.Observation
	if !broadband.UUID.MatchString(id) || !gatewayInterfacePattern.MatchString(name) {
		return o, broadband.ErrState
	}
	out, e := r.Run(ctx, `:local u [/ppp secret find where name=`+routerOSQuoted(name)+`]; :if ([:len $u] != 1) do={ :error "account unavailable"; }; :if ([/ppp secret get $u service] != "pppoe" or [/ppp secret get $u profile] != `+routerOSQuoted(bbProfile(id))+`) do={ :error "account ownership changed"; }; :put [/ppp secret get $u comment]`)
	if e != nil {
		return o, fmt.Errorf("broadband account unavailable; retained app evidence was not changed")
	}
	o.State, o.OperationID, e = broadband.Decode(strings.TrimSpace(out), id)
	if e != nil {
		return o, e
	}
	anchor, e := verifyBBEnforcement(ctx, r, id, name)
	if e != nil {
		return o, e
	}
	o.State, e = o.State.ResolveAnchor(anchor)
	if e != nil {
		return o, e
	}
	o.ObservedAt, e = probeVoucherGuardClock(ctx, r)
	if e != nil || o.State.Status(o.ObservedAt) == "clock_unverified" {
		return o, broadband.ErrState
	}
	n, e := networkCount(ctx, r, "/ppp active print count-only where name="+routerOSQuoted(name)+" service=pppoe")
	if e != nil {
		return o, e
	}
	o.Connected = n > 0
	if strict {
		status := o.State.Status(o.ObservedAt)
		deny := status == "expired" || status == "suspended"
		out, e := r.Run(ctx, `:put [/ppp secret get [find where name=`+routerOSQuoted(name)+`] disabled]`)
		v := strings.TrimSpace(out)
		if e != nil || (v != "yes" && v != "true" && v != "no" && v != "false") {
			return o, fmt.Errorf("subscriber access state is unavailable")
		}
		disabled := v == "yes" || v == "true"
		if disabled != deny || deny && o.Connected {
			return o, fmt.Errorf("subscriber enforcement has not completed; retry the saved operation")
		}
	}
	if n == 0 {
		o.UsageAvailable = true
		return o, nil
	}
	stats, err := r.Run(ctx, `:foreach a in=[/ppp active print stats as-value where name=`+routerOSQuoted(name)+` service=pppoe] do={ :put ("BBSESSION=" . ($a->"session-id") . "|" . ($a->"uptime") . "|" . ($a->"bytes")); }`)
	if err == nil {
		o.Session, err = parseBroadbandSession(stats)
		o.UsageAvailable = err == nil && o.Session != nil
	}
	return o, nil
}

func verifyBBEnforcement(ctx context.Context, r CommandRunner, id, name string) (int64, error) {
	script, e := readNamedValueList(ctx, r, "/system script", bbGuardName)
	if e != nil || normalizeScriptSource(script["source"]) != normalizeScriptSource(bbGuardSource) || script["comment"] != bbMarker || !policyAllowsReadWrite(script["policy"]) || (script["dont-require-permissions"] != "yes" && script["dont-require-permissions"] != "true") {
		return 0, fmt.Errorf("broadband expiry enforcement is missing or altered")
	}
	schedule, e := readNamedValueList(ctx, r, "/system scheduler", bbGuardName)
	if e != nil || schedule["comment"] != bbMarker || schedule["on-event"] != "/system script run "+bbGuardName || parseUptimeSeconds(schedule["interval"]) != 15 || (schedule["disabled"] != "no" && schedule["disabled"] != "false") || !policyAllowsReadWrite(schedule["policy"]) {
		return 0, fmt.Errorf("broadband expiry schedule is unavailable")
	}
	profile, e := readNamedValueList(ctx, r, "/ppp profile", bbProfile(id))
	if e != nil || profile["name"] != bbProfile(id) || profile["only-one"] != "yes" || normalizeScriptSource(profile["on-up"]) != normalizeScriptSource(bbOnUp(name, id)) || profile["on-down"] != "" {
		return 0, fmt.Errorf("subscriber first-use enforcement is missing or altered")
	}
	if profile["comment"] == bbMarker {
		return 0, nil
	}
	p := strings.Split(profile["comment"], ",")
	if len(p) != 2 || p[0] != bbMarker {
		return 0, broadband.ErrState
	}
	epoch, e := strconv.ParseInt(p[1], 10, 64)
	if e != nil || epoch < 1000000000 || epoch > 4102444800 {
		return 0, broadband.ErrState
	}
	return epoch, nil
}

func settleBroadband(ctx context.Context, r CommandRunner, s broadband.Spec) (broadband.Observation, error) {
	o, e := readBroadbandObservation(ctx, r, s.AccountID, s.Username, false)
	if e != nil {
		return o, e
	}
	if o.OperationID != s.OperationID {
		return o, fmt.Errorf("subscriber change readback did not match; retry the saved operation")
	}
	status := o.State.Status(o.ObservedAt)
	deny := status == "expired" || status == "suspended"
	disabled := "no"
	if deny {
		disabled = "yes"
	}
	cmd := `/ppp secret set [find where name=` + routerOSQuoted(s.Username) + `] disabled=` + disabled
	if _, e = r.Run(ctx, cmd); e != nil {
		return o, fmt.Errorf("subscriber access state not confirmed; retry saved operation")
	}
	if deny {
		if _, e = r.Run(ctx, `/ppp active remove [find where name=`+routerOSQuoted(s.Username)+` service=pppoe]`); e != nil {
			return o, fmt.Errorf("subscriber disconnection not confirmed; retry saved operation")
		}
		n, e := networkCount(ctx, r, "/ppp active print count-only where name="+routerOSQuoted(s.Username)+" service=pppoe")
		if e != nil || n != 0 {
			return o, fmt.Errorf("subscriber disconnection not verified")
		}
		o.Connected = false
		o.Session = nil
		o.UsageAvailable = true
	}
	v, e := r.Run(ctx, `:put [/ppp secret get [find where name=`+routerOSQuoted(s.Username)+`] disabled]`)
	want := strings.TrimSpace(v)
	if e != nil || deny && (want != "yes" && want != "true") || !deny && (want != "no" && want != "false") {
		return o, fmt.Errorf("subscriber enabled state not verified")
	}
	return o, nil
}

func parseBroadbandSession(raw string) (*broadband.SessionUsage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "none" {
		return nil, nil
	}
	if strings.ContainsAny(raw, "\r\n") || !strings.HasPrefix(raw, "BBSESSION=") {
		return nil, broadband.ErrState
	}
	p := strings.Split(strings.TrimPrefix(raw, "BBSESSION="), "|")
	if len(p) == 3 {
		pair := strings.FieldsFunc(p[2], func(r rune) bool { return r == ';' || r == ',' || r == '/' })
		if len(pair) != 2 {
			return nil, broadband.ErrState
		}
		p = []string{p[0], p[1], pair[0], pair[1]}
	}
	if len(p) != 4 || !gatewayInterfacePattern.MatchString(p[0]) {
		return nil, broadband.ErrState
	}
	up := parseUptimeSeconds(p[1])
	if up == 0 && p[1] != "0" && p[1] != "0s" && p[1] != "00:00:00" {
		return nil, broadband.ErrState
	}
	down, e := strconv.ParseInt(p[2], 10, 64)
	if e != nil || down < 0 {
		return nil, broadband.ErrState
	}
	upload, e := strconv.ParseInt(p[3], 10, 64)
	if e != nil || upload < 0 {
		return nil, broadband.ErrState
	}
	return &broadband.SessionUsage{ID: p[0], UptimeSeconds: int64(up), DownloadBytes: down, UploadBytes: upload}, nil
}
func ensureBBGuard(ctx context.Context, r CommandRunner) error {
	if _, e := probeVoucherGuardClock(ctx, r); e != nil {
		return e
	}
	ntp, e := r.Run(ctx, `:put [/system ntp client get status]`)
	if e != nil || strings.TrimSpace(ntp) != "synchronized" {
		return fmt.Errorf("synchronize router NTP before provisioning broadband")
	}
	add := "name=" + routerOSQuoted(bbGuardName) + " comment=" + routerOSQuoted(bbMarker) + " policy=read,write dont-require-permissions=yes source=" + routerOSQuoted(bbGuardSource)
	if e := ensureOrAdoptGuardObject(ctx, r, "/system script", bbGuardName, add, func(v map[string]string) bool {
		return normalizeScriptSource(v["source"]) == normalizeScriptSource(bbGuardSource) && v["comment"] == bbMarker && policyAllowsReadWrite(v["policy"])
	}); e != nil {
		return fmt.Errorf("broadband expiry script could not be verified")
	}
	event := "/system script run " + bbGuardName
	add = "name=" + routerOSQuoted(bbGuardName) + " interval=15s start-time=startup disabled=no policy=read,write on-event=" + routerOSQuoted(event) + " comment=" + routerOSQuoted(bbMarker)
	if e := ensureOrAdoptGuardObject(ctx, r, "/system scheduler", bbGuardName, add, func(v map[string]string) bool {
		return v["comment"] == bbMarker && v["on-event"] == event && parseUptimeSeconds(v["interval"]) == 15 && (v["disabled"] == "no" || v["disabled"] == "false") && policyAllowsReadWrite(v["policy"])
	}); e != nil {
		return fmt.Errorf("broadband expiry schedule could not be verified")
	}
	return nil
}
func applyBroadband(ctx context.Context, r CommandRunner, s broadband.Spec) (broadband.Observation, error) {
	if !validBBSpec(s) {
		return broadband.Observation{}, broadband.ErrState
	}
	if s.Action != "create" {
		// Read ownership before any change. A lost response retries the same marker,
		// so the period can never be added a second time.
		out, e := r.Run(ctx, `:put [/ppp secret get [find where name=`+routerOSQuoted(s.Username)+`] comment]`)
		if e != nil {
			return broadband.Observation{}, broadband.ErrState
		}
		_, last, e := broadband.Decode(strings.TrimSpace(out), s.AccountID)
		if e != nil {
			return broadband.Observation{}, e
		}
		if last == s.OperationID {
			return settleBroadband(ctx, r, s)
		}
		if last != s.ExpectedOperation {
			return broadband.Observation{}, fmt.Errorf("subscriber changed; synchronize before another operation")
		}
	}
	if e := ensureBBGuard(ctx, r); e != nil {
		return broadband.Observation{}, e
	}
	if s.Action != "create" {
		if _, e := verifyBBEnforcement(ctx, r, s.AccountID, s.Username); e != nil {
			return broadband.Observation{}, e
		}
	}
	profile := bbProfile(s.AccountID)
	if s.Action == "create" {
		n, e := networkCount(ctx, r, "/ppp secret print count-only where name="+routerOSQuoted(s.Username))
		if e != nil {
			return broadband.Observation{}, e
		}
		if n > 0 {
			o, e := readBroadband(ctx, r, s.AccountID, s.Username)
			if e != nil || o.OperationID != s.OperationID {
				// A valid owned marker with an unavailable enforcement read is
				// uncertain, not a proven pre-install rejection.
				out, readErr := r.Run(ctx, `:put [/ppp secret get [find where name=`+routerOSQuoted(s.Username)+`] comment]`)
				_, last, decodeErr := broadband.Decode(strings.TrimSpace(out), s.AccountID)
				if readErr == nil && decodeErr == nil && last == s.OperationID {
					return broadband.Observation{}, fmt.Errorf("existing subscriber enforcement needs recovery")
				}
				if readErr != nil || strings.Contains(out, s.AccountID) {
					return broadband.Observation{}, fmt.Errorf("subscriber ownership is unavailable")
				}
				return broadband.Observation{}, broadband.ErrRejected
			}
			return settleBroadband(ctx, r, s)
		}
		// Clone only a real source profile without hooks, so an unrelated owner's
		// scripts are not silently replaced. Retain address/rate/IPv6 settings.
		source, e := readNamedValueList(ctx, r, "/ppp profile", s.Profile)
		if e != nil || source["name"] != s.Profile || source["on-up"] != "" || source["on-down"] != "" {
			if e != nil {
				return broadband.Observation{}, fmt.Errorf("subscriber plan could not be read")
			}
			return broadband.Observation{}, broadband.ErrRejected
		}
		hook := bbOnUp(s.Username, s.AccountID)
		add := "copy-from=[find where name=" + routerOSQuoted(s.Profile) + "] name=" + routerOSQuoted(profile) + " comment=" + routerOSQuoted(bbMarker) + " only-one=yes on-up=" + routerOSQuoted(hook) + " on-down=" + routerOSQuoted("")
		if e = ensureOrAdoptGuardObject(ctx, r, "/ppp profile", profile, add, func(v map[string]string) bool {
			return v["name"] == profile && v["comment"] == bbMarker && v["only-one"] == "yes" && normalizeScriptSource(v["on-up"]) == normalizeScriptSource(hook) && v["on-down"] == ""
		}); e != nil {
			return broadband.Observation{}, fmt.Errorf("subscriber profile could not be verified")
		}
		state := broadband.State{PeriodSeconds: s.PeriodSeconds}
		command := "/ppp secret add name=" + routerOSQuoted(s.Username) + " password=" + routerOSQuoted(s.Password) + " service=pppoe profile=" + routerOSQuoted(profile) + " disabled=no comment=" + routerOSQuoted(state.Encode(s.AccountID, s.OperationID))
		if _, e = r.Run(ctx, command); e != nil {
			return broadband.Observation{}, fmt.Errorf("subscriber installation not confirmed; retry the saved operation")
		}
	} else {
		// Mutation is computed on the router from its latest first-use state, not
		// a stale app observation. The expected marker is checked again atomically
		// in this command; every retry carries the same operation identity.
		command := `:local u [/ppp secret find where name=` + routerOSQuoted(s.Username) + `]; :if ([:len $u] != 1) do={ :error "account missing"; }; :local a [:toarray [/ppp secret get $u comment]]; ` +
			`:if ([:len $a] != 8) do={ :error "invalid state"; }; :if (($a->1) != ` + routerOSQuoted(s.AccountID) + ` or ($a->2) != ` + routerOSQuoted(s.ExpectedOperation) + ` or [/ppp secret get $u profile] != ` + routerOSQuoted(profile) + `) do={ :error "state changed"; }; ` +
			bbClockSource + `:if ($clockOK = false) do={ :error "clock unverified"; }; :local period [:tonum ($a->3)]; :local first [:tonum ($a->4)]; :local expiry [:tonum ($a->5)]; :local suspended [:tonum ($a->6)]; :local seen [:tonum ($a->7)]; ` +
			`:if ($now < $seen) do={ :error "clock unverified"; }; :local anchor [:toarray [/ppp profile get ` + routerOSQuoted(profile) + ` comment]]; ` +
			`:if ($first = 0 and [:len $anchor] = 2) do={ :set first [:tonum ($anchor->1)]; :set expiry ($first + $period); }; `
		switch s.Action {
		case "renew":
			command += fmt.Sprintf(`:if ($first = 0) do={ :set period ($period + %d); :if ($period > 316224000) do={ :error "period too large"; }; } else={ :if ($expiry < $now) do={ :set expiry $now; }; :set expiry ($expiry + %d); :if ($expiry > 4102444800 or $expiry > ($now + 316224000)) do={ :error "period too large"; }; }; `, s.PeriodSeconds, s.PeriodSeconds)
		case "suspend":
			command += `:set suspended 1; `
		case "resume":
			command += `:set suspended 0; `
		}
		command += `:local disabled false; :if ($suspended != 0 or ($first > 0 and $now >= $expiry)) do={ :set disabled true; }; ` +
			`/ppp secret set $u comment=("mm-bb-v1," . ($a->1) . "," . ` + routerOSQuoted(s.OperationID) + ` . "," . $period . "," . $first . "," . $expiry . "," . $suspended . "," . $now) disabled=$disabled; ` +
			`:if ($disabled) do={ /ppp active remove [find where name=` + routerOSQuoted(s.Username) + ` service=pppoe]; }`
		if _, e := r.Run(ctx, command); e != nil {
			return broadband.Observation{}, fmt.Errorf("subscriber change not confirmed; retry the saved operation")
		}
	}
	return settleBroadband(ctx, r, s)
}

func (a *DeviceApplier) ApplyBroadband(ctx context.Context, biz, device string, s broadband.Spec) (broadband.Observation, error) {
	lock, _ := gatewayLocks.LoadOrStore(biz+":"+device, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if until, ok := networkGuardUntil.Load(biz + ":" + device); ok && time.Now().Before(until.(time.Time)) {
		return broadband.Observation{}, fmt.Errorf("a network restoration is pending; finish it before subscriber changes")
	}
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, biz, device)
	if e != nil {
		return broadband.Observation{}, e
	}
	return applyBroadband(ctx, r, s)
}
func (a *DeviceApplier) ReadBroadband(ctx context.Context, biz, device, id, name string) (broadband.Observation, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	r, e := runnerForDevice(ctx, a.src, a.factory, a.timeout, biz, device)
	if e != nil {
		return broadband.Observation{}, e
	}
	return readBroadband(ctx, r, id, name)
}
