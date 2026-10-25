package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

type voucherReportFixture struct {
	RecordedCount         int64  `json:"recorded_count"`
	RecordedFaceValue     int64  `json:"recorded_face_value"`
	FirstUseCount         int64  `json:"first_use_count"`
	AvailableCount        int64  `json:"available_count"`
	ReservedCount         int64  `json:"reserved_count"`
	ActivatedCount        int64  `json:"activated_count"`
	ExpiredCount          int64  `json:"expired_count"`
	RevokedCount          int64  `json:"revoked_count"`
	OtherCount            int64  `json:"other_count"`
	ActivationCount       int64  `json:"activation_count"`
	ActivationValue       int64  `json:"activation_value"`
	ActivationToday       int64  `json:"activation_today"`
	ActivationLast7       int64  `json:"activation_last7"`
	ActivationLast30      int64  `json:"activation_last30"`
	ActivationTimeUnknown int64  `json:"activation_time_unknown_count"`
	CashSalesRecorded     bool   `json:"cash_sales_recorded"`
	ConfirmedSales        *int64 `json:"confirmed_sales_count"`
	CashRevenue           *int64 `json:"cash_revenue_mmk"`
	Gateways              []struct {
		DeviceID      string `json:"device_id"`
		RecordedCount int64  `json:"recorded_count"`
	} `json:"gateways"`
}

func callVoucherReport(h *VoucherHandler, businessID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/voucher-report?business_id=ignored", nil)
	c.Set(auth.ContextKeyBusinessID, businessID)
	h.VoucherReport(c)
	return w
}

func TestVoucherReportFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		businessID string
		status     int
	}{{"", 401}, {"11111111-1111-4111-8111-111111111111", 503}} {
		w := callVoucherReport(&VoucherHandler{}, tc.businessID)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestDBVoucherReportFullInventoryAndTenantIsolation(t *testing.T) {
	db := harness_testdb(t)
	migration, err := os.ReadFile("../../migrations/20261001200000_guest_voucher_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	activationMigration, err := os.ReadFile("../../migrations/20261021090000_guest_voucher_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(activationMigration)); err != nil {
		t.Fatal(err)
	}
	biz, foreign, empty := insertBusiness(db), insertBusiness(db), insertBusiness(db)
	device := func(businessID, name string) string {
		var id string
		if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name) VALUES($1,$2) RETURNING device_id`, businessID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	primary, branch, otherDevice := device(biz, "Cafe <gateway>"), device(biz, "Branch"), device(foreign, "FOREIGN_PRIVATE_NAME")
	if _, err := db.Exec(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk,status)
 SELECT $1,$2,'guest-internet','Daily','FULL-'||i,500,'unused' FROM generate_series(1,130) i`, biz, primary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb)
 SELECT id,business_id,device_id,120,1024 FROM vouchers WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	seed := func(businessID, deviceID, code, plan, status, state string, started, expired bool, price int) {
		t.Helper()
		var id string
		if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk,status)
 VALUES($1,$2,$3,'Daily',$4,$5,$6) RETURNING id`, businessID, deviceID, plan, code, price, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb,state,client_mac,first_used_at,expires_at,lease_token,lease_until)
 VALUES($1,$2,$3,120,1024,$4,CASE WHEN $5 THEN '02:00:00:00:00:01'::macaddr END,
 CASE WHEN $5 THEN now() - CASE WHEN $6 THEN interval '1 day' ELSE interval '0' END END,
 CASE WHEN $5 THEN now() - CASE WHEN $6 THEN interval '1 day' ELSE interval '0' END + interval '120 minutes' END,
 CASE WHEN $4='provisioning' THEN 'fixture-lease' END,CASE WHEN $4='provisioning' THEN now()+interval '2 minutes' END)`, id, businessID, deviceID, state, started, expired); err != nil {
			t.Fatal(err)
		}
	}
	seed(biz, primary, "PENDING", "guest-internet", "used", "provisioning", true, false, 1000)
	seed(biz, primary, "ACTIVE", "guest-internet", "used", "active", true, false, 1000)
	seed(biz, primary, "ELAPSED", "guest-internet", "used", "active", true, true, 1000)
	seed(biz, primary, "REVOKED", "guest-internet", "revoked", "unused", false, false, 0)
	seed(biz, primary, "INCONSISTENT", "guest-internet", "used", "unused", false, false, 0)
	seed(biz, branch, "FREE", "guest-internet", "unused", "unused", false, false, 0)
	// A real successful finalization records today's activation once.
	seed(biz, primary, "TRACKED", "guest-internet", "unused", "unused", false, false, 1500)
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		repo := database.GuestVoucherRepository{}
		claim, err := repo.ReserveTXN(tx, biz, primary, "TRACKED", "02:00:00:00:00:03")
		if err != nil {
			return err
		}
		ok, err := repo.FinalizeTXN(tx, biz, claim.VoucherID, claim.LeaseToken)
		if err == nil && !ok {
			return fmt.Errorf("finalize failed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Synthetic historical records exercise Yangon calendar windows, including
	// expired activations whose recorded value must remain in all-time totals.
	for _, period := range []struct{ days, price int }{{1, 2000}, {40, 3000}} {
		var id string
		if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk,status) VALUES($1,$2,'guest-internet','History',$3,$4,'used') RETURNING id`, biz, primary, fmt.Sprintf("HISTORY-%d", period.days), period.price).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb,state,client_mac,first_used_at,expires_at,activated_at)
 SELECT $1,$2,$3,120,1024,'expired','02:00:00:00:00:04',stamp-interval '1 minute',stamp+interval '119 minutes',stamp
 FROM (SELECT (((now() AT TIME ZONE 'Asia/Yangon')::date-$4::integer)+time '12:00') AT TIME ZONE 'Asia/Yangon' AS stamp) history`, id, biz, primary, period.days); err != nil {
			t.Fatal(err)
		}
	}
	seed(foreign, otherDevice, "FOREIGN_PRIVATE_CODE", "guest-internet", "unused", "unused", false, false, 9000000)
	seed(biz, primary, "BUSINESS_PASS", "business-pass", "unused", "unused", false, false, 99999)
	if _, err := db.Exec(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk,status) VALUES($1,$2,'guest-internet','No grant','NO_GRANT',99999,'unused')`, biz, primary); err != nil {
		t.Fatal(err)
	}
	h := &VoucherHandler{db: db}
	w := callVoucherReport(h, biz)
	if w.Code != 200 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	var result voucherReportFixture
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RecordedCount != 139 || result.RecordedFaceValue != 74500 || result.FirstUseCount != 6 || result.AvailableCount != 131 || result.ReservedCount != 1 || result.ActivatedCount != 2 || result.ExpiredCount != 3 || result.RevokedCount != 1 || result.OtherCount != 1 {
		t.Fatalf("wrong full-inventory totals: %+v", result)
	}
	if result.ActivationCount != 3 || result.ActivationValue != 6500 || result.ActivationToday != 1500 || result.ActivationLast7 != 3500 || result.ActivationLast30 != 3500 || result.ActivationTimeUnknown != 3 {
		t.Fatalf("wrong activation value/calendar totals: %+v", result)
	}
	if result.CashSalesRecorded || result.CashRevenue != nil || result.ConfirmedSales != nil {
		t.Fatalf("issuance/activation cannot fabricate cash: %+v", result)
	}
	if len(result.Gateways) != 2 {
		t.Fatalf("gateway grouping: %+v", result.Gateways)
	}
	if strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), "FULL-") || strings.Contains(w.Body.String(), "02:00") {
		t.Fatal("report leaked another tenant, code or client MAC")
	}
	for _, g := range result.Gateways {
		if g.DeviceID == branch && g.RecordedCount != 1 || g.DeviceID == primary && g.RecordedCount != 138 {
			t.Fatal("incorrect gateway group")
		}
	}
	w = callVoucherReport(h, empty)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RecordedCount != 0 || result.RecordedFaceValue != 0 || len(result.Gateways) != 0 || result.CashRevenue != nil {
		t.Fatalf("empty tenant result: %+v", result)
	}
	// The authenticated role itself must also prevent reading the other tenant,
	// even when a caller deliberately supplies its ID as the SQL parameter.
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		var raw []byte
		if err := tx.QueryRow(voucherReportSQL, foreign).Scan(&raw); err != nil {
			return err
		}
		var other voucherReportFixture
		if err := json.Unmarshal(raw, &other); err != nil {
			return err
		}
		if other.RecordedCount != 0 {
			return fmt.Errorf("RLS leaked %d vouchers", other.RecordedCount)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
