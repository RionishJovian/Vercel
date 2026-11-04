package handler

import (
	"context"
	"testing"
	"time"
)

func TestGuestVoucherFirstObservedAfterOutageExpiresInOneSync(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	first := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	h := loginHandler(db)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		states := accountingObserved(specs)
		states[0].FirstUseEpoch = first.Unix()
		states[0].Disabled = true
		return states, nil
	})
	for i := 0; i < 2; i++ {
		if w := callSync(h, biz, device); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		got := accountingSnapshot(t, db, biz, id)
		if got.state != "expired" || !got.activated.Valid || !got.activated.Time.Equal(first) || !got.expiry.Time.Equal(first.Add(time.Hour)) {
			t.Fatalf("late reconciliation shifted or revived voucher: %+v", got)
		}
	}
}
