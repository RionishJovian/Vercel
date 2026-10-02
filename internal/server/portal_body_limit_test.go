package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPortalBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, length := range []int64{7 << 20, -1} {
		r := gin.New()
		r.POST("/", boundedPortalBody(), func(c *gin.Context) {
			if _, err := io.ReadAll(c.Request.Body); err != nil {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 7<<20)))
		req.ContentLength = length
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("length %d: %d", length, w.Code)
		}
	}
}
