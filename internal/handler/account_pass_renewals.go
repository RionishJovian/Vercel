package handler

import (
	"context"
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
	var in accountRenewalRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || (in.Plan != "monthly" && in.Plan != "annual") {
		c.JSON(400, gin.H{"error": "monthly or annual plan required"})
		return
	}
	if in.BankAccountID != "" || in.BankReference != "" || in.IdempotencyKey != "" || in.ExpectedAmount != 0 {
		h.requestPaidAccountRenewal(c, in)
		return
	}
	var id, plan string
	created := false
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
		e = tx.QueryRow(`INSERT INTO app_account_pass_renewals(user_id,business_id,plan)VALUES($1,$2,$3)RETURNING id`, auth.GetUserID(c), auth.GetBusinessID(c), in.Plan).Scan(&id)
		created = e == nil
		return e
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
	if created && h.NotifyRenewal != nil {
		h.NotifyRenewal(id)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"id": id, "status": "pending", "plan": in.Plan})
}
func (h *AuthHandler) AccountRenewals(c *gin.Context) {
	var raw []byte
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COALESCE(jsonb_agg(q),'[]'::jsonb) FROM(SELECT id,plan,status,created_at,decided_at,amount_mmk,bank_reference,bank_snapshot,idempotency_key FROM app_account_pass_renewals WHERE user_id=$1 AND business_id=$2 ORDER BY created_at DESC LIMIT 20)q`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&raw)
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
	err := h.db.QueryRowContext(c.Request.Context(), `SELECT COALESCE(jsonb_agg(q),'[]'::jsonb) FROM(SELECT r.id,r.user_id,r.business_id,u.email,r.plan,r.status,r.created_at,r.amount_mmk,r.bank_reference,r.bank_snapshot FROM app_account_pass_renewals r JOIN users u ON u.user_id=r.user_id AND u.business_id=r.business_id WHERE r.status='pending' ORDER BY r.created_at,r.id LIMIT 100)q`).Scan(&raw)
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
		Approve         *bool `json:"approve"`
		PaymentVerified bool  `json:"payment_verified"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil || in.Approve == nil {
		c.JSON(400, gin.H{"error": "explicit approval/rejection required"})
		return
	}
	result, err := h.decideAccountRenewal(c.Request.Context(), auth.GetUserID(c), id, *in.Approve, in.PaymentVerified)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			c.JSON(he.status, gin.H{"error": he.msg})
		} else {
			c.JSON(503, gin.H{"error": "Renewal decision unavailable"})
		}
		return
	}
	c.JSON(200, result)
}

// Shared by HTTP and the platform bot; payment verification, row locks and
// exactly-once pass activation stay in one transaction.
func (h *PlatformHandler) decideAccountRenewal(ctx context.Context, actorID, id string, approve, paymentVerified bool) (gin.H, error) {
	if !appPassUUID.MatchString(id) {
		return nil, &httpError{status: 400, msg: "Invalid renewal ID"}
	}
	var actorOK bool
	if err := h.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN businesses b ON b.business_id=u.business_id WHERE u.user_id=$1 AND u.user_role='DEVELOPER' AND u.is_active AND b.is_active)`, actorID).Scan(&actorOK); err != nil || !actorOK {
		return nil, &httpError{status: 403, msg: "Platform administrator required"}
	}
	tx, e := h.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
	}
	defer tx.Rollback()
	var user, biz, plan, status string
	var amount sql.NullInt64
	e = tx.QueryRow(`SELECT user_id,business_id FROM app_account_pass_renewals WHERE id=$1`, id).Scan(&user, &biz)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, &httpError{status: 404, msg: "Renewal decision could not be confirmed"}
	}
	if e != nil {
		return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
	}
	if _, e = tx.Exec(`SELECT user_id FROM users WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, user, biz); e != nil {
		return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
	}
	e = tx.QueryRow(`SELECT plan,status,amount_mmk FROM app_account_pass_renewals WHERE id=$1 AND user_id=$2 AND business_id=$3 FOR UPDATE`, id, user, biz).Scan(&plan, &status, &amount)
	if e != nil {
		return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
	}
	if status != "pending" {
		return nil, &httpError{status: 409, msg: "renewal already decided"}
	}
	var revision int64
	now := time.Now().UTC()
	decision := "rejected"
	if approve {
		if amount.Valid && !paymentVerified {
			return nil, &httpError{status: 400, msg: "Verify the bank payment before approving this paid pass request"}
		}
		if h.IndividualPassesEnabled {
			var primary string
			if e = tx.QueryRow(`SELECT user_id FROM business_app_primary_users WHERE business_id=$1 FOR SHARE`, biz).Scan(&primary); e != nil || primary != user {
				return nil, &httpError{status: 409, msg: "Renewal target is not the authorized client login"}
			}
			policy, e := database.AccountPassStatusTXN(tx, biz, user, true, false, now)
			if e != nil {
				return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
			}
			if !policy.Authorized {
				return nil, &httpError{status: 409, msg: "This login is not an authorized client workspace owner"}
			}
		}

		e = tx.QueryRow(`SELECT revision FROM app_account_passes WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, user, biz).Scan(&revision)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
		}
		if amount.Valid {
			revision, e = database.RenewAccountPassTXN(tx, user, biz, actorID, plan, revision, now)
		} else {
			revision, e = database.ActivateAccountPassTXN(tx, user, biz, actorID, plan, revision, now)
		}
		if e != nil {
			return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
		}
		decision = "approved"
	}
	if _, e = tx.Exec(`UPDATE app_account_pass_renewals SET status=$2,decided_at=$3,decided_by=$4,pass_revision=NULLIF($5,0) WHERE id=$1`, id, decision, now, actorID, revision); e != nil {
		return nil, &httpError{status: 503, msg: "Renewal decision could not be confirmed"}
	}
	if e = tx.Commit(); e != nil {
		return nil, &httpError{status: 503, msg: "decision not confirmed; refresh before retrying"}
	}
	return gin.H{"status": decision, "pass_revision": revision}, nil
}
