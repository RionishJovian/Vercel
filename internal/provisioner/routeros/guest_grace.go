package routeros

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var guestRequestIDPattern = regexp.MustCompile(`^[a-fA-F0-9-]{36}$`)

// GuestGrace is tied to the purchase and the client seen on this router.
// MB uses decimal bytes, matching the label shown in the portal.
type GuestGrace struct {
	RequestID string
	ClientMAC string
	Minutes   int
	DataMB    int
	ExpiresAt time.Time
}

func normalizeGuestGrace(g GuestGrace) (GuestGrace, error) {
	mac, err := net.ParseMAC(g.ClientMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return g, fmt.Errorf("a valid unicast client MAC is required")
	}
	g.ClientMAC = strings.ToUpper(mac.String())
	if !guestRequestIDPattern.MatchString(g.RequestID) || g.Minutes < 1 || g.Minutes > 1440 || g.DataMB < 1 || g.DataMB > 10240 {
		return g, fmt.Errorf("invalid grace request or limits")
	}
	return g, nil
}

// ConfigureGuestGrace uses RouterOS MAC authentication. It does not change
// the owner's authentication profile or bypass Hotspot using an IP binding.
// Existing users belonging to another request are never overwritten.
func (a *DeviceApplier) ConfigureGuestGrace(ctx context.Context, businessID, deviceID string, g GuestGrace) error {
	g, err := normalizeGuestGrace(g)
	if err != nil {
		return err
	}
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	return configureGuestGrace(ctx, runner, g)
}

