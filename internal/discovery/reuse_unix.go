//go:build unix

package discovery

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// shareablePort lets the helper listen on the announcement port at the same time as other tools such as WinBox,
// which hold it open while running. Broadcast datagrams are delivered to every socket bound this way.
func shareablePort(_, _ string, c syscall.RawConn) error {
	var serr error
	if err := c.Control(func(fd uintptr) {
		if serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); serr != nil {
			return
		}
		serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	}); err != nil {
		return err
	}
	return serr
}
