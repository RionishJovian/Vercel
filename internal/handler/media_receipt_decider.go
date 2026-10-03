package handler

import (
	"context"
	"mega-magic/internal/media/bridge"
	"time"
)

type MediaReceiptDecisionStore interface {
	ResolveMediaReview(context.Context, string) (string, error)
	DecideMediaReceipt(context.Context, string, string, bool) (bridge.MediaPass, error)
}

// MediaReceiptDecider shares the existing tenant-owned Telegram update stream.
// It must never be wired to the global/platform billing bot: the supervisor's
// chat, Telegram user and current business-binding guard authorizes each tap.
type MediaReceiptDecider struct{ Store MediaReceiptDecisionStore }

func (d MediaReceiptDecider) ResolveRequestBusiness(id string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.Store.ResolveMediaReview(ctx, id)
}
func (d MediaReceiptDecider) DecidePurchaseRequest(bid, id string, approve bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := d.Store.DecideMediaReceipt(ctx, bid, id, approve)
	return err
}