func configureGuestGrace(ctx context.Context, runner CommandRunner, g GuestGrace) error {
	// Resolve server and profile from the live host table, never from the guest's
	// submitted IP or an arbitrary router profile name.
	out, err := runner.Run(ctx, "/ip hotspot host print value-list detail where mac-address="+g.ClientMAC)
	if err != nil {
		return fmt.Errorf("read Hotspot client: %w", err)
	}
	host := parseValueList(out)
	server := host["server"]
	if server == "" || strings.ToUpper(host["mac-address"]) != g.ClientMAC {
		return fmt.Errorf("client is not present on this router's Hotspot")
	}
	out, err = runner.Run(ctx, "/ip hotspot print value-list detail where name="+routerOSQuoted(server))
	if err != nil {
		return fmt.Errorf("read Hotspot server: %w", err)
	}
	profile := parseValueList(out)["profile"]
	if profile == "" {
		return fmt.Errorf("Hotspot server has no profile")
	}
	out, err = runner.Run(ctx, "/ip hotspot profile print value-list detail where name="+routerOSQuoted(profile))
	if err != nil {
		return fmt.Errorf("read Hotspot login method: %w", err)
	}
	profileFields := parseValueList(out)
	macLogin := false
	for _, method := range strings.Split(profileFields["login-by"], ",") {
		if strings.TrimSpace(method) == "mac" {
			macLogin = true
		}
	}
	if !macLogin {
		return fmt.Errorf("this Hotspot profile needs MAC login enabled for temporary bank-transfer access")
	}
	name := routerOSQuoted(g.ClientMAC)
	comment := "mega-magic:grace:" + g.RequestID
	read := "/ip hotspot user print value-list detail where name=" + name
	out, err = runner.Run(ctx, read)
	if err != nil {
		return fmt.Errorf("read temporary user: %w", err)
	}
	user := parseValueList(out)
	if user["name"] != "" {
		if user["comment"] != comment {
			return fmt.Errorf("this client already has a different Hotspot user")
		}
		// Resume without resetting counters or increasing an existing allowance.
		return verifyGraceUser(user, g, server, comment)
	}
	// A one-shot router scheduler enforces the wall-clock payment deadline
	// even while the client disconnects or the web API is unavailable.
	expires := g.ExpiresAt
	if expires.IsZero() {
		expires = time.Now().Add(time.Duration(g.Minutes) * time.Minute)
	}
	out, err = runner.Run(ctx, "/system clock print")
	if err != nil {
		return fmt.Errorf("read router clock: %w", err)
	}
	clock := parseValueList(out)
	clockLayout := "2006-01-02 15:04:05"
	if strings.Contains(clock["date"], "/") {
		clockLayout = "Jan/02/2006 15:04:05"
	}
	clockText := clock["date"] + " " + clock["time"]
	if strings.Contains(clock["date"], "/") {
		clockText = strings.ToUpper(clockText[:1]) + clockText[1:]
	}
	routerNow, err := time.Parse(clockLayout, clockText)
	remaining := time.Until(expires)
	if err != nil || remaining <= 0 {
		return fmt.Errorf("router clock or grace deadline is invalid")
	}
	routerEnd := routerNow.Add(remaining)
	dateFormat := "2006-01-02"
	if strings.Contains(clock["date"], "/") {
		dateFormat = "Jan/02/2006"
	}
	deadlineDate := routerEnd.Format(dateFormat)
	if dateFormat != "2006-01-02" {
		deadlineDate = strings.ToLower(deadlineDate)
	}
	scheduler := "mm-grace-" + g.RequestID
	find := "[find name=" + name + " comment=" + routerOSQuoted(comment) + "]"
	event := ":if ([:len [/ip hotspot user find name=" + name + " comment=" + routerOSQuoted(comment) + "]] > 0) do={ /ip hotspot user disable " + find + "; /ip hotspot active remove [find user=" + name + "] }; /system scheduler remove [find name=" + routerOSQuoted(scheduler) + "]"
	scheduleCmd := "/system scheduler add name=" + routerOSQuoted(scheduler) + " start-date=" + deadlineDate + " start-time=" + routerEnd.Format("15:04:05") + " interval=0s policy=read,write on-event=" + routerOSQuoted(event)
	if _, err := runner.Run(ctx, scheduleCmd); err != nil {
		return fmt.Errorf("schedule grace deadline: %w", err)
	}
	count, scheduleErr := runner.Run(ctx, "/system scheduler print count-only where name="+routerOSQuoted(scheduler))
	if scheduleErr != nil || strings.TrimSpace(count) != "1" {
		_, _ = runner.Run(ctx, "/system scheduler remove [find name="+routerOSQuoted(scheduler)+"]")
		return fmt.Errorf("router did not confirm the grace deadline scheduler")
	}
	cmd := fmt.Sprintf("/ip hotspot user add name=%s mac-address=%s server=%s password=%s profile=default comment=%s limit-uptime=%ds limit-bytes-total=%d", name, g.ClientMAC, routerOSQuoted(server), routerOSQuoted(profileFields["mac-auth-password"]), routerOSQuoted(comment), g.Minutes*60, int64(g.DataMB)*1000000)
	if _, err := runner.Run(ctx, cmd); err != nil {
		_, _ = runner.Run(ctx, "/system scheduler remove [find name="+routerOSQuoted(scheduler)+"]")
		return fmt.Errorf("create temporary Hotspot user: %w", err)
	}
	out, err = runner.Run(ctx, read)
	if err == nil {
		err = verifyGraceUser(parseValueList(out), g, server, comment)
	}
	if err != nil {
		_, _ = runner.Run(ctx, "/system scheduler remove [find name="+routerOSQuoted(scheduler)+"]")
		// Only remove the record created for this request.
		_, _ = runner.Run(ctx, "/ip hotspot user remove [find name="+name+" comment="+routerOSQuoted(comment)+"]")
		return fmt.Errorf("temporary allowance verification failed: %w", err)
	}
	return nil
}

func verifyGraceUser(user map[string]string, g GuestGrace, server, comment string) error {
	bytes, err := strconv.ParseInt(user["limit-bytes-total"], 10, 64)
	if err != nil || bytes != int64(g.DataMB)*1000000 || parseUptimeSeconds(user["limit-uptime"]) != g.Minutes*60 || user["comment"] != comment || user["server"] != server || strings.ToUpper(user["mac-address"]) != g.ClientMAC || user["disabled"] == "yes" || user["disabled"] == "true" {
		return fmt.Errorf("router did not report the requested time, data and client binding")
	}
	return nil
}

