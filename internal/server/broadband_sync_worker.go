package server

import (
	"context"
	"time"
)

// Accounting is an independent read-only worker, not gated by Hotspot or an app
// subscription. Expired management access must not erase existing customer usage.
// Each scan is bounded and retries unavailable routers on the next pass.
func (s *Server) runBroadbandSync(ctx context.Context) {
	if s.broadband == nil || s.db == nil || !s.cfg.Provisioner.Applier.Enabled {
		return
	}
	scan := func() {
		cursorBiz, cursorDevice := "", ""
		for ctx.Err() == nil {
			read, cancel := context.WithTimeout(ctx, 5*time.Second)
			rows, e := s.db.QueryContext(read, `SELECT DISTINCT business_id::text,device_id::text FROM broadband_accounts
    WHERE NOT retired AND (business_id::text,device_id::text)>($1,$2) ORDER BY business_id::text,device_id::text LIMIT 25`, cursorBiz, cursorDevice)
			if e != nil {
				cancel()
				return
			} // Schema may not yet have been deployed.
			var pairs [][2]string
			for rows.Next() {
				var pair [2]string
				if e = rows.Scan(&pair[0], &pair[1]); e != nil {
					break
				}
				pairs = append(pairs, pair)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			cancel()
			if e != nil {
				return
			}
			for _, pair := range pairs {
				work, stop := context.WithTimeout(ctx, 60*time.Second)
				_, e = s.broadband.SyncBroadbandRouter(work, pair[0], pair[1])
				stop()
				if e != nil {
					s.sugar.Warnw("Broadband synchronization pending", "business_id", pair[0], "device_id", pair[1])
				}
			}
			if len(pairs) < 25 {
				return
			}
			cursorBiz, cursorDevice = pairs[len(pairs)-1][0], pairs[len(pairs)-1][1]
		}
	}
	scan()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scan()
		}
	}
}
