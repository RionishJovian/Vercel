package handler

import (
	"context"
	"fmt"
	"net"

	"mega-magic/internal/database"
)

const voucherAccountingPageSize = 200

// Read only explicit guest grants, not unrelated business-pass vouchers that
// happen to have a device and a duration. Never hold this transaction during I/O.
func (h *GuestVoucherHandler) gatewayVoucherLoginSpecs(ctx context.Context, biz, deviceID string) ([]VoucherLoginSpec, error) {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var specs []VoucherLoginSpec
	err = database.WithTenant(tx, biz, func() error {
		rows, err := tx.QueryContext(ctx, `SELECT v.id::text,v.code,g.minutes,g.data_mb
 FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id AND g.business_id=v.business_id
 WHERE g.business_id=$1 AND g.device_id=$2 AND v.device_id=$2 ORDER BY v.id`, biz, deviceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var spec VoucherLoginSpec
			if err := rows.Scan(&spec.VoucherID, &spec.Code, &spec.Minutes, &spec.DataMB); err != nil {
				return err
			}
			specs = append(specs, spec)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return specs, nil
}

// Validate every bounded page before applying any ledger change. A failed,
// truncated or cross-voucher response cannot leave a partially updated ledger.
func (h *GuestVoucherHandler) readGatewayVoucherAccounting(ctx context.Context, biz, deviceID string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
	states := make([]VoucherLoginState, 0, len(specs))
	for start := 0; start < len(specs); start += voucherAccountingPageSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := start + voucherAccountingPageSize
		if end > len(specs) {
			end = len(specs)
		}
		page := specs[start:end]
		observed, err := h.Syncer.ReadVoucherLogins(ctx, biz, deviceID, page)
		if err != nil {
			return nil, err
		}
		if len(observed) != len(page) {
			return nil, fmt.Errorf("incomplete voucher accounting page")
		}
		expected := make(map[string]bool, len(page))
		for _, spec := range page {
			expected[spec.VoucherID] = true
		}
		for _, state := range observed {
			if !expected[state.VoucherID] {
				return nil, fmt.Errorf("unexpected or repeated voucher accounting identity")
			}
			delete(expected, state.VoucherID)
			if state.UptimeSeconds < 0 || state.BytesUsed < 0 || state.FirstUseEpoch < 0 {
				return nil, fmt.Errorf("negative voucher accounting counter")
			}
			if !state.Exists && (state.Disabled || state.UptimeSeconds != 0 || state.BytesUsed != 0 || state.ActiveMAC != "" || state.FirstUseEpoch != 0 || state.FirstMAC != "") {
				return nil, fmt.Errorf("absent voucher has inconsistent accounting")
			}
			for _, identity := range []string{state.ActiveMAC, state.FirstMAC} {
				if identity == "" {
					continue
				}
				mac, err := net.ParseMAC(identity)
				if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
					return nil, fmt.Errorf("invalid voucher session identity")
				}
			}
			if state.FirstMAC != "" && state.FirstUseEpoch == 0 {
				return nil, fmt.Errorf("voucher first-use identity without an anchor")
			}
		}
		states = append(states, observed...)
	}
	return states, nil
}
