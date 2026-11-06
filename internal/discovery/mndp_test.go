package discovery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// Every packet here is SYNTHETIC, built from the layout in Wireshark's dissector. None is a real router capture.
// Passing these tests is "Tested", not "Hardware Verified".

type tlv struct {
	typ uint16
	val []byte
}

func packet(tlvs ...tlv) []byte {
	b := []byte{0, 0, 0, 1} // 2 unknown bytes, big-endian sequence number
	for _, t := range tlvs {
		b = binary.BigEndian.AppendUint16(b, t.typ)
		b = binary.BigEndian.AppendUint16(b, uint16(len(t.val)))
		b = append(b, t.val...)
	}
	return b
}

var (
	mac    = tlv{tlvMAC, []byte{0x48, 0x8F, 0x5A, 0x01, 0x02, 0x03}}
	from   = netip.MustParseAddrPort("192.168.88.1:5678")
	t0     = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	parser = MikroTik{}
)

func full() []byte {
	return packet(mac, tlv{tlvIdentity, []byte("MikroTik")}, tlv{tlvVersion, []byte("7.19.4 (stable)")}, tlv{tlvPlatform, []byte("MikroTik")},
		tlv{tlvBoard, []byte("hAP ax^2")}, tlv{tlvInterface, []byte("bridge")}, tlv{tlvIPv4, []byte{192, 168, 88, 1}}, tlv{99, []byte{1, 2, 3}})
}

func TestParseValid(t *testing.T) {
	o, err := parser.Parse(full(), from, t0)
	if err != nil {
		t.Fatal(err)
	}
	if o.MAC != "48:8F:5A:01:02:03" || o.Board != "hAP ax^2" || o.Version != "7.19.4 (stable)" || o.Interface != "bridge" || o.ClaimedIP != "192.168.88.1" || o.SourceIP != "192.168.88.1" {
		t.Fatalf("unexpected: %+v", o)
	}
	if o.Status != StatusNearbyUnverified || o.IPMismatch {
		t.Fatalf("status/mismatch wrong: %+v", o)
	}
}

