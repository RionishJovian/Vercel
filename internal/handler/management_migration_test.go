package handler

import (
	"context"
	"database/sql"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/managementmigration"
	"mega-magic/internal/model"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestManagementMigrationJournalAndAtomicTarget(t *testing.T) {
	db := harness_testdb(t)
	if e := database.CheckManagementMigrationSchema(db); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	biz, foreign := insertBusiness(db), insertBusiness(db)
	var user, other, device, id, second string
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture','OWNER')RETURNING user_id`, biz, randHex(6)+"@example.test").Scan(&user); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Other','fixture','OWNER')RETURNING user_id`, foreign, randHex(6)+"@example.test").Scan(&other); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,transport,management_ip,ssh_username,ssh_secret_ciphertext,ssh_secret_kind,ssh_host_key_fingerprint)VALUES($1,'Disposable metadata fixture','tunnel','10.77.0.20','fixture','ciphertext-fixture','password','SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA')RETURNING device_id`, biz).Scan(&device); e != nil {
		t.Fatal(e)
	}
	if e := db.QueryRow(`SELECT gen_random_uuid(),gen_random_uuid()`).Scan(&id, &second); e != nil {
		t.Fatal(e)
	}
	store := database.ManagementMigrationStore{DB: db}
	j := &managementmigration.Job{ID: id, BusinessID: biz, DeviceID: device, RequestedBy: user, OldAddress: "10.77.0.20", NewAddress: "10.77.0.21", Interface: "fixture", OldRecord: "*A", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", RouterKey: "fixture-key", Phase: "planned", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
	if e := store.Create(ctx, j); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Load(ctx, foreign, device, id); e != sql.ErrNoRows {
		t.Fatalf("foreign read %v", e)
	}
	if _, e := db.Exec(`DELETE FROM devices WHERE device_id=$1`, device); e == nil {
		t.Fatal("active migration record deleted")
	}
	clone := *j
	clone.ID = second
	if e := store.Create(ctx, &clone); e == nil {
		t.Fatal("second active job accepted")
	}
	if _, e := db.Exec(`UPDATE devices SET management_ip='10.77.0.99' WHERE device_id=$1`, device); e == nil {
		t.Fatal("unguarded tunnel address update accepted")
	}
	if _, e := db.Exec(`UPDATE devices SET transport='direct' WHERE device_id=$1`, device); e == nil {
		t.Fatal("transport downgrade bypass accepted")
	}
	if e := store.SwitchConnection(ctx, j); e == nil {
		t.Fatal("premature target switch")
	}
	for _, phase := range []string{"starting", "hub_staged", "router_staged", "new_verified", "hub_committed", "router_retained"} {
		previous := j.Phase
		j.Phase = phase
		if e := store.Save(ctx, j, previous); e != nil {
			t.Fatal(e)
		}
	}
	stale := *j
	stale.Revision--
	if e := store.SwitchConnection(ctx, &stale); e == nil {
		t.Fatal("stale switch accepted")
	}
	if e := store.SwitchConnection(ctx, j); e != nil {
		t.Fatal(e)
	}
	fresh, e := store.Load(ctx, biz, device, id)
	if e != nil || fresh.Phase != "connection_saved" {
		t.Fatalf("%+v %v", fresh, e)
	}
	var address string
	var events int
	_ = db.QueryRow(`SELECT host(management_ip)FROM devices WHERE device_id=$1`, device).Scan(&address)
	_ = db.QueryRow(`SELECT count(*)FROM device_change_events WHERE device_id=$1 AND action='router.management_address.migrated'`, device).Scan(&events)
	if address != "10.77.0.21" || events != 1 {
		t.Fatalf("target %s audit %d", address, events)
	}
	if e := store.SwitchConnection(ctx, j); e == nil {
		t.Fatal("target switch replayed")
	}
	// The per-device lease excludes another process and releases normally.
	release, e := store.Lease(ctx, biz, device)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Lease(ctx, biz, device); e == nil {
		t.Fatal("concurrent lease accepted")
	}
	release()
	release, e = store.Lease(ctx, biz, device)
	if e != nil {
		t.Fatal(e)
	}
	release()
	// Composite FK rejects a job actor from another tenant, even with a valid ID.
	j.Phase = "completed"
	if e = store.Save(ctx, j, "connection_saved"); e != nil {
		t.Fatal(e)
	}
	clone = *j
	clone.ID = second
	clone.Phase = "planned"
	clone.OldAddress = "10.77.0.21"
	clone.NewAddress = "10.77.0.22"
	clone.RequestedBy = other
	if e = store.Create(ctx, &clone); e == nil {
		t.Fatal("foreign actor accepted")
	}
	if _, e := db.Exec(`DELETE FROM devices WHERE device_id=$1`, device); e != nil {
		t.Fatal("completed migration permanently blocked record deletion", e)
	}
}
func TestManagementMigrationDisabledAndConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MM_MANAGEMENT_MIGRATION_ENABLED", "")
	h := NewSecurityHandler(nil, nil)
	r := gin.New()
	r.GET("/routers/:id/management-migration", h.ManagementMigration)
	r.POST("/routers/:id/management-migration/:action", h.ManagementMigration)
	for _, method := range []string{"GET", "POST"} {
		path := "/routers/test/management-migration"
		if method == "POST" {
			path += "/apply"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{"confirmed":true}`)))
		want := 200
		if method == "POST" {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}
func TestRouterRecordCannotBypassManagementMigration(t *testing.T) {
	db := harness_testdb(t)
	biz := insertBusiness(db)
	var device string
	if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,transport,management_ip)VALUES($1,'Fixture','tunnel','10.77.0.40')RETURNING device_id`, biz).Scan(&device); e != nil {
		t.Fatal(e)
	}
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("business_id", biz); c.Next() })
	r.PUT("/routers/:id", h.UpdateRouter)
	w := httptest.NewRecorder()
	q := httptest.NewRequest("PUT", "/routers/"+device, strings.NewReader(`{"management_ip":"10.77.0.41","transport":"direct"}`))
	q.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, q)
	if w.Code != 409 {
		t.Fatalf("unguarded rewrite %d %s", w.Code, w.Body)
	}
}

