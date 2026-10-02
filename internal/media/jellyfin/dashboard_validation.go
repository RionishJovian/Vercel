package jellyfin

import (
	"errors"
	"math"
)

func ValidateDashboard(d Dashboard) error {
	bad := errors.New("invalid dashboard snapshot")
	if d.CollectedAt.IsZero() || len(d.Playback) > 100 || len(d.Unavailable) > 4 || len(d.Server.ServerName) > 256 || len(d.Server.Version) > 256 || len(d.Server.ID) > 256 || len(d.OperatingSystem) > 256 {
		return bad
	}
	if d.Counts != nil {
		for _, n := range []*int64{d.Counts.Movies, d.Counts.Series, d.Counts.Episodes, d.Counts.Songs, d.Counts.Albums} {
			if n != nil && *n < 0 {
				return bad
			}
		}
	}
	for _, n := range []*int{d.ActiveSessions, d.ActiveUsers} {
		if n != nil && (*n < 0 || *n > 100) {
			return bad
		}
	}
	percent := func(n *float64) bool {
		return n == nil || (!math.IsNaN(*n) && !math.IsInf(*n, 0) && *n >= 0 && *n <= 100)
	}
	for _, p := range d.Playback {
		if len(p.User) > 256 || len(p.Device) > 256 || len(p.Title) > 512 || len(p.Method) > 32 || !percent(p.Transcoding) {
			return bad
		}
	}
	for _, code := range d.Unavailable {
		switch code {
		case "server_details", "counts", "sessions", "media_volumes":
		default:
			return bad
		}
	}
	if d.Host != nil {
		if len(d.Host.OS) > 32 || !percent(d.Host.CPU) || !percent(d.Host.Memory) || len(d.Host.Volumes) > 20 {
			return bad
		}
		for _, v := range d.Host.Volumes {
			if len(v.Label) > 256 || v.Total == 0 || v.Free > v.Total {
				return bad
			}
		}
	}
	return nil
}
