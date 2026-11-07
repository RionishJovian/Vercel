package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mega-magic/internal/telegram"
	"time"
)

type AccountRenewalDecider struct {
	Handler *PlatformHandler
	ActorID string
}

func (d AccountRenewalDecider) ResolveRequestBusiness(id string) (string, error) {
	if !appPassUUID.MatchString(id) {
		return "", errors.New("Invalid request ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var bid string
	err := d.Handler.db.QueryRowContext(ctx, `SELECT business_id FROM app_account_pass_renewals WHERE id=$1`, id).Scan(&bid)
	return bid, err
}
func (d AccountRenewalDecider) DecidePurchaseRequest(bid, id string, approve bool) error {
	found, err := d.ResolveRequestBusiness(id)
	if err != nil || found != bid {
		return errors.New("Renewal not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// The button text explicitly requires independent bank verification before approval.
	_, err = d.Handler.decideAccountRenewal(ctx, d.ActorID, id, approve, true)
	return err
}
func (h *PlatformHandler) NotifyAccountRenewal(id string, msg telegram.ButtonMessenger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var plan, email string
	var amount sql.NullInt64
	var reference sql.NullString
	err := h.db.QueryRowContext(ctx, `SELECT r.plan,u.email,r.amount_mmk,r.bank_reference FROM app_account_pass_renewals r JOIN users u ON u.user_id=r.user_id AND u.business_id=r.business_id WHERE r.id=$1 AND r.status='pending'`, id).Scan(&plan, &email, &amount, &reference)
	if err != nil {
		return
	}
	text := fmt.Sprintf("Mega Magic app pass request\nOrder: %s\nAccount: %s\nPlan: %s", telegram.OrderReference(id), email, plan)
	if amount.Valid {
		text += fmt.Sprintf("\nAmount: %s\nBank reference: %s\nApprove only after independently checking the payment in your bank account. Approval activates or renews this app pass.", telegram.FormatMMK(int(amount.Int64)), reference.String)
	} else {
		text += "\nNo bank payment evidence attached. Approve only if you intend to grant this pass."
	}
	_ = msg.SendButtons("platform", text, id)
}
