package handler

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/managementmigration"
	"mega-magic/internal/model"
	"net/http"
	"os"
	"regexp"
	"time"
)

type managementMigrationController interface {
	Status(context.Context, string, string) (*managementmigration.Job, error)
	Preview(context.Context, string, string, string) (*managementmigration.Job, error)
	Run(context.Context, string, string, string, string, string, int) (*managementmigration.Job, error)
}

func (h *SecurityHandler) ConfigureManagementMigrations(controller managementMigrationController) {
	h.managementMoves = controller
}

var managementMoveID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (h *SecurityHandler) ManagementMigration(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	// Separate from general network writes: deploy schema + Hub protocol before
	// allowing an address move. Readiness failure never falls back to enrollment.
	enabled := os.Getenv("MM_MANAGEMENT_MIGRATION_ENABLED") == "true"
	if !enabled || h.managementMoves == nil {
		if c.Request.Method == http.MethodGet {
			c.JSON(200, gin.H{"enabled": false, "reason": "Guided management migration is not enabled on this server yet"})
		} else {
			c.JSON(503, gin.H{"error": "Guided management migration is unavailable; no router was changed"})
		}
		return
	}
	biz, device, user := auth.GetBusinessID(c), c.Param("id"), auth.GetUserID(c)
	if biz == "" || user == "" {
		c.Status(401)
		return
	}
	if !h.deviceExists(c, biz, device) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Second)
	defer cancel()
	action := c.Param("action")
	var job *managementmigration.Job
	var err error
	if c.Request.Method == http.MethodGet {
		job, err = h.managementMoves.Status(ctx, biz, device)
	} else {
		var body struct {
			ID        string `json:"id"`
			Revision  int    `json:"revision"`
			Confirmed bool   `json:"confirmed"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
		if c.ShouldBindJSON(&body) != nil {
			c.JSON(400, gin.H{"error": "Invalid migration request"})
			return
		}
		if action == "preview" {
			job, err = h.managementMoves.Preview(ctx, biz, device, user)
		} else if (action == "apply" || action == "recover") && body.Confirmed && managementMoveID.MatchString(body.ID) && body.Revision >= 0 {
			job, err = h.managementMoves.Run(ctx, biz, device, user, body.ID, action, body.Revision)
		} else {
			c.JSON(400, gin.H{"error": "Confirm the exact migration before applying or recovering"})
			return
		}
	}
	if err != nil {
		message := "Migration could not be verified. Refresh status before retrying; use recovery for an interrupted operation."
		code := "migration_unverified"
		var safe *managementmigration.UserError
		if errors.As(err, &safe) {
			message = safe.Message
			code = safe.Code
		}
		c.JSON(409, gin.H{"enabled": true, "job": job, "code": code, "error": message})
		return
	}
	c.JSON(200, gin.H{"enabled": true, "job": job})
}

// Production and tests share the registration and role boundary.
func RegisterManagementMigrationRoutes(routers *gin.RouterGroup, h *SecurityHandler) {
	routers.GET("/:id/management-migration", auth.RequireMinRole(model.RoleAdmin), h.ManagementMigration)
	routers.POST("/:id/management-migration/:action", auth.RequireMinRole(model.RoleAdmin), h.ManagementMigration)
}
