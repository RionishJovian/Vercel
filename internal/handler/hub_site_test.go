package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

func insertSite(db *database.DB, businessID string) string {
	var id string
	err := db.QueryRow(
		`INSERT INTO sites (business_id, name) VALUES ($1, $2) RETURNING site_id`,
		businessID, "test-site",
	).Scan(&id)
	if err != nil {
		panic("insert site: " + err.Error())
	}
	return id
}

func insertGateway(db *database.DB, businessID, siteID string) string {
	var id string
	err := db.QueryRow(
		`INSERT INTO devices (business_id, site_id, hardware_name, deployment_layer) VALUES ($1, $2, 'gateway', 'MAIN_GATEWAY') RETURNING device_id`,
		businessID, siteID,
	).Scan(&id)
	if err != nil {
		panic("insert gateway: " + err.Error())
	}
	return id
}

func insertGatewayNoSite(db *database.DB, businessID string) string {
	var id string
	err := db.QueryRow(
		`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1, 'gateway-no-site', 'MAIN_GATEWAY') RETURNING device_id`,
		businessID,
	).Scan(&id)
	if err != nil {
		panic("insert gateway: " + err.Error())
	}
	return id
}

// testAuthService creates a minimal auth service for testing with a fixed secret.
func testAuthService() *auth.Service {
	svc, _ := auth.NewService("test-secret-key-for-testing-only-16chars", 24*time.Hour, 24*time.Hour, 4)
	return svc
}

// hubInspectorFunc allows using a function as a hubInspector.
type hubInspectorFunc func(context.Context, hub.Identity) (hub.Peer, error)

func (f hubInspectorFunc) InspectRouter(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
	return f(ctx, identity)
}

// hubEnrollerFunc allows using a function as a hubEnroller.
type hubEnrollerFunc func(context.Context, hub.Identity, string, int) (hub.Peer, error)

func (f hubEnrollerFunc) EnrollPeer(ctx context.Context, id hub.Identity, publicKey string, port int) (hub.Peer, error) {
	return f(ctx, id, publicKey, port)
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// makeTestToken creates a JWT token for testing with the given claims.
func makeTestToken(t *testing.T, userID, businessID, role string) string {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":         "mega-magic",
		"aud":         "mega-magic-api",
		"jti":         "test-jti",
		"sub":         userID,
		"business_id": businessID,
		"type":        "access",
		"email":       "test@example.com",
		"role":        role,
		"iat":         now.Unix(),
		"exp":         now.Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := []byte("test-secret-key-for-testing-only-16chars")
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// setupTestRouter creates a Gin router with the full production middleware chain.
// It returns the router and callback counters for verifying non-invocation on denied requests.
func setupTestRouter(t *testing.T, db *database.DB) (*gin.Engine, *callbackCounters) {
	gin.SetMode(gin.TestMode)

	counters := &callbackCounters{}

	authSvc := testAuthService()
	authHandler := NewAuthHandler(nil, authSvc, nil)

	handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")

	// Configure mock dependencies with call tracking
	handler.ConfigureManagementAuthentication(func(ctx context.Context, biz, id, addr, username, password string, port int, fingerprint string) error {
		counters.managementAuthCalls++
		return nil // Always succeed for testing validation logic
	})
	handler.ConfigureAutomaticManagement(
		func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
			counters.automaticPrepareCalls++
			return script, nil
		},
		func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
			counters.automaticVerifyCalls++
			return nil // Always succeed for testing validation logic
		},
	)
	// Mock enrollment hub for EnrollGateway/QuickEnrollGateway positive controls
	handler.ConfigureHubEnrollment(hubEnrollerFunc(func(ctx context.Context, id hub.Identity, publicKey string, port int) (hub.Peer, error) {
		counters.enrollmentHubCalls++
		return hub.Peer{
			PeerID:            "test-peer",
			TenantID:          id.TenantID,
			RouterID:          id.RouterID,
			RouterPublicKey:   publicKey,
			TunnelAddress:     "10.0.0.1/32",
			HubPublicKey:      "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
			Endpoint:          "10.0.0.1:51820",
			AllowedIPs:        []string{"10.0.0.2/32"},
			State:             "active",
			EnrollmentVersion: 1,
			LastHandshakeAt:   ptr(time.Now()),
		}, nil
	}))
	handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
		counters.liveHubCalls++
		return hub.Peer{
			PeerID:            "test-peer",
			TenantID:          identity.TenantID,
			RouterID:          identity.RouterID,
			RouterPublicKey:   "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
			TunnelAddress:     "10.0.0.1/32",
			HubPublicKey:      "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
			Endpoint:          "10.0.0.1:51820",
			AllowedIPs:        []string{"10.0.0.2/32"},
			State:             "active",
			EnrollmentVersion: 1,
			LastHandshakeAt:   ptr(time.Now()),
		}, nil
	})
	handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
		counters.liveDeviceCalls++
		return &model.Device{DeviceID: id, BusinessID: biz, DeploymentLayer: "MAIN_GATEWAY"}, nil
	}
	handler.livePlan = func(biz string) bool {
		counters.livePlanCalls++
		return true
	}

	r := gin.New()
	r.Use(gin.Recovery())

	tv1 := r.Group("/api/v1")
	{
		// Auth routes (public)
		authGroup := tv1.Group("/auth", auth.CookieOrigin())
		{
			authGroup.POST("/login", authHandler.Login)
		}

		// Protected routes with full middleware chain
		protected := tv1.Group("")
		protected.Use(auth.CookieOrigin(), auth.Middleware(authSvc))
		// Skip RequireActiveTenant in tests (requires DB)
		// protected.Use(auth.RequireActiveTenant(nil))
		protected.Use(auth.RequireMinRole(model.RoleViewer))
		// SubscriptionAccess requires DB, skip for test
		{
			vpsg := protected.Group("/vps")
			RegisterVPSRoutes(vpsg, handler)
		}
	}

	return r, counters
}

