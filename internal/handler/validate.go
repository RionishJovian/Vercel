package handler

// isUUID reports whether s is a canonical 8-4-4-4-12 hex UUID.
//
// Handlers validate path/body ids with this BEFORE touching the database.
// Without it, a malformed id reaches Postgres as a uuid cast and the driver
// error ("pq: invalid input syntax for type uuid ... (22P02)") is returned to
// the caller inside a 500 — both the wrong status and an information leak about
// the database/dialect. A bad id is a client error: 400.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') ||
				(c >= 'a' && c <= 'f') ||
				(c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
