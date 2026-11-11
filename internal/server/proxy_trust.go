package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/netip"
)

// Only exact operator-established peers are eligible to supply XFF. Never
// promote a Docker subnet, hostname or X-Real-IP into a trust boundary.
func configureProxyTrust(r *gin.Engine, peers []string) error {
	for _, peer := range peers {
		if _, err := netip.ParseAddr(peer); err != nil {
			return fmt.Errorf("server.trusted_proxy_ips requires exact proxy IP addresses")
		}
	}
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	return r.SetTrustedProxies(peers)
}
