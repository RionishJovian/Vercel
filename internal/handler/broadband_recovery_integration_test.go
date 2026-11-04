package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/broadband"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type bbTestRouter struct {
	state     broadband.State
	operation string
	writes    int
	loseReply bool
}

func (r *bbTestRouter) ReadQuickConfig(context.Context, string, string) (routeros.QuickConfig, error) {
	return routeros.QuickConfig{}, nil
}
func (r *bbTestRouter) ApplyQuickConfig(context.Context, string, string, routeros.QuickConfig) (routeros.QuickConfigResult, error) {
	return routeros.QuickConfigResult{}, nil
}
func (r *bbTestRouter) ReadBroadband(context.Context, string, string, string, string) (broadband.Observation, error) {
	return broadband.Observation{State: r.state, OperationID: r.operation, ObservedAt: time.Now().Unix()}, nil
}
func (r *bbTestRouter) ApplyBroadband(ctx context.Context, b, d string, s broadband.Spec) (broadband.Observation, error) {
	if r.operation != s.OperationID {
		r.writes++
		if s.Action == "create" {
			r.state = broadband.State{PeriodSeconds: s.PeriodSeconds}
		} else if s.Action == "renew" {
			if e := r.state.Renew(time.Now().Unix(), s.PeriodSeconds); e != nil {
				return broadband.Observation{}, e
			}
		}
		r.operation = s.OperationID
	}
	if r.loseReply {
		r.loseReply = false
		return broadband.Observation{}, errors.New("connection lost after write")
	}
	return r.ReadBroadband(ctx, b, d, s.AccountID, s.Username)
}
func TestBroadbandRecoversLostReplyWithoutDuplicatePayment(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	source, e := os.ReadFile("../../db/migrations/20261104000000_broadband.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(source)); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	h := NewSecurityHandler(db, database.NewDeviceRepository(db))
	box, e := secretbox.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if e != nil {
		t.Fatal(e)
	}
	h.EnableBroadbandSecrets(box)
	router := &bbTestRouter{loseReply: true}
	h.quickConfig = router
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(auth.ContextKeyBusinessID, biz); c.Set(auth.ContextKeyUserRole, "OWNER") })
	r.POST("/routers/:id/broadband/:command", h.Broadband)
	r.POST("/routers/:id/broadband/operations/:operationId/:command", h.Broadband)
	op := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	call := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	w := call("/routers/"+device+"/broadband/accounts", `{"operation_id":"`+op+`","username":"home001","password":"test-password-only","profile":"default","period_days":30,"amount_mmk":15000,"confirm":true}`)
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "test-password-only") {
		t.Fatal("password leaked")
	}
	if _, e = h.SyncBroadbandRouter(context.Background(), biz, device); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		w = call("/routers/"+device+"/broadband/operations/"+op+"/retry", `{"confirm":true}`)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if router.writes != 1 {
		t.Fatal("recovery repeated installation", router.writes)
	}
	w = call("/routers/ffffffff-ffff-4fff-8fff-ffffffffffff/broadband/accounts", `{"operation_id":"ffffffff-ffff-4fff-8fff-ffffffffffff","username":"other","password":"test-password-only","profile":"default","period_days":30,"confirm":true}`)
	if w.Code != 404 || router.writes != 1 {
		t.Fatal("unknown/foreign router reached provisioning", w.Code, router.writes)
	}
	if e = withTenant(db, biz, func(tx *sql.Tx) error {
		var n, total int
		e := tx.QueryRow(`SELECT count(*),sum(amount_mmk) FROM broadband_operations WHERE business_id=$1 AND state='applied'`, biz).Scan(&n, &total)
		if n != 1 || total != 15000 {
			t.Fatal(n, total)
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	// A delayed read of the create must not overwrite a later suspension's
	// traffic sample, even when the delayed response has a newer timestamp.
	if e = withTenant(db, biz, func(tx *sql.Tx) error {
		repo := database.BroadbandRepository{}
		later := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		if e := repo.ReserveTXN(tx, biz, device, database.BroadbandOperation{ID: later, AccountID: op, Kind: "suspend", RequestHash: "later", Payload: "sealed"}); e != nil {
			return e
		}
		s := router.state
		s.Suspended = true
		at := time.Now().Unix() + 1
		if e := recordBroadbandObservation(tx, biz, device, op, broadband.Observation{State: s, OperationID: later, ObservedAt: at, UsageAvailable: true}); e != nil {
			return e
		}
		if e := recordBroadbandObservation(tx, biz, device, op, broadband.Observation{State: router.state, OperationID: router.operation, ObservedAt: at + 1, UsageAvailable: true, Session: &broadband.SessionUsage{ID: "stale", DownloadBytes: 123}}); e != nil {
			return e
		}
		var empty bool
		if e := tx.QueryRow(`SELECT session IS NULL FROM broadband_accounts WHERE business_id=$1 AND id=$2`, biz, op).Scan(&empty); e != nil {
			return e
		}
		if !empty {
			t.Fatal("stale operation replaced the current session")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
