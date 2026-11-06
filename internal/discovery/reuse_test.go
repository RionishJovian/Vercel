//go:build unix

package discovery

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

// WinBox keeps UDP 5678 open while it runs. The helper must be able to listen alongside it and still receive broadcasts.
func TestHelperCanListenAlongsideAnotherProgramOnThePort(t *testing.T) {
	first, err := OpenUDP(testAdapter{port: 56781})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenUDP(testAdapter{port: 56781})
	if err != nil {
		t.Fatalf("a second listener on the same port must be allowed: %v", err)
	}
	defer second.Close()
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 56781})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("hello"))
	got := 0
	buf := make([]byte, 16)
	for _, l := range []*net.UDPConn{first, second} {
		_ = l.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		if n, _, err := l.ReadFromUDP(buf); err == nil && n == 5 {
			got++
		}
	}
	if got == 0 {
		t.Fatal("neither listener received the datagram")
	}
}

type testAdapter struct{ port int }

func (a testAdapter) Capabilities() Capabilities { return Capabilities{} }
func (a testAdapter) Port() int                  { return a.port }
func (testAdapter) Parse([]byte, netip.AddrPort, time.Time) (Observation, error) {
	return Observation{}, nil
}
