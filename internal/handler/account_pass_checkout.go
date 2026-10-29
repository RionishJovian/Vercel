package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/store"
)

type accountRenewalRequest struct {
	Plan           string `json:"plan"`
	BankAccountID  string `json:"bank_account_id"`
	BankReference  string `json:"bank_reference"`
	IdempotencyKey string `json:"idempotency_key"`
	ExpectedAmount int    `json:"expected_amount_mmk"`
}

func (h *AuthHandler) AccountPassCheckoutOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.IndividualPassesEnabled {
		c.JSON(409, gin.H{"error": "Individual pass checkout is unavailable"})
		return
	}
	pricing, err := database.NewPricingRepository(h.db).GetPromo(c.Request.Context())
	if err != nil {
		c.JSON(503, gin.H{"error": "Pass prices are not published"})
		return
	}
	plans := gin.H{}
	for _, key := range []string{store.ProductMonthly, store.ProductAnnual} {
		product, e := store.ResolveProduct(key, 0, *pricing, time.Now().UTC())
		if e != nil {
			c.JSON(503, gin.H{"error": "Pass prices are not published"})
			return
		}
		plans[key] = gin.H{"amount_mmk": product.PriceMMK, "label": product.Label}
	}
	banks, err := database.NewStorePurchaseRepository(h.db).ListEnabledBankAccounts()
	if err != nil {
		c.JSON(503, gin.H{"error": "Payment accounts are unavailable"})
		return
	}
	valid := make([]model.StoreBankAccount, 0, len(banks))
	for _, bank := range banks {
		if strings.TrimSpace(bank.BankName) != "" && strings.TrimSpace(bank.HolderName) != "" && strings.TrimSpace(bank.AccountNumber) != "" {
			valid = append(valid, bank)
		}
	}
	c.JSON(200, gin.H{"plans": plans, "bank_accounts": valid, "available": len(valid) > 0, "currency": "MMK", "approval": "verified_bank_payment_then_platform_approval"})
}

func (h *AuthHandler) checkoutActor(tx *sql.Tx, c *gin.Context) error {
	if !h.IndividualPassesEnabled {
		return &httpError{status: 409, msg: "Individual pass checkout is unavailable"}
	}
	if auth.GetUserRole(c) == string(model.RoleDeveloper) {
		return &httpError{status: 403, msg: "Platform administrator accounts are exempt; no pass payment is required"}
	}
	var locked string
	if err := tx.QueryRow(`SELECT user_id FROM users WHERE user_id=$1 AND business_id=$2 AND is_active FOR UPDATE`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&locked); err != nil {
		return &httpError{status: 403, msg: "Active client account required"}
	}
	status, err := database.AccountPassStatusTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), true, false, time.Now().UTC())
	if err != nil {
		return err
	}
	if !status.Authorized {
		return &httpError{status: 403, msg: "Only the authorized client login may purchase this pass"}
	}
	_, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "renewal:"+auth.GetUserID(c))
	return err
}

func checkoutError(c *gin.Context, err error) {
	var he *httpError
	if errors.As(err, &he) {
		c.JSON(he.status, gin.H{"error": he.msg})
		return
	}
	c.JSON(503, gin.H{"error": "Payment request was not confirmed. Refresh its history before retrying; keep the same request ID."})
}

func (h *AuthHandler) requestPaidAccountRenewal(c *gin.Context, in accountRenewalRequest) {
	in.BankReference = strings.TrimSpace(in.BankReference)
	if !appPassUUID.MatchString(in.BankAccountID) || !appPassUUID.MatchString(in.IdempotencyKey) || len(in.BankReference) == 0 || len(in.BankReference) > 120 || strings.IndexFunc(in.BankReference, unicode.IsControl) >= 0 || in.ExpectedAmount <= 0 {
		c.JSON(400, gin.H{"error": "Select a payment account and provide the transfer reference, quoted amount and stable request ID"})
		return
	}
	var id, status string
	err := runInTenant(c, h.db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		if e := h.checkoutActor(tx, c); e != nil {
			return e
		}
		var plan, bank, reference string
		var amount int
		e := tx.QueryRow(`SELECT id,status,plan,bank_account_id,bank_reference,amount_mmk FROM app_account_pass_renewals WHERE user_id=$1 AND business_id=$2 AND idempotency_key=$3`, auth.GetUserID(c), auth.GetBusinessID(c), in.IdempotencyKey).Scan(&id, &status, &plan, &bank, &reference, &amount)
		if e == nil {
			if plan != in.Plan || !strings.EqualFold(bank, in.BankAccountID) || reference != in.BankReference || amount != in.ExpectedAmount {
				return &httpError{status: 409, msg: "This request ID already belongs to different payment details; refresh the original request"}
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var pending bool
		if e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM app_account_pass_renewals WHERE user_id=$1 AND business_id=$2 AND status='pending')`, auth.GetUserID(c), auth.GetBusinessID(c)).Scan(&pending); e != nil {
			return e
		}
		if pending {
			return &httpError{status: 409, msg: "A pass request is already awaiting review. Do not transfer another payment."}
		}
		var pricing model.PricingPromo
		var raw []byte
		if e = tx.QueryRow(`SELECT to_jsonb(p) FROM pricing_promo p WHERE id=true`).Scan(&raw); e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &pricing); e != nil {
			return e
		}
		product, e := store.ResolveProduct(in.Plan, 0, pricing, time.Now().UTC())
		if e != nil {
			return &httpError{status: 503, msg: "Pass price is unavailable"}
		}
		if product.PriceMMK != in.ExpectedAmount {
			return &httpError{status: 409, msg: "The published price changed. Refresh the price and contact the administrator if you already transferred money."}
		}
		var bankAccount model.StoreBankAccount
		e = tx.QueryRow(`SELECT id,holder_name,bank_name,account_number FROM platform_service_bank_accounts WHERE id=$1 AND enabled=true`, in.BankAccountID).Scan(&bankAccount.ID, &bankAccount.HolderName, &bankAccount.BankName, &bankAccount.AccountNumber)
		if errors.Is(e, sql.ErrNoRows) {
			return &httpError{status: 409, msg: "This payment account is no longer available. Refresh before transferring money."}
		}
		if e != nil {
			return e
		}
		if strings.TrimSpace(bankAccount.HolderName) == "" || strings.TrimSpace(bankAccount.BankName) == "" || strings.TrimSpace(bankAccount.AccountNumber) == "" {
			return &httpError{status: 409, msg: "This payment account is incomplete"}
		}
		snapshot, _ := json.Marshal(bankAccount)
		status = "pending"
		return tx.QueryRow(`INSERT INTO app_account_pass_renewals(user_id,business_id,plan,amount_mmk,bank_account_id,bank_reference,bank_snapshot,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8) RETURNING id`, auth.GetUserID(c), auth.GetBusinessID(c), in.Plan, product.PriceMMK, in.BankAccountID, in.BankReference, string(snapshot), in.IdempotencyKey).Scan(&id)
	})
	if err != nil {
		checkoutError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"id": id, "status": status, "plan": in.Plan, "payment_verified": status == "approved", "message": "Payment reference submitted for platform verification. Access changes only after approval."})
}
