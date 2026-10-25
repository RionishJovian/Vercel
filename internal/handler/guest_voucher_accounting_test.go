package handler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type accountingReadFunc func(context.Context, string, string, []VoucherLoginSpec) ([]VoucherLoginState, error)

func (f accountingReadFunc) ReadVoucherLogins(ctx context.Context, biz, device string, refs []VoucherLoginSpec) ([]VoucherLoginState, error) {
	return f(ctx, biz, device, refs)
}

func TestVoucherAccountingPagesBeforeApplying(t *testing.T) {
	refs := make([]VoucherLoginSpec, 401)
	for i := range refs {
		refs[i].VoucherID = fmt.Sprintf("voucher-%d", i)
	}
	var calls []int
	h := GuestVoucherHandler{Syncer: accountingReadFunc(func(ctx context.Context, biz, device string, page []VoucherLoginSpec) ([]VoucherLoginState, error) {
		calls = append(calls, len(page))
		states := make([]VoucherLoginState, len(page))
		for i, ref := range page {
			states[i] = VoucherLoginState{VoucherID: ref.VoucherID, Exists: true}
		}
		return states, nil
	})}
	states, err := h.readGatewayVoucherAccounting(context.Background(), "business", "gateway", refs)
	if err != nil || len(states) != 401 || !reflect.DeepEqual(calls, []int{200, 200, 1}) {
		t.Fatalf("pages=%v records=%d err=%v", calls, len(states), err)
	}
	// An uncertain later page yields no partial snapshot to the ledger caller.
	h.Syncer = accountingReadFunc(func(ctx context.Context, biz, device string, page []VoucherLoginSpec) ([]VoucherLoginState, error) {
		if page[0].VoucherID == "voucher-200" {
			return nil, errors.New("router disconnected")
		}
		states := make([]VoucherLoginState, len(page))
		for i, ref := range page {
			states[i] = VoucherLoginState{VoucherID: ref.VoucherID}
		}
		return states, nil
	})
	if states, err := h.readGatewayVoucherAccounting(context.Background(), "business", "gateway", refs); err == nil || states != nil {
		t.Fatalf("uncertain page returned partial snapshot: %d, %v", len(states), err)
	}
}

func TestVoucherAccountingRejectsUntrustedObservations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []VoucherLoginState
	}{
		{"missing", nil},
		{"foreign", []VoucherLoginState{{VoucherID: "foreign"}}},
		{"duplicate", []VoucherLoginState{{VoucherID: "one"}, {VoucherID: "one"}}},
		{"negative", []VoucherLoginState{{VoucherID: "one", Exists: true, BytesUsed: -1}, {VoucherID: "two"}}},
		{"absentWithUsage", []VoucherLoginState{{VoucherID: "one", UptimeSeconds: 1}, {VoucherID: "two"}}},
		{"multicastIdentity", []VoucherLoginState{{VoucherID: "one", Exists: true, ActiveMAC: "11:22:33:44:55:66"}, {VoucherID: "two"}}},
		{"multicastFirstIdentity", []VoucherLoginState{{VoucherID: "one", Exists: true, FirstUseEpoch: 1792941234, FirstMAC: "11:22:33:44:55:66"}, {VoucherID: "two"}}},
		{"negativeAnchor", []VoucherLoginState{{VoucherID: "one", Exists: true, FirstUseEpoch: -1}, {VoucherID: "two"}}},
		{"identityWithoutAnchor", []VoucherLoginState{{VoucherID: "one", Exists: true, FirstMAC: "02:ab:cd:ef:12:34"}, {VoucherID: "two"}}},
		{"absentWithAnchor", []VoucherLoginState{{VoucherID: "one", FirstUseEpoch: 1792941234}, {VoucherID: "two"}}},
		{"absentWithFirstIdentity", []VoucherLoginState{{VoucherID: "one", FirstMAC: "02:ab:cd:ef:12:34"}, {VoucherID: "two"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := GuestVoucherHandler{Syncer: accountingReadFunc(func(context.Context, string, string, []VoucherLoginSpec) ([]VoucherLoginState, error) {
				return tc.states, nil
			})}
			if states, err := h.readGatewayVoucherAccounting(context.Background(), "business", "gateway", []VoucherLoginSpec{{VoucherID: "one"}, {VoucherID: "two"}}); err == nil || states != nil {
				t.Fatalf("accepted invalid accounting: %+v %v", states, err)
			}
		})
	}
}
