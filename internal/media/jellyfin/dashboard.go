package jellyfin

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Nullable counts distinguish an empty scanned library from an unsupported API.
type Counts struct {
	Movies   *int64 `json:"MovieCount"`
	Series   *int64 `json:"SeriesCount"`
	Episodes *int64 `json:"EpisodeCount"`
	Songs    *int64 `json:"SongCount"`
	Albums   *int64 `json:"AlbumCount"`
}
type Playback struct {
	User        string   `json:"user"`
	Device      string   `json:"device"`
	Title       string   `json:"title"`
	Paused      bool     `json:"paused"`
	Method      string   `json:"method"`
	Transcoding *float64 `json:"transcoding_percent,omitempty"`
}
type Volume struct {
	Label string `json:"label"`
	Total uint64 `json:"total_bytes"`
	Free  uint64 `json:"free_bytes"`
}
type HostMetrics struct {
	OS          string   `json:"os"`
	CPU         *float64 `json:"cpu_percent"`
	Memory      *float64 `json:"memory_percent"`
	MemoryTotal *uint64  `json:"memory_total_bytes"`
	Volumes     []Volume `json:"volumes"`
}
type Dashboard struct {
	CollectedAt     time.Time    `json:"collected_at"`
	Server          ServerInfo   `json:"server"`
	OperatingSystem string       `json:"operating_system"`
	Counts          *Counts      `json:"counts"`
	ActiveSessions  *int         `json:"active_sessions"`
	ActiveUsers     *int         `json:"active_users"`
	Playback        []Playback   `json:"playback"`
	Host            *HostMetrics `json:"host"`
	Unavailable     []string     `json:"unavailable"`
}

func (c *Client) Dashboard(ctx context.Context) (Dashboard, error) {
	d := Dashboard{Playback: []Playback{}, Unavailable: []string{}}
	info, err := c.Status(ctx)
	if err != nil {
		return d, err
	}
	d.Server = info
	var system struct {
		OperatingSystem            string
		OperatingSystemDisplayName string
	}
	if c.request(ctx, http.MethodGet, "/System/Info", nil, nil, true, &system) == nil {
		d.OperatingSystem = system.OperatingSystemDisplayName
		if d.OperatingSystem == "" {
			d.OperatingSystem = system.OperatingSystem
		}
	} else {
		d.Unavailable = append(d.Unavailable, "server_details")
	}
	var counts Counts
	if c.request(ctx, http.MethodGet, "/Items/Counts", nil, nil, true, &counts) == nil {
		d.Counts = &counts
	} else {
		d.Unavailable = append(d.Unavailable, "counts")
	}
	// Decode only supported fields; never relay API tokens, remote IPs or raw DTOs.
	var sessions []struct {
		UserId         string
		UserName       string
		DeviceName     string
		NowPlayingItem *struct{ Name string }
		PlayState      struct {
			IsPaused   bool
			PlayMethod string
		}
		TranscodingInfo *struct{ CompletionPercentage *float64 }
	}
	if c.request(ctx, http.MethodGet, "/Sessions", url.Values{"activeWithinSeconds": {"120"}}, nil, true, &sessions) == nil && len(sessions) <= 100 {
		n := len(sessions)
		d.ActiveSessions = &n
		users := map[string]bool{}
		for _, s := range sessions {
			if s.UserId != "" {
				users[s.UserId] = true
			}
			if s.NowPlayingItem == nil {
				continue
			}
			p := Playback{User: s.UserName, Device: s.DeviceName, Title: s.NowPlayingItem.Name, Paused: s.PlayState.IsPaused, Method: s.PlayState.PlayMethod}
			if s.TranscodingInfo != nil {
				p.Transcoding = s.TranscodingInfo.CompletionPercentage
			}
			d.Playback = append(d.Playback, p)
		}
		nUsers := len(users)
		d.ActiveUsers = &nUsers
	} else {
		d.Unavailable = append(d.Unavailable, "sessions")
	}
	libraries, e := c.Libraries(ctx)
	if e != nil {
		d.Unavailable = append(d.Unavailable, "media_volumes")
	}
	d.Host = collectHost(ctx, libraries)
	d.CollectedAt = time.Now().UTC()
	return d, nil
}
