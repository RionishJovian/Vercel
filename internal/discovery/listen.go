package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"time"
)

// PacketSource is receive-only on purpose: the listener cannot send anything.
type PacketSource interface {
	ReadFrom(p []byte) (int, net.Addr, error)
	SetReadDeadline(t time.Time) error
}

// Result of one listening run.
type Result struct {
	Observations []Observation `json:"observations"`
	Packets      int           `json:"packets"`
	Rejected     int           `json:"rejected"`
	Dropped      int           `json:"dropped"`
}

// Listen reads announcements until the window ends or ctx is cancelled.
// A run that hears nothing is a normal result: it proves nothing about whether a router is near.
func Listen(ctx context.Context, src PacketSource, a Adapter, window time.Duration, clock func() time.Time) (Result, error) {
	end := clock().Add(window)
	col := NewCollector()
	var res Result
	buf := make([]byte, maxPacket+1)
	for ctx.Err() == nil && clock().Before(end) {
		_ = src.SetReadDeadline(clock().Add(time.Second))
		n, addr, err := src.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return finish(col, res), err
		}
		res.Packets++
		var from netip.AddrPort
		if ua, ok := addr.(*net.UDPAddr); ok {
			from = ua.AddrPort()
		}
		o, err := a.Parse(buf[:n], from, clock())
		if err != nil {
			res.Rejected++
			continue
		}
		col.Add(o)
	}
	return finish(col, res), nil
}

func finish(c *Collector, r Result) Result {
	r.Observations, r.Dropped = c.Snapshot(), c.Dropped()
	return r
}

// OpenUDP opens the adapter's port for receiving only.
func OpenUDP(a Adapter) (*net.UDPConn, error) {
	return net.ListenUDP("udp4", &net.UDPAddr{Port: a.Port()})
}
