package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

func TestGeoTopologyRouteRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authService, err := auth.NewService(
		"test-secret-long-enough",
		time.Hour,
		24*time.Hour,
		4,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	r := gin.New()
	protected := r.Group("/api/v1", auth.Middleware(authService))
	registerGeoTopologyRoute(protected, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"source": "real-topology-handler"})
	})

	withoutToken := httptest.NewRecorder()
	r.ServeHTTP(withoutToken, httptest.NewRequest(http.MethodGet, "/api/v1/geo/topology-canvas", nil))
	if withoutToken.Code != http.StatusUnauthorized {
		t.Fatalf("without token = %d, want 401", withoutToken.Code)
	}

	token, err := authService.GenerateToken(&model.User{
		UserID:     "user-1",
		BusinessID: "business-1",
		Email:      "owner@example.test",
		UserRole:   model.RoleOwner,
	})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	withToken := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/topology-canvas", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(withToken, req)
	if withToken.Code != http.StatusOK {
		t.Fatalf("with token = %d, want 200", withToken.Code)
	}
	if body := withToken.Body.String(); body != "{\"source\":\"real-topology-handler\"}" {
		t.Fatalf("handler response = %s", body)
	}
}
