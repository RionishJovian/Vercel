package handler

import (
	"database/sql"
	"errors"
	"mega-magic/internal/accountpass"
	"mega-magic/internal/database"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"regexp"
)

var appPassUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// RecordAccountPass activates server-timed individual access or revokes it.
// Operator approval does not assert payment settlement or import Store history.
func (h *PlatformHandler) RecordAccountPass(c *gin.Context) {
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.JSON(403, gin.H{"error": "developer access required"})
		return
	}
	userID := c.Param("userId")
	if !appPassUUID.MatchString(userID) {
		c.JSON(400, gin.H{"error": "valid account ID required"})
		return
	}

	var in struct {
		Plan     string `json:"plan"`
		Action   string `json:"action"`
		Revision int64  `json:"revision"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || (in.Action != "activate" && in.Action != "revoke") || (in.Action == "activate" && in.Plan != "monthly" && in.Plan != "annual") || in.Revision < 0 {
		c.JSON(400, gin.H{"error": "activation/revocation action, plan and expected revision required"})
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(503, gin.H{"error": "pass recording unavailable"})
		return
	}
	defer tx.Rollback()
	var businessID string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT business_id FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&businessID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"error": "account not found"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "account lookup unavailable"})
		return
	}

	if h.IndividualPassesEnabled {
		var primary string
		if e := tx.QueryRow(`SELECT user_id FROM business_app_primary_users WHERE business_id=$1 FOR SHARE`, businessID).Scan(&primary); e != nil || primary != userID {
			c.JSON(409, gin.H{"error": "Select this business's authorized primary login before activating its pass"})
			return
		}
	}
	now := time.Now().UTC()
	var revision int64
	if in.Action == "activate" {
		revision, err = database.ActivateAccountPassTXN(tx, userID, businessID, auth.GetUserID(c), in.Plan, in.Revision, now)
	} else {
		revision, err = database.RevokeAccountPassTXN(tx, userID, businessID, auth.GetUserID(c), in.Revision, now)
	}
	if errors.Is(err, database.ErrAccountPassRevision) {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "pass decision was not recorded"})
		return
	}

	if err = tx.Commit(); err != nil {
		c.JSON(503, gin.H{"error": "recording was not confirmed; refresh before retrying"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"recorded": true, "revision": revision, "scope": "account", "runtime_access_changed": h.IndividualPassesEnabled})
}

// AppPass reads only the authenticated login; business staff never inherit it.
func (h *AuthHandler) AppPass(c *gin.Context) {
	var status accountpass.Status
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		var err error
		status, err = database.AccountPassStatusTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), h.IndividualPassesEnabled, auth.GetUserRole(c) == string(model.RoleDeveloper), time.Now().UTC())
		return err
	})
	if err != nil {
		c.JSON(503, gin.H{"error": "individual app pass unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, status)
}
