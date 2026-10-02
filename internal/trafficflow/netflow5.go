// Package trafficflow decodes flow metadata only; it never captures payloads.
// No UDP listener is enabled. Future ingress must bind authenticated tunnel
// exporter identity to a tenant/gateway before accepting a datagram.
package trafficflow

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"time"
)

var ErrDatagram = errors.New("invalid or unsupported NetFlow v5 datagram")

type Export struct {
	Sequence   uint32
	EngineType uint8
	EngineID   uint8
	// Nonzero sampling mode means counts are sampled, not complete totals.
	SamplingMode     uint16
	SamplingInterval uint16
	ExportedAt       time.Time
	Records          []Record
}

type Record struct {
	Source          netip.Addr
	Destination     netip.Addr
	InputInterface  uint16
	OutputInterface uint16
	Packets         uint32
	Bytes           uint32
	StartedAt       time.Time
	EndedAt         time.Time
	SourcePort      uint16
	DestinationPort uint16
	Protocol        uint8
}

// DecodeV5 is the IPv4 baseline. IPFIX/v9 templates and IPv6 are separate work.
// Exporter IDs and sequence numbers are not authentication credentials.
func DecodeV5(data []byte) (Export, error) {
	if len(data) < 24 || binary.BigEndian.Uint16(data[:2]) != 5 {
		return Export{}, ErrDatagram
	}
	count := int(binary.BigEndian.Uint16(data[2:4]))
	if count < 1 || count > 30 || len(data) != 24+count*48 {
		return Export{}, ErrDatagram
	}
	uptime := binary.BigEndian.Uint32(data[4:8])
	seconds, nanos := binary.BigEndian.Uint32(data[8:12]), binary.BigEndian.Uint32(data[12:16])
	if nanos >= 1000000000 {
		return Export{}, ErrDatagram
	}
	at := time.Unix(int64(seconds), int64(nanos)).UTC()
	sampling := binary.BigEndian.Uint16(data[22:24])
	mode, interval := sampling>>14, sampling&0x3fff
	if mode == 3 || (mode != 0 && interval == 0) {
		return Export{}, ErrDatagram
	}
	out := Export{Sequence: binary.BigEndian.Uint32(data[16:20]), EngineType: data[20], EngineID: data[21], SamplingMode: mode, SamplingInterval: interval, ExportedAt: at, Records: make([]Record, 0, count)}
	for offset := 24; offset < len(data); offset += 48 {
		r := data[offset : offset+48]
		first, last := binary.BigEndian.Uint32(r[24:28]), binary.BigEndian.Uint32(r[28:32])
		// Unsigned subtraction intentionally handles the 32-bit uptime rollover.
		startAge, endAge := uptime-first, uptime-last
		if startAge < endAge {
			return Export{}, ErrDatagram
		}
		source := [4]byte{r[0], r[1], r[2], r[3]}
		destination := [4]byte{r[4], r[5], r[6], r[7]}
		out.Records = append(out.Records, Record{Source: netip.AddrFrom4(source), Destination: netip.AddrFrom4(destination),
			InputInterface: binary.BigEndian.Uint16(r[12:14]), OutputInterface: binary.BigEndian.Uint16(r[14:16]),
			Packets: binary.BigEndian.Uint32(r[16:20]), Bytes: binary.BigEndian.Uint32(r[20:24]),
			StartedAt: at.Add(-time.Duration(startAge) * time.Millisecond), EndedAt: at.Add(-time.Duration(endAge) * time.Millisecond),
			SourcePort: binary.BigEndian.Uint16(r[32:34]), DestinationPort: binary.BigEndian.Uint16(r[34:36]), Protocol: r[38]})
	}
	return out, nil
}