func TestParseMissingFieldsStayEmpty(t *testing.T) {
	o, err := parser.Parse(packet(mac), from, t0)
	if err != nil || o.Board != "" || o.Version != "" || o.Identity != "" || o.ClaimedIP != "" {
		t.Fatalf("missing fields must stay empty, got %+v err=%v", o, err)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	long := bytes.Repeat([]byte{'a'}, maxPacket)
	cases := map[string]struct {
		in  []byte
		err error
	}{
		"empty":             {nil, ErrTooShort},
		"header only":       {[]byte{0, 0, 0, 1}, ErrTooShort},
		"too large":         {append(full(), long...), ErrTooLarge},
		"length past end":   {append(packet(mac), 0x00, 0x05, 0xFF, 0xFF, 1), ErrTruncated},
		"dangling tlv head": {append(packet(mac), 0x00, 0x05), ErrTruncated},
		"no mac":            {packet(tlv{tlvIdentity, []byte("x")}), ErrNoMAC},
		"short mac":         {packet(tlv{tlvMAC, []byte{1, 2, 3}}), ErrBadField},
		"long mac":          {packet(tlv{tlvMAC, make([]byte, 8)}), ErrBadField},
		"bad ipv4 length":   {packet(mac, tlv{tlvIPv4, []byte{1, 2, 3}}), ErrBadField},
		"multicast mac":     {packet(tlv{tlvMAC, []byte{0x01, 0, 0x5E, 1, 2, 3}}), ErrBadField},
		"zero mac":          {packet(tlv{tlvMAC, make([]byte, 6)}), ErrBadField},
		"broadcast mac":     {packet(tlv{tlvMAC, bytes.Repeat([]byte{0xFF}, 6)}), ErrBadField},
		"conflicting mac":   {packet(mac, tlv{tlvMAC, []byte{0x48, 0x8F, 0x5A, 9, 9, 9}}), ErrBadField},
	}
	for name, c := range cases {
		if _, err := parser.Parse(c.in, from, t0); !errors.Is(err, c.err) {
			t.Errorf("%s: got %v, want %v", name, err, c.err)
		}
	}
}

func TestParseSanitisesText(t *testing.T) {
	o, err := parser.Parse(packet(mac, tlv{tlvIdentity, []byte("ro\x00u\x1b[31mter\n\xff\xfe")}, tlv{tlvBoard, bytes.Repeat([]byte("é"), 200)}), from, t0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(o.Identity, "\x00\x1b\n") || !strings.HasPrefix(o.Identity, "ro") {
		t.Fatalf("identity not cleaned: %q", o.Identity)
	}
	if len(o.Board) > maxText {
		t.Fatalf("board not bounded: %d", len(o.Board))
	}
}

func TestParseFlagsSourceMismatch(t *testing.T) {
	o, err := parser.Parse(full(), netip.MustParseAddrPort("192.168.88.77:5678"), t0)
	if err != nil || !o.IPMismatch {
		t.Fatalf("a packet claiming another address than it came from must be flagged: %+v %v", o, err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(full())
	f.Add([]byte{0, 0, 0, 1, 0, 1, 0, 6})
	f.Fuzz(func(t *testing.T, b []byte) {
		o, err := parser.Parse(b, from, t0)
		if err == nil && o.MAC == "" {
			t.Fatal("accepted a packet without a MAC")
		}
	})
}

func obs(mac, ip string, at time.Time) Observation {
	return Observation{MAC: mac, SourceIP: ip, FirstSeen: at, LastSeen: at, Count: 1, Status: StatusNearbyUnverified}
}

func TestCollectorDedupeAndMove(t *testing.T) {
	c := NewCollector()
	c.Add(obs("AA:00:00:00:00:01", "192.168.88.1", t0))
	c.Add(obs("AA:00:00:00:00:01", "192.168.88.1", t0.Add(30*time.Second)))
	if s := c.Snapshot(); len(s) != 1 || s[0].Count != 2 || s[0].IPChanged {
		t.Fatalf("duplicate MAC must merge: %+v", s)
	}
	c.Add(obs("AA:00:00:00:00:01", "192.168.1.1", t0.Add(60*time.Second)))
	if s := c.Snapshot(); !s[0].IPChanged || s[0].SourceIP != "192.168.1.1" {
		t.Fatalf("a moved device must be flagged and show its newest address: %+v", s)
	}
}

func TestCollectorRateLimitCapAndExpiry(t *testing.T) {
	c := NewCollector()
	c.Add(obs("AA:00:00:00:00:01", "10.0.0.1", t0))
	if c.Add(obs("AA:00:00:00:00:01", "10.0.0.1", t0.Add(200*time.Millisecond))) {
		t.Fatal("updates faster than the minimum interval must be ignored")
	}
	for i := 0; i < MaxCandidates+10; i++ {
		c.Add(obs(string([]byte{'B', byte('A' + i/26), byte('A' + i%26)}), "10.0.0.2", t0))
	}
	if n := len(c.Snapshot()); n != MaxCandidates || c.Dropped() == 0 {
		t.Fatalf("candidate flood must be bounded: %d kept, %d dropped", n, c.Dropped())
	}
	c.Expire(t0.Add(10*time.Minute), 5*time.Minute)
	if len(c.Snapshot()) != 0 {
		t.Fatal("old observations must expire")
	}
}

// fakeConn can only receive, like the real listener's PacketSource.
type fakeConn struct {
	packets [][]byte
	now     *time.Time
}

func (f *fakeConn) SetReadDeadline(time.Time) error { return nil }
func (f *fakeConn) ReadFrom(p []byte) (int, net.Addr, error) {
	*f.now = f.now.Add(2 * time.Second)
	if len(f.packets) == 0 {
		return 0, nil, os_deadline{}
	}
	b := f.packets[0]
	f.packets = f.packets[1:]
	return copy(p, b), &net.UDPAddr{IP: net.IPv4(192, 168, 88, 1), Port: 5678}, nil
}

type os_deadline struct{}

func (os_deadline) Error() string   { return "timeout" }
func (os_deadline) Timeout() bool   { return true }
func (os_deadline) Temporary() bool { return true }

func TestListenCollectsRejectsAndStops(t *testing.T) {
	now := t0
	conn := &fakeConn{now: &now, packets: [][]byte{full(), {1, 2, 3}, full(), packet(tlv{tlvIdentity, []byte("x")})}}
	res, err := Listen(context.Background(), conn, parser, 40*time.Second, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if res.Packets != 4 || res.Rejected != 2 || len(res.Observations) != 1 || res.Observations[0].Count != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestListenHearingNothingIsNotAnError(t *testing.T) {
	now := t0
	res, err := Listen(context.Background(), &fakeConn{now: &now}, parser, 10*time.Second, func() time.Time { return now })
	if err != nil || len(res.Observations) != 0 {
		t.Fatalf("silence must be a normal empty result: %+v %v", res, err)
	}
}

func TestAdapterDeclaresItsLimits(t *testing.T) {
	c := parser.Capabilities()
	if c.Brand != "MikroTik" || parser.Port() != 5678 || !strings.Contains(c.Authentication, "spoof") {
		t.Fatalf("adapter must declare brand, port and that announcements are unauthenticated: %+v", c)
	}
}
