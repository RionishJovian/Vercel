package handler

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/accountpass"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"time"
)

func IndividualPassAccess(db *database.DB, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		c.Header("Cache-Control", "no-store")
		operator := auth.GetUserRole(c) == string(model.RoleDeveloper)
		var status accountpass.Status
		if operator {
			status = accountpass.Evaluate(time.Now().UTC(), true, true, nil, nil, nil, nil)
		} else {
			err := runInTenant(c, db, auth.GetBusinessID(c), func(tx *sql.Tx) error {
				var e error
				status, e = database.AccountPassStatusTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), true, false, time.Now().UTC())
				return e
			})
			if err != nil {
				c.AbortWithStatusJSON(503, gin.H{"error": "Individual pass status unavailable", "code": "account_pass_unavailable"})
				return
			}
		}
		c.Header("Cache-Control", "no-store")
		c.Set("individual_pass_status", status)
		if !status.Access && !accountpass.RecoveryRoute(c.Request.Method, c.FullPath()) {
			c.AbortWithStatusJSON(402, gin.H{"error": "Your account pass has expired or needs activation. Sign in to request renewal.", "code": "account_pass_required", "subscription": status})
			return
		}
		c.Next()
	}
}