// DecideGuestAccess switches only this purchase's temporary account to paid
// limits, or removes it on rejection. Counters remain intact: paid limits add
// the purchased allowance to usage already consumed during the grace session.
func (a *DeviceApplier) DecideGuestAccess(ctx context.Context, businessID, deviceID, requestID, clientMAC string, approve bool, minutes, dataMB int) error {
	mac, err := net.ParseMAC(clientMAC)
	if err != nil || len(mac) != 6 || !guestRequestIDPattern.MatchString(requestID) || minutes < 0 || dataMB < 0 {
		return fmt.Errorf("invalid guest access decision")
	}
	runner, err := runnerForDevice(ctx, a.src, a.factory, a.timeout, businessID, deviceID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	name := routerOSQuoted(strings.ToUpper(mac.String()))
	graceComment := "mega-magic:grace:" + requestID
	paidComment := "mega-magic:paid:" + requestID
	read := "/ip hotspot user print value-list detail where name=" + name
	out, err := runner.Run(ctx, read)
	if err != nil {
		return err
	}
	user := parseValueList(out)
	removeScheduler := "/system scheduler remove [find name=" + routerOSQuoted("mm-grace-"+requestID) + "]"
	if user["name"] == "" && !approve {
		_, err := runner.Run(ctx, removeScheduler)
		return err
	}
	if approve && user["comment"] == paidComment {
		if _, err := runner.Run(ctx, removeScheduler); err != nil {
			return err
		}
		_, err := runner.Run(ctx, "/ip hotspot active remove [find user="+name+"]")
		return err
	} // retry after a database commit failure
	if user["comment"] != graceComment {
		return fmt.Errorf("the Hotspot user does not belong to this purchase")
	}
	if !approve {
		if _, err := runner.Run(ctx, "/system scheduler remove [find name="+routerOSQuoted("mm-grace-"+requestID)+"]"); err != nil {
			return err
		}
		if _, err := runner.Run(ctx, "/ip hotspot active remove [find user="+name+"]"); err != nil {
			return err
		}
		_, err := runner.Run(ctx, "/ip hotspot user remove [find name="+name+" comment="+routerOSQuoted(graceComment)+"]")
		return err
	}
	if minutes == 0 && dataMB == 0 {
		return fmt.Errorf("the paid plan requires a time or data allowance")
	}
	if minutes > 0 {
		if err := schedulePaidGuestExpiry(ctx, runner, requestID, name, paidComment, minutes); err != nil {
			return err
		}
	}
	var bytes int64
	if dataMB > 0 {
		in, e := strconv.ParseInt(user["bytes-in"], 10, 64)
		if e != nil || in < 0 {
			return fmt.Errorf("could not read guest upload usage")
		}
		out, e := strconv.ParseInt(user["bytes-out"], 10, 64)
		if e != nil || out < 0 {
			return fmt.Errorf("could not read guest download usage")
		}
		bytes = in + out + int64(dataMB)*1000000
	}
	seconds := 0
	if minutes > 0 {
		seconds = parseUptimeSeconds(user["uptime"]) + minutes*60
	}
	cmd := fmt.Sprintf("/ip hotspot user set [find name=%s comment=%s] disabled=no limit-uptime=%ds limit-bytes-total=%d comment=%s", name, routerOSQuoted(graceComment), seconds, bytes, routerOSQuoted(paidComment))
	if _, err := runner.Run(ctx, cmd); err != nil {
		return err
	}
	out, err = runner.Run(ctx, read)
	if err != nil {
		return err
	}
	verified := parseValueList(out)
	actualBytes, e := strconv.ParseInt(verified["limit-bytes-total"], 10, 64)
	if e != nil || actualBytes != bytes || parseUptimeSeconds(verified["limit-uptime"]) != seconds || verified["comment"] != paidComment {
		return fmt.Errorf("paid allowance verification failed")
	}
	if _, err := runner.Run(ctx, "/system scheduler remove [find name="+routerOSQuoted("mm-grace-"+requestID)+"]"); err != nil {
		return err
	}
	// Re-authentication is required for an already active session to pick up
	// the new caps. MAC login reconnects it using the same bound account.
	_, err = runner.Run(ctx, "/ip hotspot active remove [find user="+name+"]")
	return err
}
