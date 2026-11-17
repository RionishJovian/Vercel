package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
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

// NotifyAccountRenewal alerts the platform operator once. A failure is logged and left
// for RunRenewalNotifier to retry; it is never silent.
func (h *PlatformHandler) NotifyAccountRenewal(id string, msg telegram.ButtonMessenger) {
	if err := h.sendAccountRenewal(id, msg); err != nil {
		log.Printf("app pass request %s: platform Telegram alert not delivered: %v", id, err)
	}
}

// RunRenewalNotifier retries alerts that were never delivered (receiver still starting after a
// restart, Telegram down, no platform binding yet). Requests older than a week are left to the console.
func (h *PlatformHandler) RunRenewalNotifier(ctx context.Context, msg telegram.ButtonMessenger) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		rows, err := h.db.QueryContext(qctx, `SELECT id FROM app_account_pass_renewals WHERE status='pending' AND notified_at IS NULL AND created_at < now()-interval '30 seconds' AND created_at > now()-interval '7 days' ORDER BY created_at LIMIT 20`)
		var ids []string
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
		} else {
			log.Printf("app pass alert retry: list failed: %v", err)
		}
		cancel()
		for _, id := range ids {
			h.NotifyAccountRenewal(id, msg)
		}
	}
}

func (h *PlatformHandler) sendAccountRenewal(id string, msg telegram.ButtonMessenger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var plan, email string
	var amount sql.NullInt64
	var reference sql.NullString
	err := h.db.QueryRowContext(ctx, `SELECT r.plan,u.email,r.amount_mmk,r.bank_reference FROM app_account_pass_renewals r JOIN users u ON u.user_id=r.user_id AND u.business_id=r.business_id WHERE r.id=$1 AND r.status='pending'`, id).Scan(&plan, &email, &amount, &reference)
	if err != nil {
		return fmt.Errorf("request lookup: %w", err)
	}
	text := fmt.Sprintf("Mega Magic app pass request\nOrder: %s\nAccount: %s\nPlan: %s", telegram.OrderReference(id), email, plan)
	if amount.Valid {
		text += fmt.Sprintf("\nAmount: %s\nBank reference: %s\nApprove only after independently checking the payment in your bank account. Approval activates or renews this app pass.", telegram.FormatMMK(int(amount.Int64)), reference.String)
	} else {
		text += "\nNo bank payment evidence attached. Approve only if you intend to grant this pass."
	}
	if err := msg.SendButtons("platform", text, id); err != nil {
		return err
	}
	_, err = h.db.ExecContext(ctx, `UPDATE app_account_pass_renewals SET notified_at=now() WHERE id=$1`, id)
	return err
}