// callbackCounters tracks invocations of downstream callbacks for verifying
// that denied requests never reach the handler logic.
type callbackCounters struct {
	managementAuthCalls    int
	automaticPrepareCalls  int
	automaticVerifyCalls   int
	enrollmentHubCalls     int
	liveHubCalls           int
	liveDeviceCalls        int
	livePlanCalls          int
}

func (c *callbackCounters) reset() {
	c.managementAuthCalls = 0
	c.automaticPrepareCalls = 0
	c.automaticVerifyCalls = 0
	c.enrollmentHubCalls = 0
	c.liveHubCalls = 0
	c.liveDeviceCalls = 0
	c.livePlanCalls = 0
}

func TestVPSHandler_GatewaySite(t *testing.T) {
	db := harness_testdb(t)

	bizID := insertBusiness(db)
	siteID := insertSite(db, bizID)

	deviceID := insertGatewayNoSite(db, bizID)
	deviceIDWithSite := insertGateway(db, bizID, siteID)

	router, _ := setupTestRouter(t, db)

	// Admin token
	adminToken := makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin))

	t.Run("GET site - no site assigned", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/site", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			SiteID   *string `json:"site_id"`
			Assigned bool    `json:"assigned"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Assigned {
			t.Fatal("expected assigned=false")
		}
		if resp.SiteID != nil {
			t.Fatal("expected nil site_id")
		}
	})

	t.Run("GET site - site assigned", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceIDWithSite+"/site", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			SiteID   *string `json:"site_id"`
			Assigned bool    `json:"assigned"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if !resp.Assigned {
			t.Fatal("expected assigned=true")
		}
		if resp.SiteID == nil || *resp.SiteID != siteID {
			t.Fatal("expected site_id to match")
		}
	})

	t.Run("PUT site - assign site to gateway", func(t *testing.T) {
		newDeviceID := insertGatewayNoSite(db, bizID)

		body := `{"site_id": "` + siteID + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/vps/peers/"+newDeviceID+"/site", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			SiteID   *string `json:"site_id"`
			Assigned bool    `json:"assigned"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if !resp.Assigned {
			t.Fatal("expected assigned=true")
		}
		if resp.SiteID == nil || *resp.SiteID != siteID {
			t.Fatal("expected site_id to match")
		}

		var dbSiteID sql.NullString
		if err := db.QueryRow(`SELECT site_id FROM devices WHERE device_id = $1`, newDeviceID).Scan(&dbSiteID); err != nil {
			t.Fatal(err)
		}
		if !dbSiteID.Valid || dbSiteID.String != siteID {
			t.Fatal("database not updated")
		}
	})

	t.Run("PUT site - invalid site ID (foreign business)", func(t *testing.T) {
		newDeviceID := insertGatewayNoSite(db, bizID)

		otherBizID := insertBusiness(db)
		otherSiteID := insertSite(db, otherBizID)

		body := `{"site_id": "` + otherSiteID + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/vps/peers/"+newDeviceID+"/site", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["error"] != "Gateway or site not found in your business" {
			t.Fatalf("unexpected error: %v", resp)
		}

		// Verify database unchanged: gateway still has no site assigned
		var site sql.NullString
		err := db.QueryRow(`SELECT site_id FROM devices WHERE business_id=$1 AND device_id=$2`, bizID, newDeviceID).Scan(&site)
		if err != nil {
			t.Fatalf("database readback failed: %v", err)
		}
		if site.Valid {
			t.Fatalf("database incorrectly updated: foreign site assignment persisted: %s", site.String)
		}
	})

	t.Run("PUT site - invalid UUID format", func(t *testing.T) {
		newDeviceID := insertGatewayNoSite(db, bizID)

		body := `{"site_id": "not-a-uuid"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/vps/peers/"+newDeviceID+"/site", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["error"] != "Select a valid site" {
			t.Fatalf("unexpected error: %v", resp)
		}
	})

	t.Run("PUT site - non-MAIN_GATEWAY device", func(t *testing.T) {
		var otherDeviceID string
		if err := db.QueryRow(`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1, 'ap', 'ACCESS_POINT') RETURNING device_id`, bizID).Scan(&otherDeviceID); err != nil {
			t.Fatal(err)
		}

		body := `{"site_id": "` + siteID + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/vps/peers/"+otherDeviceID+"/site", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Unauthenticated request
	t.Run("unauthenticated - returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/site", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	// VIEWER role - should be denied by Admin middleware
	t.Run("VIEWER role - returns 403", func(t *testing.T) {
		viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/site", nil)
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	// VIEWER role write attempt - should be denied and database unchanged
	t.Run("VIEWER role write - returns 403, database unchanged", func(t *testing.T) {
		viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))
		body := `{"site_id": "` + siteID + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/vps/peers/"+deviceIDWithSite+"/site", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}

		// Verify database unchanged: gateway still has original site assignment
		var site sql.NullString
		err := db.QueryRow(`SELECT site_id FROM devices WHERE business_id=$1 AND device_id=$2`, bizID, deviceIDWithSite).Scan(&site)
		if err != nil {
			t.Fatalf("database readback failed: %v", err)
		}
		if !site.Valid || site.String != siteID {
			t.Fatalf("database incorrectly modified: expected site %s, got %v", siteID, site)
		}
	})
}

func TestVPSHandler_RequireGatewaySite_GuardedRoutes(t *testing.T) {
	db := harness_testdb(t)

	bizID := insertBusiness(db)
	siteID := insertSite(db, bizID)

	deviceWithSite := insertGateway(db, bizID, siteID)
	deviceWithoutSite := insertGatewayNoSite(db, bizID)

	router, counters := setupTestRouter(t, db)

	adminToken := makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin))
	viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))

	// Positive control
	counters.reset()
	body := `{"router_public_key":"Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=","management_rest_port":22,"confirm_enrollment":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/enroll", io.NopCloser(bytes.NewReader([]byte(body))))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("positive control failed: %d: %s", w.Code, w.Body.String())
	}
	if counters.liveDeviceCalls == 0 || counters.livePlanCalls == 0 || counters.enrollmentHubCalls == 0 {
		t.Fatal("positive control: expected liveDevice, livePlan, and enrollmentHub to be called")
	}
	t.Run("enroll - gateway with site passes", func(t *testing.T) {
		body := `{"router_public_key":"Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=","management_rest_port":22,"confirm_enrollment":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/enroll", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		// Site guard passes, and with full mocks EnrollGateway succeeds
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("enroll - gateway without site aborts with 409", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithoutSite+"/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["code"] != "site_required" {
			t.Fatalf("expected site_required code, got %v", resp)
		}
	})

	t.Run("enroll - non-existent gateway 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/00000000-0000-0000-0000-000000000000/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("enroll - invalid UUID 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/not-a-uuid/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("enroll - cross-business 404", func(t *testing.T) {
		otherBizID := insertBusiness(db)
		otherSiteID := insertSite(db, otherBizID)
		var otherDeviceID string
		if err := db.QueryRow(`INSERT INTO devices (business_id, site_id, hardware_name, deployment_layer) VALUES ($1, $2, 'gateway', 'MAIN_GATEWAY') RETURNING device_id`, otherBizID, otherSiteID).Scan(&otherDeviceID); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+otherDeviceID+"/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	// VIEWER role should be denied by Admin middleware before site guard
	t.Run("enroll - VIEWER role 403", func(t *testing.T) {
		viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Quick-enroll guard
	t.Run("quick-enroll - gateway with site passes site guard", func(t *testing.T) {
			// Create handler with liveHub that returns ErrPeerNotFound to trigger new enrollment path
			handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
			handler.ConfigureAutomaticManagement(
				func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
					return script, nil
				},
				func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
					return nil
				},
			)
			handler.ConfigureHubEnrollment(hubEnrollerFunc(func(ctx context.Context, id hub.Identity, publicKey string, port int) (hub.Peer, error) {
				return hub.Peer{
					PeerID:            "test-peer",
					TenantID:          id.TenantID,
					RouterID:          id.RouterID,
					RouterPublicKey:   publicKey,
					TunnelAddress:     "10.0.0.1/32",
					HubPublicKey:      "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
					Endpoint:          "10.0.0.1:51820",
					AllowedIPs:        []string{"10.0.0.2/32"},
					State:             "active",
					EnrollmentVersion: 1,
					LastHandshakeAt:   ptr(time.Now()),
				}, nil
			}))
			handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
				return hub.Peer{}, hub.ErrPeerNotFound
			})
			handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
						return &model.Device{DeviceID: deviceWithSite, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
					}
			handler.livePlan = func(biz string) bool {
				return true
			}

			r := gin.New()
			r.Use(gin.Recovery())
			r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
			r.Use(auth.RequireMinRole(model.RoleAdmin))
			r.POST("/api/v1/vps/peers/:deviceId/quick-enroll", handler.RequireGatewaySite, handler.QuickEnrollGateway)

			body := `{"management_rest_port":22,"confirm_enrollment":true}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/quick-enroll", io.NopCloser(bytes.NewReader([]byte(body))))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// Site guard passes, and with full mocks QuickEnrollGateway succeeds
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
		})

	t.Run("quick-enroll - without site 409", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithoutSite+"/quick-enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Finish guard
	t.Run("finish - without site 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/finish", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["code"] != "site_required" {
			t.Fatalf("expected site_required code, got %v", resp)
		}
	})

	t.Run("finish - with site but no automaticVerify 503", func(t *testing.T) {
		// Create handler without automaticVerify mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureManagementAuthentication(func(ctx context.Context, biz, id, addr, username, password string, port int, fingerprint string) error {
			return nil
		})
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{PeerID: "test-peer", TenantID: identity.TenantID, RouterID: identity.RouterID, State: "active", LastHandshakeAt: ptr(time.Now())}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: deviceWithSite, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
		}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.POST("/api/v1/vps/peers/:deviceId/finish", handler.RequireGatewaySite, handler.FinishGatewayConnection)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/finish", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Bootstrap guard
	t.Run("bootstrap - without site 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+noSiteDeviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bootstrap - with site but no enrollmentHub 503", func(t *testing.T) {
		// Create handler without enrollmentHub mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureAutomaticManagement(
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
				return script, nil
			},
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
				return nil
			},
		)
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{
				PeerID:            "test-peer",
				TenantID:          identity.TenantID,
				RouterID:          identity.RouterID,
				RouterPublicKey:   "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
				TunnelAddress:     "10.0.0.1/32",
				HubPublicKey:      "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
				Endpoint:          "10.0.0.1:51820",
				AllowedIPs:        []string{"10.0.0.2/32"},
				State:             "active",
				EnrollmentVersion: 1,
				LastHandshakeAt:   ptr(time.Now()),
			}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: deviceWithSite, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
		}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.GET("/api/v1/vps/peers/:deviceId/bootstrap", handler.RequireGatewaySite, handler.GatewayBootstrap)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceWithSite+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Authenticate guard
	t.Run("authenticate - without site 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["code"] != "site_required" {
			t.Fatalf("expected site_required code, got %v", resp)
		}
	})

	t.Run("authenticate - with site but no managementAuth 503", func(t *testing.T) {
		// Create handler without managementAuth mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureAutomaticManagement(
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
				return script, nil
			},
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
				return nil
			},
		)
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{PeerID: "test-peer", TenantID: identity.TenantID, RouterID: identity.RouterID, State: "active", LastHandshakeAt: ptr(time.Now())}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: deviceWithSite, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
		}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.POST("/api/v1/vps/peers/:deviceId/authenticate", handler.RequireGatewaySite, handler.AuthenticateGateway)

		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("authenticate - invalid input 400", func(t *testing.T) {
		body := `{"username":"admin","password":"pass","port":0,"confirm":false}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("authenticate - VIEWER role 403", func(t *testing.T) {
		viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))
		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Bootstrap guard
	t.Run("bootstrap - without site 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+noSiteDeviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bootstrap - with site but no enrollmentHub 503", func(t *testing.T) {
		// Create handler without enrollmentHub mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureAutomaticManagement(
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
				return script, nil
			},
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
				return nil
			},
		)
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{
				PeerID:            "test-peer",
				TenantID:          identity.TenantID,
				RouterID:          identity.RouterID,
				RouterPublicKey:   "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
				TunnelAddress:     "10.0.0.1/32",
				HubPublicKey:      "Gwzy3Vr4QPuD/6TNpF4pZ7pWNxqlKEiKcgmdvK91HzA=",
				Endpoint:          "10.0.0.1:51820",
				AllowedIPs:        []string{"10.0.0.2/32"},
				State:             "active",
				EnrollmentVersion: 1,
				LastHandshakeAt:   ptr(time.Now()),
			}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: deviceWithSite, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
		}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.GET("/api/v1/vps/peers/:deviceId/bootstrap", handler.RequireGatewaySite, handler.GatewayBootstrap)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceWithSite+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Unauthenticated access denied on all guarded routes
	t.Run("unauthenticated enroll 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/enroll", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthenticated finish 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/finish", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unauthenticated authenticate 401", func(t *testing.T) {
		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceWithSite+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Cross-business access denied
	t.Run("enroll - cross-business 404", func(t *testing.T) {
		otherBizID := insertBusiness(db)
		otherSiteID := insertSite(db, otherBizID)
		var otherDeviceID string
		if err := db.QueryRow(`INSERT INTO devices (business_id, site_id, hardware_name, deployment_layer) VALUES ($1, $2, 'gateway', 'MAIN_GATEWAY') RETURNING device_id`, otherBizID, otherSiteID).Scan(&otherDeviceID); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+otherDeviceID+"/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Invalid UUID format
	t.Run("enroll - invalid UUID 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/not-a-uuid/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Non-existent gateway
	t.Run("enroll - non-existent 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/00000000-0000-0000-0000-000000000000/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Callback non-invocation: denied requests must never reach enrollment/management callbacks
	t.Run("callback non-invocation on denied requests", func(t *testing.T) {
		checkAllZero := func(t *testing.T, c *callbackCounters, context string) {
			if c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 || c.enrollmentHubCalls != 0 {
				t.Errorf("%s: management/enrollment callbacks invoked unexpectedly: %+v", context, c)
			}
		}

		// Unauthenticated requests - no callbacks should be invoked
		endpoints := []struct {
			method string
			path   string
			body   string
		}{
			{"POST", "/api/v1/vps/peers/" + deviceWithSite + "/enroll", ""},
			{"POST", "/api/v1/vps/peers/" + deviceWithSite + "/finish", ""},
			{"POST", "/api/v1/vps/peers/" + deviceWithSite + "/authenticate", `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`},
		}

		for _, ep := range endpoints {
			counters.reset()
			var body io.Reader
			if ep.body != "" {
				body = bytes.NewReader([]byte(ep.body))
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(ep.method, ep.path, body)
			if ep.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, w.Code)
			}
			checkAllZero(t, counters, "unauthenticated "+ep.method+" "+ep.path)
		}

		// VIEWER role - Admin middleware should reject before any handler logic
		for _, ep := range endpoints {
			counters.reset()
			var body io.Reader
			if ep.body != "" {
				body = bytes.NewReader([]byte(ep.body))
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(ep.method, ep.path, body)
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			if ep.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s: expected 403, got %d", ep.method, ep.path, w.Code)
			}
			checkAllZero(t, counters, "viewer "+ep.method+" "+ep.path)
		}

		// Gateway without site - site guard should reject before enrollment/management callbacks
		noSiteDeviceID := insertGatewayNoSite(db, bizID)
		siteGuardEndpoints := []struct {
			method string
			path   string
			body   string
		}{
			{"POST", "/api/v1/vps/peers/" + noSiteDeviceID + "/enroll", ""},
			{"POST", "/api/v1/vps/peers/" + noSiteDeviceID + "/finish", ""},
			{"POST", "/api/v1/vps/peers/" + noSiteDeviceID + "/authenticate", `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`},
			{"GET", "/api/v1/vps/peers/" + noSiteDeviceID + "/bootstrap", ""},
		}

		for _, ep := range siteGuardEndpoints {
			counters.reset()
			var body io.Reader
			if ep.body != "" {
				body = bytes.NewReader([]byte(ep.body))
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(ep.method, ep.path, body)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			if ep.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			router.ServeHTTP(w, req)

			if w.Code != http.StatusConflict {
				t.Errorf("%s %s: expected 409, got %d", ep.method, ep.path, w.Code)
			}
			// Site guard runs liveDevice/livePlan to check site status, so those may be called
			// But enrollment/management callbacks must NOT be called
			if c := counters; c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 {
				t.Errorf("%s %s: management callbacks invoked on missing site: %+v", ep.method, ep.path, c)
			}
		}

		// Cross-business access - should 404 before any handler logic
		counters.reset()
		otherBizID := insertBusiness(db)
		otherSiteID := insertSite(db, otherBizID)
		var otherDeviceID string
		if err := db.QueryRow(`INSERT INTO devices (business_id, site_id, hardware_name, deployment_layer) VALUES ($1, $2, 'gateway', 'MAIN_GATEWAY') RETURNING device_id`, otherBizID, otherSiteID).Scan(&otherDeviceID); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+otherDeviceID+"/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("cross-business: expected 404, got %d", w.Code)
		}
		checkAllZero(t, counters, "cross-business enroll")

		// Invalid UUID - should 400 before any handler logic
		counters.reset()
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/not-a-uuid/enroll", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid UUID: expected 400, got %d", w.Code)
		}
		checkAllZero(t, counters, "invalid UUID enroll")
	})
}

// TestVPSHandler_FinishGatewayConnection verifies the connection completion endpoint
// is guarded by RequireGatewaySite and requires a recent Hub handshake.
func TestVPSHandler_FinishGatewayConnection(t *testing.T) {
	db := harness_testdb(t)

	bizID := insertBusiness(db)
	siteID := insertSite(db, bizID)
	deviceID := insertGateway(db, bizID, siteID)

	router, counters := setupTestRouter(t, db)
	adminToken := makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin))
	viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))

	// Positive control
	counters.reset()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/finish", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("positive control failed: %d: %s", w.Code, w.Body.String())
	}
	if counters.liveDeviceCalls == 0 || counters.livePlanCalls == 0 || counters.automaticVerifyCalls == 0 {
		t.Fatal("positive control: expected liveDevice, livePlan, and automaticVerify to be called")
	}

	t.Run("without site - guard aborts with 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/finish", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 from site guard, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["code"] != "site_required" {
			t.Fatalf("expected site_required code, got %v", resp)
		}
	})

	t.Run("with site but no automaticVerify - returns 503", func(t *testing.T) {
			// Create handler without automaticVerify mock
			handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
			handler.ConfigureManagementAuthentication(func(ctx context.Context, biz, id, addr, username, password string, port int, fingerprint string) error {
				return nil
			})
			handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
				return hub.Peer{PeerID: "test-peer", TenantID: identity.TenantID, RouterID: identity.RouterID, State: "active", LastHandshakeAt: ptr(time.Now())}, nil
			})
			handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
				return &model.Device{DeviceID: deviceID, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
			}
			handler.livePlan = func(biz string) bool {
				return true
			}

			r := gin.New()
			r.Use(gin.Recovery())
			r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
			r.Use(auth.RequireMinRole(model.RoleAdmin))
			r.POST("/api/v1/vps/peers/:deviceId/finish", handler.RequireGatewaySite, handler.FinishGatewayConnection)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/finish", nil)
			req.Header.Set("Authorization", "Bearer "+makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin)))
			r.ServeHTTP(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
			}
		})

		// Callback non-invocation: denied requests must not reach management callbacks
		t.Run("callback non-invocation on denied requests", func(t *testing.T) {
			checkAllZero := func(t *testing.T, c *callbackCounters, context string) {
				if c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 || c.enrollmentHubCalls != 0 {
					t.Errorf("%s: management/enrollment callbacks invoked unexpectedly: %+v", context, c)
				}
			}

			// Unauthenticated
			counters.reset()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/finish", nil)
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("unauthenticated finish: expected 401, got %d", w.Code)
			}
			checkAllZero(t, counters, "unauthenticated finish")

			// VIEWER role
			counters.reset()
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/finish", nil)
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("viewer finish: expected 403, got %d", w.Code)
			}
			checkAllZero(t, counters, "viewer finish")

			// Gateway without site
			noSiteDeviceID := insertGatewayNoSite(db, bizID)
			counters.reset()
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/finish", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			router.ServeHTTP(w, req)
			if w.Code != http.StatusConflict {
				t.Errorf("finish without site: expected 409, got %d", w.Code)
			}
			if c := counters; c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 {
				t.Errorf("finish without site: management callbacks invoked: %+v", c)
			}
		})
	}

