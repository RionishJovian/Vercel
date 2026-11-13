package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/lib/pq"
)

// Gateway capacity against the REAL production schema: a throwaway database is
// built with deploy/server/mm-migrate (db/baseline.sql + db/migrations), so the
// devices trigger, the grant ledger, RLS and the runtime role are the shipped
// ones. Opt-in with MM_TEST_DB_URL (an admin URL to a disposable server; needs
// psql on PATH and the roles authenticated/anon/service_role). Never point it at
// production.
func gatewaySchemaDB(t *testing.T) *DB {
	t.Helper()
	base := os.Getenv("MM_TEST_DB_URL")
	if base == "" {
		t.Skip("set MM_TEST_DB_URL to a disposable PostgreSQL admin URL to run the gateway capacity schema tests")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql is required to build the schema with mm-migrate")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := "mm_gateway_" + randHex(6)
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Skipf("cannot create a throwaway database: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`) })
	u.Path = "/" + name
	for _, step := range []string{"init", "up"} {
		cmd := exec.Command("bash", "../../deploy/server/mm-migrate", step)
		cmd.Env = append(os.Environ(), "MM_DB_URL="+u.String())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("mm-migrate %s: %v\n%s", step, err, out)
		}
	}
	raw, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return &DB{raw}
}

type gatewayFixture struct {
	t  *testing.T
	db *DB
}

func (f gatewayFixture) business() (biz, user, site string) {
	f.t.Helper()
	biz = insertBusiness(f.t, f.db)
	user = insertUser(f.t, f.db, biz)
	if err := f.db.QueryRow(`INSERT INTO sites(business_id,name) VALUES($1,'Shop') RETURNING site_id`, biz).Scan(&site); err != nil {
		f.t.Fatal(err)
	}
	return
}

func (f gatewayFixture) gateway(biz, site, name string) (string, error) {
	var id string
	err := f.db.QueryRow(`INSERT INTO devices(business_id,hardware_name,deployment_layer,site_id) VALUES($1,$2,'MAIN_GATEWAY',$3) RETURNING device_id`, biz, name, site).Scan(&id)
	return id, err
}

func (f gatewayFixture) purchase(biz, user, kind string) string {
	f.t.Helper()
	var addon, req string
	key := "slot_" + randHex(4)
	if err := f.db.QueryRow(`INSERT INTO store_addons(addon_key,kind,name_en,name_my,price_mmk,period) VALUES($1,$2,'Extra gateway','Gateway',30000,'monthly') RETURNING id`, key, kind).Scan(&addon); err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.QueryRow(`INSERT INTO store_addon_requests(business_id,user_id,addon_id,price_mmk,period,bank_reference,idempotency_key) VALUES($1,$2,$3,30000,'monthly','KBZ-1234','idem-'||$4) RETURNING id`, biz, user, addon, randHex(6)).Scan(&req); err != nil {
		f.t.Fatal(err)
	}
	return req
}

func (f gatewayFixture) capacity(biz string) *GatewayCapacity {
	f.t.Helper()
	tx, err := f.db.Begin()
	if err != nil {
		f.t.Fatal(err)
	}
	defer tx.Rollback()
	var out *GatewayCapacity
	if err := WithTenant(tx, biz, func() error { var e error; out, e = GatewayCapacityTXN(tx, biz); return e }); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f gatewayFixture) fulfill(req string) (string, bool, error) {
	tx, err := f.db.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	id, created, err := FulfillRouterSlotTXN(tx, req, "")
	if err != nil {
		return "", false, err
	}
	return id, created, tx.Commit()
}

func isConstraint(err error, name string) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Constraint == name
}

