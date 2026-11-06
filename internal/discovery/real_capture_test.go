package discovery

import (
	"encoding/hex"
	"net/netip"
	"testing"
	"time"
)

// realHAPAX2 is a real MNDP announcement captured on 2026-11-06 from a hAP ax2 (RouterOS 7.25) over its own Wi-Fi.
// The MAC tail, the software id and the IPv6 tail are overwritten with same-length placeholders; everything else is as received.
const realHAPAX2 = "" +
	"050000000001000648a98a0000060005000368415000070018372e3235203230" +
	"32362d31302d32322031343a31323a3030000800084d696b726f54696b000a00" +
	"04e3020000000b0009585858582d58585858000c001043353269472d35486178" +
	"443248617844000e0001010010000c6272696467652f77696669320012000100" +
	"001300017900110004c0a85801000f0010fe800000000000004aa98afffe0000" +
	"01"

func TestParseRealHAPAX2Capture(t *testing.T) {
	raw, err := hex.DecodeString(realHAPAX2)
	if err != nil || len(raw) != 161 {
		t.Fatalf("fixture: %v len=%d", err, len(raw))
	}
	o, err := MikroTik{}.Parse(raw, netip.MustParseAddrPort("192.168.88.1:5678"), time.Now())
	if err != nil {
		t.Fatalf("a real packet must parse: %v", err)
	}
	if o.MAC != "48:A9:8A:00:00:06" || o.Identity != "hAP" || o.Board != "C52iG-5HaxD2HaxD" || o.Platform != "MikroTik" ||
		o.Version != "7.25 2026-10-22 14:12:00" || o.Interface != "bridge/wifi2" || o.ClaimedIP != "192.168.88.1" || o.IPMismatch {
		t.Fatalf("unexpected real-packet fields: %+v", o)
	}
	if o.DeviceKey == "" || len(o.DeviceKey) > 20 {
		t.Fatalf("software id must become a short hash, got %q", o.DeviceKey)
	}
}

func TestOneRouterAnnouncingFromTwoInterfacesIsOneDevice(t *testing.T) {
	raw, _ := hex.DecodeString(realHAPAX2)
	from := netip.MustParseAddrPort("192.168.88.1:5678")
	t0 := time.Now()
	a, _ := MikroTik{}.Parse(raw, from, t0)
	// Same device, another interface: a different MAC TLV (bytes 8..13), same software id.
	other := append([]byte(nil), raw...)
	other[13] = 0x07
	b, err := MikroTik{}.Parse(other, from, t0.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	c := NewCollector()
	c.Add(a)
	c.Add(b)
	s := c.Snapshot()
	if len(s) != 1 || len(s[0].MACs) != 2 || s[0].Count != 2 {
		t.Fatalf("one device heard on two interfaces must be one candidate with two MACs: %+v", s)
	}
}