// TestVPSHandler_AuthenticateGateway verifies the management authentication endpoint
// is guarded by RequireGatewaySite.
func TestVPSHandler_AuthenticateGateway(t *testing.T) {
	db := harness_testdb(t)

	bizID := insertBusiness(db)
	siteID := insertSite(db, bizID)
	deviceID := insertGateway(db, bizID, siteID)

	router, counters := setupTestRouter(t, db)
	adminToken := makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin))
	viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))

	// Positive control
	counters.reset()
	body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("positive control failed: %d: %s", w.Code, w.Body.String())
	}
	if counters.liveDeviceCalls == 0 || counters.livePlanCalls == 0 || counters.managementAuthCalls == 0 {
		t.Fatal("positive control: expected liveDevice, livePlan, and managementAuth to be called")
	}

	t.Run("without site - guard aborts with 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 from site guard, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["code"] != "site_required" {
			t.Fatalf("expected site_required code, got %v", resp)
		}
	})

	t.Run("with site but no managementAuth - returns 503", func(t *testing.T) {
		// Create handler without managementAuth mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureAutomaticManagement(
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
				return script, nil
			},
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
				return nil
			},
		)
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{PeerID: "test-peer", TenantID: identity.TenantID, RouterID: identity.RouterID, State: "active", LastHandshakeAt: ptr(time.Now())}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
				return &model.Device{DeviceID: deviceID, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
			}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.POST("/api/v1/vps/peers/:deviceId/authenticate", handler.RequireGatewaySite, handler.AuthenticateGateway)

		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid input - returns 400", func(t *testing.T) {
		body := `{"username":"admin","password":"pass","port":0,"confirm":false}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin)))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Role denial regression
	t.Run("VIEWER role - returns 403", func(t *testing.T) {
		viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))
		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Callback non-invocation: denied requests must not reach management callbacks
	t.Run("callback non-invocation on denied requests", func(t *testing.T) {
		checkAllZero := func(t *testing.T, c *callbackCounters, context string) {
			if c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 || c.enrollmentHubCalls != 0 {
				t.Errorf("%s: management/enrollment callbacks invoked unexpectedly: %+v", context, c)
			}
		}

		// Unauthenticated
		counters.reset()
		body := `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated authenticate: expected 401, got %d", w.Code)
		}
		checkAllZero(t, counters, "unauthenticated authenticate")

		// VIEWER role
		counters.reset()
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+deviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("viewer authenticate: expected 403, got %d", w.Code)
		}
		checkAllZero(t, counters, "viewer authenticate")

		// Gateway without site
		noSiteDeviceID := insertGatewayNoSite(db, bizID)
		counters.reset()
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/vps/peers/"+noSiteDeviceID+"/authenticate", io.NopCloser(bytes.NewReader([]byte(body))))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Errorf("authenticate without site: expected 409, got %d", w.Code)
		}
		if c := counters; c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 {
			t.Errorf("authenticate without site: management callbacks invoked: %+v", c)
		}
	})
}

