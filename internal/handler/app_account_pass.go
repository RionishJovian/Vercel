package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"regexp"
)

var appPassUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// RecordAccountPass is a platform-only record of explicit per-login terms.
// It never approves a payment, imports Store history or changes runtime gates.
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
		Plan        string    `json:"plan"`
		ActivatedAt time.Time `json:"activated_at"`
		ExpiresAt   time.Time `json:"expires_at"`
		Revoked     bool      `json:"revoked"`
		Revision    int64     `json:"revision"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || (in.Plan != "monthly" && in.Plan != "annual") || in.ActivatedAt.IsZero() || !in.ExpiresAt.After(in.ActivatedAt) || in.Revision < 0 {
		c.JSON(400, gin.H{"error": "plan, explicit activation/expiry dates and expected revision required"})
		return
	}
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(503, gin.H{"error": "pass recording unavailable"})
		return
	}
	defer tx.Rollback()
	var businessID string
	err = tx.QueryRowContext(c.Request.Context(), `SELECT business_id FROM users WHERE user_id=$1 FOR SHARE`, userID).Scan(&businessID)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(404, gin.H{"error": "account not found"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "account lookup unavailable"})
		return
	}
	var revoked any
	if in.Revoked {
		revoked = time.Now().UTC()
	}
	var revision int64
	err = tx.QueryRowContext(c.Request.Context(), `INSERT INTO app_account_passes(user_id,business_id,plan,activated_at,expires_at,revoked_at,recorded_by)
 SELECT $1,$2,$3,$4,$5,$6,$7 WHERE $8=0 OR EXISTS(SELECT 1 FROM app_account_passes WHERE user_id=$1 AND revision=$8)
 ON CONFLICT(user_id) DO UPDATE SET plan=excluded.plan,activated_at=excluded.activated_at,expires_at=excluded.expires_at,revoked_at=excluded.revoked_at,recorded_by=excluded.recorded_by,updated_at=now(),revision=app_account_passes.revision+1
 WHERE app_account_passes.revision=$8 AND app_account_passes.business_id=excluded.business_id RETURNING revision`, userID, businessID, in.Plan, in.ActivatedAt, in.ExpiresAt, revoked, auth.GetUserID(c), in.Revision).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(409, gin.H{"error": "pass changed; refresh before recording"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "pass was not recorded"})
		return
	}
	details, _ := json.Marshal(in)
	if _, err = tx.ExecContext(c.Request.Context(), `INSERT INTO app_account_pass_events(user_id,business_id,actor_id,revision,details) VALUES($1,$2,$3,$4,$5)`, userID, businessID, auth.GetUserID(c), revision, details); err != nil {
		c.JSON(503, gin.H{"error": "pass audit was not recorded"})
		return
	}
	if err = tx.Commit(); err != nil {
		c.JSON(503, gin.H{"error": "recording was not confirmed; refresh before retrying"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"recorded": true, "revision": revision, "scope": "account", "runtime_access_changed": false})
}

// AppPass reads only the authenticated login; business staff never inherit it.
func (h *AuthHandler) AppPass(c *gin.Context) {
	var raw []byte
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		return tx.QueryRowContext(c.Request.Context(), `SELECT jsonb_build_object('scope','account','authority','app_account_passes','enforced',false,
   'kind',COALESCE(p.plan,'none'),'activated_at',p.activated_at,'expires_at',p.expires_at,'revoked_at',p.revoked_at,
   'state',CASE WHEN p.user_id IS NULL THEN 'no_pass' WHEN p.revoked_at IS NOT NULL THEN 'revoked' WHEN p.expires_at<=now() THEN 'expired' WHEN p.activated_at>now() THEN 'scheduled' ELSE 'active' END)
   FROM (SELECT $1::uuid AS user_id) me LEFT JOIN app_account_passes p ON p.user_id=me.user_id AND p.business_id=$2`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&raw)
	})
	if err != nil {
		c.JSON(503, gin.H{"error": "individual app pass unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(200, "application/json", raw)
}
