package handler

import "github.com/gin-gonic/gin"

// ClientStaffInvitationsUnavailable retires the public redemption path as well
// as the client-facing creation UI. Previously issued invitations remain stored
// for audit; they cannot create a second authorized client login.
func ClientStaffInvitationsUnavailable(c *gin.Context) {
	c.AbortWithStatusJSON(410, gin.H{"error": "Client accounts use one authorized login. Contact the platform administrator for account changes.", "code": "staff_invitations_retired"})
}

// Legacy coin adjustments are historical tooling, not current app features.
func LegacyCoinWritesUnavailable(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled {
			c.AbortWithStatusJSON(409, gin.H{"error": "Legacy coin adjustments are retired under individual app passes", "code": "legacy_coins_retired"})
			return
		}
		c.Next()
	}
}
