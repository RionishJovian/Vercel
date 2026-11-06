package discovery

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MNDP is MikroTik Neighbor Discovery: UDP 5678 broadcast.
// Layout from Wireshark's open-source dissector (packet-mndp.c): a 4-byte header (2 unknown bytes, then a
// big-endian sequence number), then repeated TLVs of big-endian 2-byte type, 2-byte length (value only), value.
// Checked on 2026-11-06 against a real capture from a hAP ax2 (RouterOS 7.25): TLV type/length are big-endian and the
// header is 4 bytes that this parser ignores. Other models, RouterOS versions and operating systems are not yet checked.
// The MAC TLV is the sending interface's address, and unknown TLVs (for example 0x12, 0x13) occur in real packets.
const (
	mndpPort      = 5678
	maxPacket     = 1500
	maxText       = 64
	mndpHeaderLen = 4

	tlvMAC        = 1
	tlvIdentity   = 5
	tlvVersion    = 7
	tlvPlatform   = 8
	tlvSoftwareID = 11
	tlvBoard      = 12
	tlvInterface  = 16
	tlvIPv4       = 17
)

var (
	ErrTooShort  = errors.New("packet too short")
	ErrTooLarge  = errors.New("packet too large")
	ErrTruncated = errors.New("tlv runs past the end of the packet")
	ErrNoMAC     = errors.New("no MAC address tlv")
	ErrBadField  = errors.New("invalid field")
)

// MikroTik decodes MNDP announcements.
type MikroTik struct{}

func (MikroTik) Port() int { return mndpPort }

func (MikroTik) Capabilities() Capabilities {
	return Capabilities{
		Brand: "MikroTik", Method: "MNDP (UDP 5678 broadcast)", Models: "RouterOS devices with neighbor discovery enabled on the interface",
		Evidence:       []string{"mac", "identity", "board", "platform", "version", "interface", "ipv4"},
		Authentication: "none: announcements are unauthenticated and can be spoofed",
	}
}

// clean makes advertised text safe to show: valid UTF-8, no control characters, bounded.
func clean(b []byte) string {
	if !utf8.Valid(b) {
		b = []byte(strings.ToValidUTF8(string(b), ""))
	}
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, string(b))
	s = strings.TrimSpace(s)
	if len(s) > maxText {
		s = strings.ToValidUTF8(s[:maxText], "")
	}
	return s
}

func (MikroTik) Parse(p []byte, from netip.AddrPort, now time.Time) (Observation, error) {
	if len(p) > maxPacket {
		return Observation{}, ErrTooLarge
	}
	if len(p) < mndpHeaderLen+4 {
		return Observation{}, ErrTooShort
	}
	o := Observation{Brand: "MikroTik", Method: "MNDP", Status: StatusNearbyUnverified, FirstSeen: now, LastSeen: now, Count: 1}
	var gotMAC bool
	for off := mndpHeaderLen; off < len(p); {
		if len(p)-off < 4 {
			return Observation{}, ErrTruncated
		}
		typ := binary.BigEndian.Uint16(p[off:])
		n := int(binary.BigEndian.Uint16(p[off+2:]))
		off += 4
		if n > len(p)-off {
			return Observation{}, ErrTruncated
		}
		val := p[off : off+n]
		off += n
		switch typ {
		case tlvMAC:
			if n != 6 {
				return Observation{}, fmt.Errorf("%w: mac length %d", ErrBadField, n)
			}
			mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", val[0], val[1], val[2], val[3], val[4], val[5])
			if gotMAC && mac != o.MAC {
				return Observation{}, fmt.Errorf("%w: conflicting mac", ErrBadField)
			}
			if val[0]&1 == 1 || mac == "00:00:00:00:00:00" || mac == "FF:FF:FF:FF:FF:FF" {
				return Observation{}, fmt.Errorf("%w: not a unicast device address", ErrBadField)
			}
			o.MAC, gotMAC = mac, true
		case tlvSoftwareID:
			// Only ever kept as a one-way hash: it is tied to the device's license.
			if n > 0 && n <= maxText {
				h := sha256.Sum256(val)
				o.DeviceKey = "sw:" + hex.EncodeToString(h[:8])
			}
		case tlvIdentity:
			o.Identity = clean(val)
		case tlvVersion:
			o.Version = clean(val)
		case tlvPlatform:
			o.Platform = clean(val)
		case tlvBoard:
			o.Board = clean(val)
		case tlvInterface:
			o.Interface = clean(val)
		case tlvIPv4:
			if n != 4 {
				return Observation{}, fmt.Errorf("%w: ipv4 length %d", ErrBadField, n)
			}
			a, _ := netip.AddrFromSlice(val)
			o.ClaimedIP = a.String()
		}
	}
	if !gotMAC {
		return Observation{}, ErrNoMAC
	}
	o.MACs = []string{o.MAC}
	if from.IsValid() {
		o.SourceIP = from.Addr().Unmap().String()
		if o.ClaimedIP != "" && o.ClaimedIP != o.SourceIP {
			o.IPMismatch = true
		}
	}
	return o, nil
}
