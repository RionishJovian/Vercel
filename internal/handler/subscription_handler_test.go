package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"mega-magic/internal/subscription"
)

func TestSubscriptionMiddlewareAccessAndRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	for _, tc := range []struct {
		name    string
		enabled bool
		status  subscription.Status
		err     error
		path    string
		want    int
	}{
		{"disabled", false, subscription.Status{}, errors.New("must not query"), "/api/v1/routers", 200},
		{"trial", true, subscription.Evaluate(now, &future, nil, "require_payment"), nil, "/api/v1/routers", 200},
		{"paid", true, subscription.Evaluate(now, nil, &future, "require_payment"), nil, "/api/v1/media/jobs", 200},
		{"expired", true, subscription.Evaluate(now, &now, nil, "require_payment"), nil, "/api/v1/routers", 402},
		{"account recovery", true, subscription.Status{}, nil, "/api/v1/me/password", 200},
		{"checkout recovery", true, subscription.Status{}, nil, "/api/v1/store/checkout-options", 200},
		{"history recovery", true, subscription.Status{}, nil, "/api/v1/store/purchases", 200},
		{"not prefix", true, subscription.Status{}, nil, "/api/v1/store/purchases/extra", 402},
		{"media denied", true, subscription.Status{}, nil, "/api/v1/media/jobs", 402},
		{"issuance denied", true, subscription.Status{}, nil, "/api/v1/guest-vouchers", 402},
		{"lookup fails closed", true, subscription.Status{}, errors.New("private DB details"), "/api/v1/routers", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called, queries := false, 0
			r := gin.New()
			r.Use(subscriptionAccess(tc.enabled, func(*gin.Context) (subscription.Status, error) { queries++; return tc.status, tc.err }))
			r.GET(tc.path, func(c *gin.Context) { called = true; c.Status(200) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.want || called != (tc.want == 200) {
				t.Fatalf("code=%d called=%v", w.Code, called)
			}
			if !tc.enabled && queries != 0 {
				t.Fatal("disabled policy queried DB")
			}
			if strings.Contains(w.Body.String(), "private DB") {
				t.Fatal("error leaked")
			}
			if tc.enabled && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("subscription decision cacheable")
			}
		})
	}
}

func TestExpiredSubscriptionDoesNotBypassApprovalRole(t *testing.T) {
	for _, role := range []string{"VIEWER", "ADMIN"} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set(auth.ContextKeyUserRole, role) }, subscriptionAccess(true, func(*gin.Context) (subscription.Status, error) { return subscription.Status{}, nil }))
		r.POST("/api/v1/store/purchases/:purchaseId/approve", auth.RequireMinRole(model.RoleAdmin), func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/store/purchases/test/approve", nil))
		want := 200
		if role == "VIEWER" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("role %s code %d", role, w.Code)
		}
	}
}

func TestSubscriptionMissingDatabaseFailsClosed(t *testing.T) {
	r := gin.New()
	r.Use(SubscriptionAccess(nil, true, "require_payment"))
	r.GET("/api/v1/routers", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/routers", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