type fixtureMoveController struct {
	calls int
	job   *managementmigration.Job
}

func (f *fixtureMoveController) Status(context.Context, string, string) (*managementmigration.Job, error) {
	return f.job, nil
}
func (f *fixtureMoveController) Preview(context.Context, string, string, string) (*managementmigration.Job, error) {
	f.calls++
	return f.job, nil
}
func (f *fixtureMoveController) Run(context.Context, string, string, string, string, string, int) (*managementmigration.Job, error) {
	f.calls++
	return f.job, nil
}
func TestManagementMigrationRouteRoleTenantAndConfirmation(t *testing.T) {
	db := harness_testdb(t)
	biz, foreign := insertBusiness(db), insertBusiness(db)
	var device string
	if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name)VALUES($1,'Disposable route fixture')RETURNING device_id`, biz).Scan(&device); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MM_MANAGEMENT_MIGRATION_ENABLED", "true")
	h := NewSecurityHandler(db, database.NewDeviceRepository(db))
	controller := &fixtureMoveController{job: &managementmigration.Job{ID: "11111111-1111-4111-8111-111111111111", Phase: "planned"}}
	h.ConfigureManagementMigrations(controller)
	service, _ := auth.NewService("fixture-management-migration-test-secret", time.Hour, 24*time.Hour, 4)
	router := gin.New()
	router.Use(auth.Middleware(service))
	RegisterManagementMigrationRoutes(router.Group("/routers"), h)
	request := func(role, business, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		q := httptest.NewRequest("POST", "/routers/"+device+"/management-migration/apply", strings.NewReader(body))
		q.Header.Set("Content-Type", "application/json")
		if role != "" {
			token, e := service.GenerateToken(&model.User{UserID: "11111111-1111-4111-8111-111111111111", BusinessID: business, UserRole: model.UserRole(role)})
			if e != nil {
				t.Fatal(e)
			}
			q.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(w, q)
		return w
	}
	valid := `{"id":"11111111-1111-4111-8111-111111111111","revision":0,"confirmed":true}`
	for _, tc := range []struct {
		role, biz, body string
		code            int
	}{{"", biz, valid, 401}, {"VIEWER", biz, valid, 403}, {"ADMIN", foreign, valid, 404}, {"OWNER", biz, `{"confirmed":false}`, 400}} {
		w := request(tc.role, tc.biz, tc.body)
		if w.Code != tc.code {
			t.Fatalf("%s %d %s", tc.role, w.Code, w.Body)
		}
	}
	if controller.calls != 0 {
		t.Fatal("denied request invoked migration")
	}
	w := request("OWNER", biz, valid)
	if w.Code != 200 || controller.calls != 1 {
		t.Fatalf("positive control %d %s", w.Code, w.Body)
	}
}

func TestManagementMigrationSchemaHold(t *testing.T) {
	db := harness_testdb(t)
	if _, e := db.Exec(`ALTER TABLE devices DISABLE TRIGGER guard_hub_management_address_move`); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = db.Exec(`ALTER TABLE devices ENABLE TRIGGER guard_hub_management_address_move`) })
	if e := database.CheckManagementMigrationSchema(db); e == nil {
		t.Fatal("disabled address guard accepted")
	}
	if _, e := db.Exec(`ALTER TABLE devices ENABLE TRIGGER guard_hub_management_address_move`); e != nil {
		t.Fatal(e)
	}
	if e := database.CheckManagementMigrationSchema(db); e != nil {
		t.Fatal(e)
	}
}

func TestManagementMigrationGuardAllowsFirstAuthentication(t *testing.T) {
	db := harness_testdb(t)
	biz := insertBusiness(db)
	var device string
	if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,transport,management_ip)VALUES($1,'Unverified metadata fixture','tunnel','10.77.0.50')RETURNING device_id`, biz).Scan(&device); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`UPDATE devices SET management_ip='10.77.0.51',ssh_username='fixture',ssh_secret_kind='password',ssh_secret_ciphertext='test-cipher',ssh_host_key_fingerprint='SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' WHERE device_id=$1`, device); e != nil {
		t.Fatal("initial verified authentication was blocked", e)
	}
	if _, e := db.Exec(`UPDATE devices SET management_ip='10.77.0.52' WHERE device_id=$1`, device); e == nil {
		t.Fatal("configured connection rewrite accepted")
	}
}
