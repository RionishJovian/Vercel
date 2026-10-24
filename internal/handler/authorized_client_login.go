package handler

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"net/http"
	"strings"
)

// AuthorizeClientLogin resolves a legacy workspace explicitly, never by role
// promotion or deleting former users. Expected identity prevents stale changes.
func (h *PlatformHandler) AuthorizeClientLogin(c *gin.Context) {
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.Status(403)
		return
	}
	user := c.Param("userId")
	if !appPassUUID.MatchString(user) {
		c.Status(400)
		return
	}
	var in struct {
		Expected *string `json:"expected_primary_user_id"`
		Reason   string  `json:"reason"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || in.Expected == nil || (*in.Expected != "" && !appPassUUID.MatchString(*in.Expected)) || len(strings.TrimSpace(in.Reason)) < 5 || len(in.Reason) > 500 {
		c.JSON(400, gin.H{"error": "Expected authorized login and a review reason are required"})
		return
	}
	tx, e := h.db.BeginTx(c.Request.Context(), nil)
	if e != nil {
		c.Status(503)
		return
	}
	defer tx.Rollback()
	var biz, role string
	var active bool
	e = tx.QueryRow(`SELECT business_id,user_role,is_active FROM users WHERE user_id=$1 FOR UPDATE`, user).Scan(&biz, &role, &active)
	if errors.Is(e, sql.ErrNoRows) {
		c.Status(404)
		return
	}
	if e != nil {
		c.Status(503)
		return
	}
	if role != string(model.RoleOwner) || !active {
		c.JSON(409, gin.H{"error": "Choose an enabled client owner login; roles are not changed by this operation"})
		return
	}
	if _, e = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "primary-login:"+biz); e != nil {
		c.Status(503)
		return
	}
	var previous string
	e = tx.QueryRow(`SELECT user_id FROM business_app_primary_users WHERE business_id=$1 FOR UPDATE`, biz).Scan(&previous)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		c.Status(503)
		return
	}
	if previous != *in.Expected {
		c.JSON(409, gin.H{"error": "Authorized login changed. Refresh before reviewing again."})
		return
	}
	if previous == user {
		c.JSON(200, gin.H{"authorized_login": true, "changed": false})
		return
	}
	if _, e = tx.Exec(`INSERT INTO business_app_primary_users(business_id,user_id,updated_by)VALUES($1,$2,$3) ON CONFLICT(business_id) DO UPDATE SET user_id=excluded.user_id,updated_by=excluded.updated_by,updated_at=now()`, biz, user, auth.GetUserID(c)); e != nil {
		c.Status(503)
		return
	}
	if _, e = tx.Exec(`INSERT INTO business_app_primary_events(business_id,user_id,previous_user_id,changed_by,reason)VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5)`, biz, user, previous, auth.GetUserID(c), strings.TrimSpace(in.Reason)); e != nil {
		c.Status(503)
		return
	}
	if e = tx.Commit(); e != nil {
		c.JSON(503, gin.H{"error": "Selection not confirmed. Refresh before retrying."})
		return
	}
	c.JSON(200, gin.H{"authorized_login": true, "changed": true})
}
