package database

import (
	"database/sql"
	"net"
	"time"
)

// nullString converts a *string to a *string for SQL (nil passthrough)
func nullString(s *string) *string {
	if s == nil {
		return nil
	}
	return s
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// nullTime converts a *time.Time to a *time.Time for SQL (nil passthrough)
func nullTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return t
}

// macAddrStr converts *net.HardwareAddr to *string for SQL binding
func macAddrStr(mac *net.HardwareAddr) *string {
	if mac == nil {
		return nil
	}
	s := mac.String()
	return &s
}

// mgmtIPStr converts *net.IP to *string for SQL binding
func mgmtIPStr(ip *net.IP) *string {
	if ip == nil {
		return nil
	}
	s := ip.String()
	return &s
}

// scanNullableString scans a sql.NullString into a *string
func scanNullableString(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// scanNullableTime scans a sql.NullTime into a *time.Time
func scanNullableTime(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

// scanMACAddr scans a sql.NullString macaddr into *net.HardwareAddr
func scanMACAddr(ns sql.NullString) *net.HardwareAddr {
	if !ns.Valid {
		return nil
	}
	parsedMAC, err := net.ParseMAC(ns.String)
	if err != nil {
		return nil
	}
	return &parsedMAC
}

// scanIP scans a sql.NullString inet into *net.IP
func scanIP(ns sql.NullString) *net.IP {
	if !ns.Valid {
		return nil
	}
	parsedIP := net.ParseIP(ns.String)
	if parsedIP == nil {
		return nil
	}
	return &parsedIP
}

// scanNullableInt scans a sql.NullInt64 into *int
func scanNullableInt(ni sql.NullInt64) *int {
	if ni.Valid {
		v := int(ni.Int64)
		return &v
	}
	return nil
}
