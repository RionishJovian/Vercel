package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"mega-magic/internal/subscription"
	"net/http"
	"time"
)

func accountTime(v any) *time.Time {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return &t
}

func (h *PlatformHandler) Accounts(c *gin.Context) {
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.JSON(http.StatusForbidden, gin.H{"error": "developer access required"})
		return
	}
	limit, err := parseBoundedInt(c, "limit", 50, 1, 100)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1000000)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	accounts, total, err := h.businessRepo.PlatformAccounts(c.Request.Context(), limit, offset, h.SubscriptionOnly, now)
	if err != nil {
		c.JSON(503, gin.H{"error": "account subscription and presence data unavailable"})
		return
	}
	for _, a := range accounts {
		pass := a["pass"].(map[string]any)
		expiry := accountTime(pass["expires_at"])
		state := "no_pass"
		if pass["kind"] == "spark" {
			state = "free"
		} else if pass["kind"] != "none" {
			state = "unverified"
			if expiry != nil {
				state = "expired"
				if expiry.After(now) {
					state = "active"
				}
			}
		}
		pass["state"] = state
		if h.SubscriptionOnly {
			sub := subscription.Evaluate(now, accountTime(pass["trial_expires_at"]), accountTime(pass["paid_expires_at"]), h.ExistingAccountPolicy)
			a["subscription"] = sub
		} else {
			a["subscription"] = gin.H{"mode": "legacy", "state": "legacy_feature_gates", "access": a["is_active"]}
		}
		access := a["is_active"] == true
		if h.SubscriptionOnly {
			access = access && a["subscription"].(subscription.Status).Access
		}
		role := model.UserRole(a["user_role"].(string))
		admin := model.HasAtLeastRole(role, model.RoleAdmin)
		hubExpiry := accountTime(a["limits"].(map[string]any)["hub_pass_expires_at"])
		hubPlan := h.SubscriptionOnly || (hubExpiry != nil && hubExpiry.After(now))
		a["permissions"] = gin.H{"magic_hub": access && admin && hubPlan, "router_management": access && admin, "voucher_issuance": access && admin, "staff_management": access && model.HasAtLeastRole(role, model.RoleOwner), "jellyfin_management": access && admin, "magic_dude": access && (h.SubscriptionOnly || admin || a["magic_dude_key"] == true)}
		delete(a, "magic_dude_key")
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"accounts": accounts, "total": total, "limit": limit, "offset": offset, "as_of": now, "presence_timeout_seconds": 90, "idle_after_seconds": 300})
}
