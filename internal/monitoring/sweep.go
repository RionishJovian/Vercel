package monitoring

import (
	"context"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/database"
)

// Sweep periodically evaluates every managed peer's reachability state,
// independent of any HTTP request. This is the push side of the monitoring
// story: stale/offline peers generate monitor.peer.state events that the
// evaluator escalates to incidents — without anyone asking.
type Sweep struct {
	devices  *database.DeviceRepository
	monitor  *MonitoringService
	logger   *zap.SugaredLogger
	interval time.Duration
}

func NewSweep(devices *database.DeviceRepository, monitor *MonitoringService, logger *zap.SugaredLogger, interval time.Duration) *Sweep {
	return &Sweep{devices: devices, monitor: monitor, logger: logger, interval: interval}
}

// Start launches the sweep loop on a goroutine. It never returns; the loop
// runs until the process exits.
func (s *Sweep) Start() {
	go func() {
		if s.interval.Nanoseconds() == 0 {
			s.logger.Warnf("state sweep: zero interval, using 60s")
			s.interval = 60 * time.Second
		}
		for {
			s.pass()
			time.Sleep(s.interval)
		}
	}()
}

// pass enumerates all peers and evaluates each. One failing peer must not abort
// the fleet sweep, so errors are logged and swallowed per peer.
func (s *Sweep) pass() {
	keys, err := s.devices.ListAllPeerKeys()
	if err != nil {
		s.logger.Warnf("state sweep: list all peers: %v", err)
		return
	}
	if len(keys) == 0 {
		return
	}

	for _, k := range keys {
		ctx := context.Background()
		if state, err := s.monitor.EvaluatePeer(ctx, k.BusinessID, k.DeviceID); err != nil {
			s.logger.Warnf("state sweep: evaluate peer %s (%s): %v", k.DeviceID, k.BusinessID, err)
		} else {
			s.logger.Infof("state sweep: peer %s (%s) -> %s", k.DeviceID, k.BusinessID, state)
		}
	}
}