// TestVPSHandler_GatewayBootstrap verifies the bootstrap endpoint is guarded.
func TestVPSHandler_GatewayBootstrap(t *testing.T) {
	db := harness_testdb(t)

	bizID := insertBusiness(db)
	siteID := insertSite(db, bizID)
	deviceID := insertGateway(db, bizID, siteID)

	router, counters := setupTestRouter(t, db)
	adminToken := makeTestToken(t, "user-admin", bizID, string(model.RoleAdmin))
	viewerToken := makeTestToken(t, "user-viewer", bizID, string(model.RoleViewer))

	// Positive control
	counters.reset()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("positive control failed: %d: %s", w.Code, w.Body.String())
	}
	if counters.liveDeviceCalls == 0 || counters.livePlanCalls == 0 {
		t.Fatal("positive control: expected liveDevice and livePlan to be called")
	}

	t.Run("without site - guard aborts with 409", func(t *testing.T) {
		noSiteDeviceID := insertGatewayNoSite(db, bizID)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+noSiteDeviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 from site guard, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("with site but no enrollmentHub - returns 503", func(t *testing.T) {
		// Create handler without enrollmentHub mock
		handler := NewVPSHandler(db, database.NewDeviceRepository(db), database.NewBusinessMeteredQuotaRepository(db), nil, config.WireGuardConfig{}, "")
		handler.ConfigureAutomaticManagement(
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) (string, error) {
				return script, nil
			},
			func(ctx context.Context, biz, deviceID, routerPubKey, script string) error {
				return nil
			},
		)
		handler.liveHub = hubInspectorFunc(func(ctx context.Context, identity hub.Identity) (hub.Peer, error) {
			return hub.Peer{
				PeerID:            "test-peer",
				TenantID:          identity.TenantID,
				RouterID:          identity.RouterID,
				RouterPublicKey:   "test-router-pubkey",
				TunnelAddress:     "10.0.0.1/32",
				HubPublicKey:      "hub-pubkey",
				Endpoint:          "hub.example.com:51820",
				AllowedIPs:        []string{"10.0.0.0/24"},
				State:             "active",
				EnrollmentVersion: 1,
				LastHandshakeAt:   ptr(time.Now()),
			}, nil
		})
		handler.liveDevice = func(c *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: deviceID, BusinessID: bizID, DeploymentLayer: "MAIN_GATEWAY"}, nil
		}
		handler.livePlan = func(biz string) bool {
			return true
		}

		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(auth.CookieOrigin(), auth.Middleware(testAuthService()))
		r.Use(auth.RequireMinRole(model.RoleAdmin))
		r.GET("/api/v1/vps/peers/:deviceId/bootstrap", handler.RequireGatewaySite, handler.GatewayBootstrap)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Callback non-invocation: denied requests must not reach enrollment callbacks
	t.Run("callback non-invocation on denied requests", func(t *testing.T) {
		checkAllZero := func(t *testing.T, c *callbackCounters, context string) {
			if c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 {
				t.Errorf("%s: management callbacks invoked unexpectedly: %+v", context, c)
			}
		}

		// Unauthenticated
		counters.reset()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/bootstrap", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated bootstrap: expected 401, got %d", w.Code)
		}
		checkAllZero(t, counters, "unauthenticated bootstrap")

		// VIEWER role
		counters.reset()
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+deviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("viewer bootstrap: expected 403, got %d", w.Code)
		}
		checkAllZero(t, counters, "viewer bootstrap")

		// Gateway without site
		noSiteDeviceID := insertGatewayNoSite(db, bizID)
		counters.reset()
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/vps/peers/"+noSiteDeviceID+"/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Errorf("bootstrap without site: expected 409, got %d", w.Code)
		}
		if c := counters; c.managementAuthCalls != 0 || c.automaticPrepareCalls != 0 || c.automaticVerifyCalls != 0 {
			t.Errorf("bootstrap without site: management callbacks invoked: %+v", c)
		}
	})
}

