package handler

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/subscription"
	"net/http"
	"time"
)

func SubscriptionAccess(db *database.DB, enabled bool, existingPolicy string) gin.HandlerFunc {
	return subscriptionAccess(enabled, func(c *gin.Context) (subscription.Status, error) {
		if db == nil {
			return subscription.Status{}, sql.ErrConnDone
		}
		var status subscription.Status
		err := runInTenant(c, db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
			var e error
			status, e = database.SubscriptionStatusTXN(tx, auth.GetBusinessID(c), existingPolicy, time.Now().UTC())
			return e
		})
		return status, err
	})
}

func subscriptionAccess(enabled bool, load func(*gin.Context) (subscription.Status, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		status, err := load(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Subscription status unavailable", "code": "subscription_unavailable"})
			return
		}
		c.Set("subscription_status", status)
		if !status.Access && !subscription.RecoveryRoute(c.FullPath()) {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{"error": "Choose a monthly or annual plan to continue", "code": "subscription_required", "subscription": status})
			return
		}
		c.Next()
	}
}

func SubscriptionStatus(c *gin.Context) {
	if value, ok := c.Get("individual_pass_status"); ok {
		c.JSON(http.StatusOK, value)
		return
	}
	if value, ok := c.Get("subscription_status"); ok {
		c.JSON(http.StatusOK, value)
		return
	}
	c.JSON(http.StatusOK, subscription.Status{Mode: "legacy", FeatureRequirementsEnabled: true})
}
