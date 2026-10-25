package handler

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"strings"
)

func (h *GuestVoucherHandler) BatchReseller(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.DB == nil {
		c.JSON(503, gin.H{"error": "Shop assignment unavailable"})
		return
	}
	biz, user, batch := auth.GetBusinessID(c), auth.GetUserID(c), c.Param("batchId")
	if biz == "" || user == "" {
		c.Status(401)
		return
	}
	if !guestVoucherUUID.MatchString(batch) {
		c.Status(400)
		return
	}
	var body struct {
		ResellerID string  `json:"reseller_id"`
		Expected   *string `json:"expected_reseller_id"`
		Confirmed  bool    `json:"confirmed"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirmed || body.Expected == nil {
		c.JSON(400, gin.H{"error": "confirm the exact current shop assignment"})
		return
	}
	body.ResellerID = strings.ToLower(body.ResellerID)
	expected := strings.ToLower(*body.Expected)
	if body.ResellerID != "" && !guestVoucherUUID.MatchString(body.ResellerID) || expected != "" && !guestVoucherUUID.MatchString(expected) {
		c.Status(400)
		return
	}
	var out database.GuestBatchReseller
	e := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		var e error
		out, e = database.AssignGuestBatchResellerTXN(tx, biz, batch, body.ResellerID, expected, user)
		return e
	})
	if e != nil {
		switch {
		case errors.Is(e, sql.ErrNoRows):
			c.JSON(404, gin.H{"error": "batch not found"})
		case errors.Is(e, database.ErrBatchAssignmentChanged):
			c.JSON(409, gin.H{"error": e.Error()})
		case errors.Is(e, database.ErrBatchResellerUnavailable):
			c.JSON(409, gin.H{"error": e.Error()})
		default:
			c.JSON(503, gin.H{"error": "Shop assignment not confirmed. Refresh the batch before retrying."})
		}
		return
	}
	c.JSON(200, gin.H{"assignment": out, "codes_changed": false})
}