func TestGatewayCapacitySchema(t *testing.T) {
	db := gatewaySchemaDB(t)
	f := gatewayFixture{t, db}

	t.Run("included gateway is the baseline and the second is refused", func(t *testing.T) {
		biz, _, site := f.business()
		if _, err := f.gateway(biz, site, "A"); err != nil {
			t.Fatal(err)
		}
		_, err := f.gateway(biz, site, "B")
		if !isConstraint(err, "app_business_gateway_capacity") {
			t.Fatalf("second gateway without a slot: got %v", err)
		}
		c := f.capacity(biz)
		if c.Capacity != 1 || c.Used != 1 || c.Available != 0 || c.Over {
			t.Fatalf("capacity %+v", c)
		}
	})

	t.Run("pending and rejected purchases add nothing; approval is fulfilled exactly once", func(t *testing.T) {
		biz, user, site := f.business()
		if _, err := f.gateway(biz, site, "A"); err != nil {
			t.Fatal(err)
		}
		pending := f.purchase(biz, user, "router_slot")
		if _, _, err := f.fulfill(pending); !errors.Is(err, ErrPurchaseNotFulfillable) {
			t.Fatalf("pending purchase fulfilled: %v", err)
		}
		// A grant cannot be forged for an unapproved purchase even with direct SQL.
		if _, err := db.Exec(`INSERT INTO gateway_capacity_grants(business_id,units,source,store_request_id) VALUES($1,1,'store_purchase',$2)`, biz, pending); !isConstraint(err, "gateway_grant_purchase") {
			t.Fatalf("forged grant: %v", err)
		}
		rejected := f.purchase(biz, user, "router_slot")
		if _, err := db.Exec(`UPDATE store_addon_requests SET status='rejected',decided_at=now() WHERE id=$1`, rejected); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.fulfill(rejected); !errors.Is(err, ErrPurchaseNotFulfillable) {
			t.Fatalf("rejected purchase fulfilled: %v", err)
		}
		if c := f.capacity(biz); c.Capacity != 1 {
			t.Fatalf("capacity grew without approval: %+v", c)
		}
		if _, err := db.Exec(`UPDATE store_addon_requests SET status='approved',decided_at=now() WHERE id=$1`, pending); err != nil {
			t.Fatal(err)
		}
		c := f.capacity(biz)
		if len(c.Purchases) != 2 || c.Purchases[0].Fulfillment == "fulfilled" {
			t.Fatalf("approved purchase reported fulfilled before the grant: %+v", c.Purchases)
		}
		first, created, err := f.fulfill(pending)
		if err != nil || !created {
			t.Fatalf("fulfil: %v %v", created, err)
		}
		again, created, err := f.fulfill(pending)
		if err != nil || created || again != first {
			t.Fatalf("retry must return the same grant: %v %v %v", again, created, err)
		}
		c = f.capacity(biz)
		if c.Capacity != 2 || c.Purchased != 1 || len(c.Grants) != 1 || c.Grants[0].Period == nil || *c.Grants[0].Period != "monthly" || c.Grants[0].TermPolicy != "not_enforced" {
			t.Fatalf("capacity after one fulfilment %+v", c)
		}
		if _, err := f.gateway(biz, site, "B"); err != nil {
			t.Fatalf("gateway B with a paid slot: %v", err)
		}
		if _, err := f.gateway(biz, site, "C"); !isConstraint(err, "app_business_gateway_capacity") {
			t.Fatalf("gateway C beyond paid capacity: %v", err)
		}
		// A server or feature add-on never becomes a gateway slot.
		feature := f.purchase(biz, user, "feature")
		_, _ = db.Exec(`UPDATE store_addon_requests SET status='approved',decided_at=now() WHERE id=$1`, feature)
		if _, _, err := f.fulfill(feature); !errors.Is(err, ErrPurchaseNotFulfillable) {
			t.Fatalf("feature add-on fulfilled as a gateway: %v", err)
		}
	})

	t.Run("concurrent registrations cannot both take the last slot", func(t *testing.T) {
		biz, user, site := f.business()
		if _, err := f.gateway(biz, site, "A"); err != nil {
			t.Fatal(err)
		}
		req := f.purchase(biz, user, "router_slot")
		_, _ = db.Exec(`UPDATE store_addon_requests SET status='approved',decided_at=now() WHERE id=$1`, req)
		if _, _, err := f.fulfill(req); err != nil {
			t.Fatal(err)
		}
		const racers = 8
		var wg sync.WaitGroup
		results := make(chan error, racers)
		start := make(chan struct{})
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				tx, err := db.BeginTx(context.Background(), nil)
				if err != nil {
					results <- err
					return
				}
				defer tx.Rollback()
				if _, err = tx.Exec(`INSERT INTO devices(business_id,hardware_name,deployment_layer,site_id) VALUES($1,'racer','MAIN_GATEWAY',$2)`, biz, site); err != nil {
					results <- err
					return
				}
				results <- tx.Commit()
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		ok, refused := 0, 0
		for err := range results {
			switch {
			case err == nil:
				ok++
			case isConstraint(err, "app_business_gateway_capacity"):
				refused++
			default:
				t.Fatalf("unexpected race error: %v", err)
			}
		}
		if ok != 1 || refused != racers-1 {
			t.Fatalf("last slot: %d succeeded, %d refused", ok, refused)
		}
		if c := f.capacity(biz); c.Used != 2 || c.Capacity != 2 {
			t.Fatalf("after race %+v", c)
		}
	})

	t.Run("legacy router grants still count and are not rewritten", func(t *testing.T) {
		biz, _, site := f.business()
		if _, err := db.Exec(`INSERT INTO device_allowances(business_id,routers) VALUES($1,3)`, biz); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"A", "B", "C"} {
			if _, err := f.gateway(biz, site, n); err != nil {
				t.Fatalf("legacy gateway %s: %v", n, err)
			}
		}
		if _, err := f.gateway(biz, site, "D"); !isConstraint(err, "app_business_gateway_capacity") {
			t.Fatalf("beyond legacy grant: %v", err)
		}
		var routers int
		_ = db.QueryRow(`SELECT routers FROM device_allowances WHERE business_id=$1`, biz).Scan(&routers)
		if c := f.capacity(biz); c.Legacy != 2 || c.Capacity != 3 || routers != 3 {
			t.Fatalf("legacy %+v routers=%d", c, routers)
		}
	})

	t.Run("revoking a grant never removes devices and only blocks new enrollments beyond capacity", func(t *testing.T) {
		biz, _, site := f.business()
		var grant string
		if err := db.QueryRow(`INSERT INTO gateway_capacity_grants(business_id,units,source,reason) VALUES($1,1,'operator_grant','pilot site') RETURNING id`, biz).Scan(&grant); err != nil {
			t.Fatal(err)
		}
		a, _ := f.gateway(biz, site, "A")
		b, err := f.gateway(biz, site, "B")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE gateway_capacity_grants SET units=5 WHERE id=$1`, grant); !isConstraint(err, "gateway_grant_immutable") {
			t.Fatalf("grant edited: %v", err)
		}
		if _, err := db.Exec(`UPDATE gateway_capacity_grants SET revoked_at=now(),revoked_by=gen_random_uuid(),revoke_reason='ended' WHERE id=$1`, grant); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE gateway_capacity_grants SET revoke_reason='again' WHERE id=$1`, grant); !isConstraint(err, "gateway_grant_immutable") {
			t.Fatalf("revoked grant edited: %v", err)
		}
		c := f.capacity(biz)
		if c.Used != 2 || c.Capacity != 1 || !c.Over {
			t.Fatalf("after revocation %+v", c)
		}
		// Ordinary edits of the existing gateway still work (no capacity re-check).
		if _, err := db.Exec(`UPDATE devices SET hardware_name='B renamed' WHERE device_id=$1`, b); err != nil {
			t.Fatalf("rename over-capacity gateway: %v", err)
		}
		tx, _ := db.Begin()
		defer tx.Rollback()
		okA, errA := GatewayEnrollmentAllowedTXN(tx, biz, a)
		okB, errB := GatewayEnrollmentAllowedTXN(tx, biz, b)
		if errA != nil || errB != nil || !okA || okB {
			t.Fatalf("enrollment A=%v B=%v (%v %v)", okA, okB, errA, errB)
		}
	})

	t.Run("tenants cannot see or use each other's capacity", func(t *testing.T) {
		bizA, userA, siteA := f.business()
		bizB, _, siteB := f.business()
		req := f.purchase(bizA, userA, "router_slot")
		_, _ = db.Exec(`UPDATE store_addon_requests SET status='approved',decided_at=now() WHERE id=$1`, req)
		if _, err := db.Exec(`INSERT INTO gateway_capacity_grants(business_id,units,source,store_request_id) VALUES($1,1,'store_purchase',$2)`, bizB, req); !isConstraint(err, "gateway_grant_purchase") {
			t.Fatalf("B used A's purchase: %v", err)
		}
		if _, _, err := f.fulfill(req); err != nil {
			t.Fatal(err)
		}
		if c := f.capacity(bizB); c.Capacity != 1 || len(c.Grants) != 0 || len(c.Purchases) != 0 {
			t.Fatalf("B sees A's capacity: %+v", c)
		}
		tx, _ := db.Begin()
		defer tx.Rollback()
		var visible int
		if err := WithTenant(tx, bizB, func() error {
			return tx.QueryRow(`SELECT count(*) FROM gateway_capacity_grants`).Scan(&visible)
		}); err != nil || visible != 0 {
			t.Fatalf("RLS leak: %d %v", visible, err)
		}
		var denied error
		_ = WithTenant(tx, bizB, func() error {
			_, denied = tx.Exec(`INSERT INTO gateway_capacity_grants(business_id,units,source,reason) VALUES($1,1,'operator_grant','self grant')`, bizB)
			return nil
		})
		if denied == nil || !strings.Contains(denied.Error(), "permission denied") {
			t.Fatalf("a tenant granted itself capacity: %v", denied)
		}
		for _, x := range []struct{ biz, site string }{{bizA, siteA}, {bizA, siteA}} {
			if _, err := f.gateway(x.biz, x.site, "gw"); err != nil {
				t.Fatalf("A within its own capacity: %v", err)
			}
		}
		if _, err := f.gateway(bizB, siteB, "B1"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.gateway(bizB, siteB, "B2"); !isConstraint(err, "app_business_gateway_capacity") {
			t.Fatalf("B used A's slot: %v", err)
		}
	})

	t.Run("switches keep the previous rule and do not use gateway slots", func(t *testing.T) {
		biz, _, site := f.business()
		if _, err := f.gateway(biz, site, "A"); err != nil {
			t.Fatal(err)
		}
		// Legacy mode (enforcement off): the database does not limit switches.
		if _, err := db.Exec(`INSERT INTO devices(business_id,hardware_name,deployment_layer) VALUES($1,'sw','DOWNSTREAM_SWITCH')`, biz); err != nil {
			t.Fatalf("switch in legacy mode: %v", err)
		}
		if c := f.capacity(biz); c.Used != 1 {
			t.Fatalf("switch counted as a gateway: %+v", c)
		}
	})

	t.Run("under pass enforcement paid gateways work while switches and sites keep their rule", func(t *testing.T) {
		biz, user, site := f.business()
		if _, err := db.Exec(`INSERT INTO app_account_pass_rollouts(rollout_key,starts_at,transition_days,enforcement_enabled) VALUES('individual-v1',now()-interval '1 day',7,true)`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM app_account_pass_rollouts WHERE rollout_key='individual-v1'`) })
		if _, err := f.gateway(biz, site, "A"); err != nil {
			t.Fatal(err)
		}
		req := f.purchase(biz, user, "router_slot")
		_, _ = db.Exec(`UPDATE store_addon_requests SET status='approved',decided_at=now() WHERE id=$1`, req)
		if _, _, err := f.fulfill(req); err != nil {
			t.Fatal(err)
		}
		if _, err := f.gateway(biz, site, "B"); err != nil {
			t.Fatalf("paid gateway under enforcement: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO devices(business_id,hardware_name,deployment_layer) VALUES($1,'sw','DOWNSTREAM_SWITCH')`, biz); !isConstraint(err, "app_business_router_limit") {
			t.Fatalf("switch rule changed under enforcement: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO sites(business_id,name) VALUES($1,'Second')`, biz); !isConstraint(err, "app_business_site_limit") {
			t.Fatalf("site rule changed under enforcement: %v", err)
		}
	})

	t.Run("platform operator workspaces are exempt", func(t *testing.T) {
		biz, user, site := f.business()
		if _, err := db.Exec(`UPDATE users SET user_role='DEVELOPER' WHERE user_id=$1`, user); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"A", "B", "C"} {
			if _, err := f.gateway(biz, site, n); err != nil {
				t.Fatalf("operator gateway %s: %v", n, err)
			}
		}
		if c := f.capacity(biz); !c.Exempt {
			t.Fatalf("operator not exempt: %+v", c)
		}
	})
}
