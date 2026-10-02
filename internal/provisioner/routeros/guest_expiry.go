package routeros

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// schedulePaidGuestExpiry uses the router's wall clock and calendar arithmetic,
// not uptime counters or string date comparisons. It only touches a user whose
// MAC name AND purchase comment match this request. It is installed before the
// paid write, so a partial failure cannot leave newly paid access unbounded.
func schedulePaidGuestExpiry(ctx context.Context, r CommandRunner, requestID, name, comment string, minutes int) error {
	if minutes < 1 || minutes > 525600 {
		return fmt.Errorf("paid guest elapsed validity must be 1-525600 minutes")
	}
	scheduler := "mm-paid-" + requestID
	count, err := r.Run(ctx, "/system scheduler print count-only where name="+routerOSQuoted(scheduler))
	if err != nil {
		return fmt.Errorf("read paid deadline scheduler")
	}
	if strings.TrimSpace(count) == "1" {
		return nil
	} // Never extend a retry.
	if strings.TrimSpace(count) != "0" {
		return fmt.Errorf("invalid paid deadline scheduler count")
	}
	out, err := r.Run(ctx, "/system clock print")
	if err != nil {
		return fmt.Errorf("read router clock for paid validity")
	}
	clock := parseValueList(out)
	layout, format := "2006-01-02 15:04:05", "2006-01-02"
	clockText := clock["date"] + " " + clock["time"]
	if strings.Contains(clock["date"], "/") {
		layout, format = "Jan/02/2006 15:04:05", "Jan/02/2006"
		if len(clockText) > 0 {
			clockText = strings.ToUpper(clockText[:1]) + clockText[1:]
		}
	}
	now, err := time.Parse(layout, clockText)
	if err != nil {
		return fmt.Errorf("invalid router clock for paid validity")
	}
	end := now.Add(time.Duration(minutes) * time.Minute)
	date := end.Format(format)
	if format == "Jan/02/2006" {
		date = strings.ToLower(date)
	}
	find := "[find name=" + name + " comment=" + routerOSQuoted(comment) + "]"
	event := ":if ([:len [/ip hotspot user find name=" + name + " comment=" + routerOSQuoted(comment) + "]] > 0) do={ /ip hotspot user disable " + find + "; /ip hotspot active remove [find user=" + name + "] }; /system scheduler remove [find name=" + routerOSQuoted(scheduler) + "]"
	cmd := "/system scheduler add name=" + routerOSQuoted(scheduler) + " start-date=" + date + " start-time=" + end.Format("15:04:05") + " interval=0s policy=read,write on-event=" + routerOSQuoted(event)
	if _, err := r.Run(ctx, cmd); err != nil {
		return fmt.Errorf("install paid elapsed deadline")
	}
	count, err = r.Run(ctx, "/system scheduler print count-only where name="+routerOSQuoted(scheduler))
	if err != nil || strings.TrimSpace(count) != "1" {
		return fmt.Errorf("paid deadline did not verify")
	}
	return nil
}
