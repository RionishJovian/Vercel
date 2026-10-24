package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"net/http"
	"time"
)

// RequestAccountRenewal creates no payment/grant. It is usable after app expiry.
func (h *AuthHandler) RequestAccountRenewal(c *gin.Context) {
	var in struct {
		Plan string `json:"plan"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || (in.Plan != "monthly" && in.Plan != "annual") {
		c.JSON(400, gin.H{"error": "monthly or annual plan required"})
		return
	}
	var id, plan string
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		if h.IndividualPassesEnabled {
			status, e := database.AccountPassStatusTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), true, false, time.Now().UTC())
			if e != nil {
				return e
			}
			if !status.Authorized {
				return &httpError{status: 403, msg: "Only the authorized client login may request renewal"}
			}
		}
		if _, e := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "renewal:"+auth.GetUserID(c)); e != nil {
			return e
		}
		e := tx.QueryRow(`SELECT id,plan FROM app_account_pass_renewals WHERE user_id=$1 AND business_id=$2 AND status='pending'`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&id, &plan)
		if e == nil {
			if plan != in.Plan {
				return &httpError{status: 409, msg: "A different renewal is already pending"}
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		return tx.QueryRow(`INSERT INTO app_account_pass_renewals(user_id,business_id,plan)VALUES($1,$2,$3)RETURNING id`, auth.GetUserID(c), auth.GetBusinessID(c), in.Plan).Scan(&id)
	})
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			c.JSON(he.status, gin.H{"error": he.msg})
		} else {
			c.JSON(503, gin.H{"error": "Renewal request could not be confirmed. Refresh before retrying."})
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"id": id, "status": "pending", "plan": in.Plan})
}
func (h *AuthHandler) AccountRenewals(c *gin.Context) {
	var raw []byte
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COALESCE(jsonb_agg(q),'[]'::jsonb) FROM(SELECT id,plan,status,created_at,decided_at FROM app_account_pass_renewals WHERE user_id=$1 AND business_id=$2 ORDER BY created_at DESC LIMIT 20)q`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&raw)
	})
	if err != nil {
		c.JSON(503, gin.H{"error": "renewal history unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"requests": json.RawMessage(raw)})
}
func (h *PlatformHandler) AccountRenewalInbox(c *gin.Context) {
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.Status(403)
		return
	}
	var raw []byte
	err := h.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(jsonb_agg(q),'[]'::jsonb) FROM(SELECT r.id,r.user_id,r.business_id,u.email,r.plan,r.status,r.created_at FROM app_account_pass_renewals r JOIN users u ON u.user_id=r.user_id AND u.business_id=r.business_id WHERE r.status='pending' ORDER BY r.created_at,r.id LIMIT 100)q`).Scan(&raw)
	if err != nil {
		c.JSON(503, gin.H{"error": "renewal inbox unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"requests": json.RawMessage(raw)})
}
func (h *PlatformHandler) DecideAccountRenewal(c *gin.Context) {
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.Status(403)
		return
	}
	id := c.Param("id")
	if !appPassUUID.MatchString(id) {
		c.Status(400)
		return
	}
	var in struct {
		Approve *bool `json:"approve"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || in.Approve == nil {
		c.JSON(400, gin.H{"error": "explicit approval/rejection required"})
		return
	}
	tx, e := h.db.BeginTx(c.Request.Context(), nil)
	if e != nil {
		c.Status(503)
		return
	}
	defer tx.Rollback()
	var user, biz, plan, status string
	e = tx.QueryRow(`SELECT user_id,business_id FROM app_account_pass_renewals WHERE id=$1`, id).Scan(&user, &biz)
	if errors.Is(e, sql.ErrNoRows) {
		c.Status(404)
		return
	}
	if e != nil {
		c.Status(503)
		return
	}
	if _, e = tx.Exec(`SELECT user_id FROM users WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, user, biz); e != nil {
		c.Status(503)
		return
	}
	e = tx.QueryRow(`SELECT plan,status FROM app_account_pass_renewals WHERE id=$1 AND user_id=$2 AND business_id=$3 FOR UPDATE`, id, user, biz).Scan(&plan, &status)
	if e != nil {
		c.Status(503)
		return
	}
	if status != "pending" {
		c.JSON(409, gin.H{"error": "renewal already decided"})
		return
	}
	var revision int64
	now := time.Now().UTC()
	decision := "rejected"
	if *in.Approve {
		if h.IndividualPassesEnabled {
			var primary string
			if e = tx.QueryRow(`SELECT user_id FROM business_app_primary_users WHERE business_id=$1 FOR SHARE`, biz).Scan(&primary); e != nil || primary != user {
				c.JSON(409, gin.H{"error": "Renewal target is not the authorized client login"})
				return
			}
		}

		e = tx.QueryRow(`SELECT revision FROM app_account_passes WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, user, biz).Scan(&revision)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			c.Status(503)
			return
		}
		revision, e = database.ActivateAccountPassTXN(tx, user, biz, auth.GetUserID(c), plan, revision, now)
		if e != nil {
			c.Status(503)
			return
		}
		decision = "approved"
	}
	if _, e = tx.Exec(`UPDATE app_account_pass_renewals SET status=$2,decided_at=$3,decided_by=$4,pass_revision=NULLIF($5,0) WHERE id=$1`, id, decision, now, auth.GetUserID(c), revision); e != nil {
		c.Status(503)
		return
	}
	if e = tx.Commit(); e != nil {
		c.JSON(503, gin.H{"error": "decision not confirmed; refresh before retrying"})
		return
	}
	c.JSON(200, gin.H{"status": decision, "pass_revision": revision})
}
