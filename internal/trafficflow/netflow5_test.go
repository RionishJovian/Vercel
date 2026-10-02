package trafficflow

import (
	"encoding/binary"
	"testing"
	"time"
)

func datagram() []byte {
	b := make([]byte, 72)
	binary.BigEndian.PutUint16(b[:2], 5)
	binary.BigEndian.PutUint16(b[2:4], 1)
	binary.BigEndian.PutUint32(b[4:8], 10000)
	binary.BigEndian.PutUint32(b[8:12], 1700000000)
	binary.BigEndian.PutUint32(b[16:20], 42)
	copy(b[24:28], []byte{192, 168, 88, 20})
	copy(b[28:32], []byte{1, 1, 1, 1})
	binary.BigEndian.PutUint32(b[40:44], 10)
	binary.BigEndian.PutUint32(b[44:48], 12000)
	binary.BigEndian.PutUint32(b[48:52], 8000)
	binary.BigEndian.PutUint32(b[52:56], 9000)
	binary.BigEndian.PutUint16(b[56:58], 50000)
	binary.BigEndian.PutUint16(b[58:60], 443)
	b[62] = 6
	return b
}

func TestDecode(t *testing.T) {
	x, err := DecodeV5(datagram())
	if err != nil {
		t.Fatal(err)
	}
	r := x.Records[0]
	if x.Sequence != 42 || r.Source.String() != "192.168.88.20" || r.Destination.String() != "1.1.1.1" || r.Bytes != 12000 || r.Packets != 10 || r.DestinationPort != 443 || r.Protocol != 6 {
		t.Fatalf("wrong decoded metadata: %+v", x)
	}
	if !r.StartedAt.Equal(x.ExportedAt.Add(-2*time.Second)) || !r.EndedAt.Equal(x.ExportedAt.Add(-time.Second)) {
		t.Fatal("wrong flow timestamps")
	}
	// Uptime wraps every ~49 days. A flow crossing the rollover remains valid.
	b := datagram()
	binary.BigEndian.PutUint32(b[4:8], 100)
	binary.BigEndian.PutUint32(b[48:52], ^uint32(0)-99)
	binary.BigEndian.PutUint32(b[52:56], 50)
	x, err = DecodeV5(b)
	if err != nil || x.ExportedAt.Sub(x.Records[0].StartedAt) != 200*time.Millisecond {
		t.Fatal("rollover failed", err)
	}
}

func TestRejectMalformed(t *testing.T) {
	for _, change := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:23] }, func(b []byte) []byte { return b[:71] }, func(b []byte) []byte { return append(b, 0) },
		func(b []byte) []byte { b[1] = 9; return b }, func(b []byte) []byte { b[3] = 31; return b },
		func(b []byte) []byte { binary.BigEndian.PutUint32(b[12:16], 1000000000); return b },
		func(b []byte) []byte { binary.BigEndian.PutUint32(b[48:52], 9500); return b },
		func(b []byte) []byte { binary.BigEndian.PutUint16(b[22:24], 0xc001); return b },
	} {
		if _, err := DecodeV5(change(datagram())); err == nil {
			t.Fatal("malformed datagram accepted")
		}
	}
}

func TestSamplingIsPreserved(t *testing.T) {
	b := datagram()
	binary.BigEndian.PutUint16(b[22:24], 0x400a)
	x, err := DecodeV5(b)
	if err != nil || x.SamplingMode != 1 || x.SamplingInterval != 10 || x.Records[0].Bytes != 12000 {
		t.Fatal("sampling must remain explicit, no invented scaling", err)
	}
}

func FuzzDecodeV5(f *testing.F) {
	f.Add(datagram())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeV5(b) })
}
