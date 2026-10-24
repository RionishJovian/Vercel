package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// Only our named constraints are a subscription capacity response. Other SQL
// failures must not be disguised as an upgrade prompt or a successful write.
func respondAppCapacity(c *gin.Context, err error) bool {
	var pe *pq.Error
	if !errors.As(err, &pe) || pe.Code != "23514" {
		return false
	}
	resource := ""
	switch pe.Constraint {
	case "app_business_site_limit":
		resource = "site"
	case "app_business_router_limit":
		resource = "router"
	default:
		return false
	}
	c.JSON(409, gin.H{"code": "workspace_capacity_reached", "error": "Your client workspace includes one " + resource + ". Manage the existing record before adding another.", "resource": resource, "limit": 1, "scope": "business"})
	return true
}
