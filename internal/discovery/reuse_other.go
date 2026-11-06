//go:build !unix

package discovery

import "syscall"

// On other systems the port is bound normally; if another program holds it the helper says so.
func shareablePort(_, _ string, _ syscall.RawConn) error { return nil }
