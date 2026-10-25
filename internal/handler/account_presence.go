package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"net/http"
)

// Presence updates only the already-authenticated session, never a client user ID.
// Session material remains private in auth_sessions; no new tenant grants.
func (h *AuthHandler) Presence(c *gin.Context) {
	var req struct {
		Visible     *bool `json:"visible" binding:"required"`
		IdleSeconds *int  `json:"idle_seconds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Visible == nil || req.IdleSeconds == nil || *req.IdleSeconds < 0 || *req.IdleSeconds > 86400 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "visible and idle_seconds (0–86400) required"})
		return
	}
	sid := c.GetString("auth_session_id")
	if sid == "" {
		c.JSON(401, gin.H{"error": "authenticated session required"})
		return
	}
	result, err := h.db.ExecContext(c.Request.Context(), `UPDATE public.auth_sessions SET last_seen_at=clock_timestamp(),
 last_active_at=GREATEST(last_active_at,clock_timestamp()-($4*interval '1 second')),last_visible_at=CASE WHEN $5 THEN clock_timestamp() ELSE last_visible_at END
 WHERE session_id=$1 AND user_id=$2 AND business_id=$3 AND NOT revoked AND expires_at>clock_timestamp()`, sid, auth.GetUserID(c), auth.GetBusinessID(c), *req.IdleSeconds, *req.Visible)
	if err != nil {
		c.JSON(503, gin.H{"error": "presence unavailable"})
		return
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		c.JSON(401, gin.H{"error": "session no longer active"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(204)
}
