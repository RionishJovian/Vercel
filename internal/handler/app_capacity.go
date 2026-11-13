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
	case "app_business_gateway_capacity":
		c.JSON(409, gin.H{"code": "gateway_capacity_reached", "error": "All gateway slots on this account are in use. Buy another gateway slot in the Store, or remove a gateway you no longer use.", "resource": "gateway", "scope": "business"})
		return true
	default:
		return false
	}
	c.JSON(409, gin.H{"code": "workspace_capacity_reached", "error": "Your client workspace includes one " + resource + ". Manage the existing record before adding another.", "resource": resource, "limit": 1, "scope": "business"})
	return true
}

// isGatewayCapacity reports the database refusal of a gateway beyond the
// account's gateway capacity (business_gateway_capacity()).
func isGatewayCapacity(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23514" && pe.Constraint == "app_business_gateway_capacity"
}