// TestVPSHandler_RouteRegistration verifies all expected routes are registered.
func TestVPSHandler_RouteRegistration(t *testing.T) {
	db := harness_testdb(t)
	router, _ := setupTestRouter(t, db)

	// Check that all expected routes exist by making a request and verifying
	// they don't return 404 (route not found). They may return other codes
	// due to missing dependencies or auth, but not 404.

	testCases := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/vps/peers/test-device/site", ""},
		{"PUT", "/api/v1/vps/peers/test-device/site", `{"site_id":"00000000-0000-0000-0000-000000000000"}`},
		{"POST", "/api/v1/vps/peers/test-device/enroll", ""},
		{"POST", "/api/v1/vps/peers/test-device/quick-enroll", ""},
		{"POST", "/api/v1/vps/peers/test-device/finish", ""},
		{"GET", "/api/v1/vps/peers/test-device/bootstrap", ""},
		{"POST", "/api/v1/vps/peers/test-device/authenticate", `{"username":"admin","password":"pass","port":22,"fingerprint":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","confirm":true}`},
	}

	for _, tc := range testCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = io.NopCloser(bytes.NewReader([]byte(tc.body)))
			}
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer "+makeTestToken(t, "user-admin", "test-biz", string(model.RoleAdmin)))
			router.ServeHTTP(w, req)

			// Route exists if not 404 (may return other codes due to missing deps/auth)
			if w.Code == http.StatusNotFound {
				t.Fatalf("route %s %s not registered (got 404)", tc.method, tc.path)
			}
		})
	}
}