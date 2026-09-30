package database

import (
	"testing"

	"mega-magic/internal/model"
)

func TestPortalAccessMethods(t *testing.T) {
	tests := []struct {
		name              string
		cfg               model.PortalConfig
		wantVoucher       bool
		wantMobilePayment bool
	}{
		{name: "legacy zero value enables both", wantVoucher: true, wantMobilePayment: true},
		{name: "voucher only", cfg: model.PortalConfig{VoucherLoginEnabled: true}, wantVoucher: true},
		{name: "payment only", cfg: model.PortalConfig{MobilePaymentEnabled: true}, wantMobilePayment: true},
		{name: "both", cfg: model.PortalConfig{VoucherLoginEnabled: true, MobilePaymentEnabled: true}, wantVoucher: true, wantMobilePayment: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voucher, payment := portalAccessMethods(&tt.cfg)
			if voucher != tt.wantVoucher || payment != tt.wantMobilePayment {
				t.Fatalf("got voucher=%v payment=%v; want voucher=%v payment=%v", voucher, payment, tt.wantVoucher, tt.wantMobilePayment)
			}
		})
	}
}
