package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// forceDeleteHistory lists, in delete order, every ledger that keeps a device record alive
// (a foreign key without a cascade). Children come before the tables they point at. The
// tenant role has no DELETE on these on purpose, so only this Platform Admin path, on the
// owner connection, may remove them. Vouchers, purchases and incidents are not listed: they
// only lose their device link (ON DELETE SET NULL) and stay as business records.
var forceDeleteHistory = []string{
	"broadband_session_usage",
	"broadband_operations",
	"broadband_accounts",
	"vip_wifi_codes",
	"ap_radio_proposals",
	"guest_access_recovery",
	"guest_voucher_grants",
	"guest_voucher_issue_batches",
}

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// forceDeleteConfirmation is what the Platform Admin must type: the record's name, or its id
// when it has no name. It stops a stray request from erasing history.
func forceDeleteConfirmation(d *model.Device) string {
	if name := strings.TrimSpace(d.HardwareName); name != "" {
		return name
	}
	return d.DeviceID
}

// forceDeleteRouter permanently deletes a router record together with the history that
// normally keeps it: PPPoE accounts and payments, voucher grants, batches and recovery, VIP
// Wi-Fi codes and AP review records. Everything happens in one transaction: if the Hub cannot
// end the connection, or anything still points at the device, nothing changes.
//
//	DELETE /routers/:id?force=true&confirm=<record name>
func (h *RouterHandler) forceDeleteRouter(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)
	userID := auth.GetUserID(c)
	if auth.GetUserRole(c) != string(model.RoleDeveloper) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the Platform Admin can permanently delete a record that has history."})
		return
	}

	cleanup, hubEnded := "", false
	removed := map[string]int64{}
	err := func() error {
		tx, err := h.db.Begin()
		if err != nil {
			log.Printf("router force delete: begin failed for %s: %v", deviceID, err)
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`SET LOCAL lock_timeout='3s'`); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		existing, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "router not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}
		if c.Query("confirm") != forceDeleteConfirmation(existing) {
			return &httpError{status: http.StatusBadRequest, msg: "Type the record's name exactly to confirm. Nothing was deleted."}
		}

		for _, table := range forceDeleteHistory {
			var present bool
			if err := tx.QueryRow(`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&present); err != nil {
				return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
			}
			if !present {
				continue
			}
			// The table name comes from the fixed list above, never from the request.
			res, err := tx.Exec(`DELETE FROM `+table+` WHERE device_id=$1 AND business_id=$2`, deviceID, businessID)
			if err != nil {
				log.Printf("router force delete: clearing %s for %s failed: %v", table, deviceID, err)
				return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
			}
			if n, _ := res.RowsAffected(); n > 0 {
				removed[table] = n
			}
		}

		var actor any
		if uuidShape.MatchString(userID) {
			actor = userID
		}
		before, _ := json.Marshal(map[string]any{"hardware_name": existing.HardwareName, "deployment_layer": existing.DeploymentLayer, "history_removed": removed})
		if _, err := tx.Exec(`INSERT INTO audit_events(business_id,user_id,user_role,action,entity_type,entity_id,before_state,reason)
VALUES($1,$2,$3,'router.force_delete','device',$4,$5::jsonb,'Platform Admin permanently deleted a router record and its history')`,
			businessID, actor, string(model.RoleDeveloper), deviceID, string(before)); err != nil {
			log.Printf("router force delete: audit failed for %s: %v", deviceID, err)
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}

		if err := h.deviceRepo.DeleteDeviceTXN(tx, deviceID); err != nil {
			var pg *pq.Error
			switch {
			case errors.As(err, &pg) && pg.Code == "55000" && pg.Message == "management_migration_pending":
				return &httpError{status: http.StatusConflict, msg: "Recover or cancel this router's management migration before deleting its record"}
			case errors.As(err, &pg) && pg.Code == "23503":
				log.Printf("router force delete: %s is still referenced by %s", deviceID, pg.Constraint)
				return &httpError{status: http.StatusConflict, msg: "This record is still linked to other history, so nothing was deleted."}
			default:
				log.Printf("router force delete: delete failed for %s: %v", deviceID, err)
				return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
			}
		}
		// Last, as in the normal delete: if the Hub cannot end the connection, nothing changes.
		if h.hubRemoval != nil && existing.DeploymentLayer == "MAIN_GATEWAY" {
			script, err := h.hubRemoval(c.Request.Context(), businessID, userID, deviceID)
			if err != nil {
				return &httpError{status: http.StatusServiceUnavailable, msg: "Magic Hub could not end this router's connection, so nothing was deleted. Try again."}
			}
			cleanup, hubEnded = script, true
		}
		return tx.Commit()
	}()
	if err != nil {
		if hubEnded {
			log.Printf("router force delete: Hub connection ended for %s but the record was not updated: %v", deviceID, err)
			response := gin.H{"error": "Magic Hub ended this router's connection, but its record could not be updated. Press Delete again to finish.", "code": "hub_ended_record_unchanged"}
			if cleanup != "" {
				c.Header("Cache-Control", "no-store")
				response["router_cleanup_script"] = cleanup
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}
		respondErr(c, err)
		return
	}
	log.Printf("router force delete: %s deleted by platform admin %s, history removed: %v", deviceID, userID, removed)
	response := gin.H{"message": "router deleted", "forced": true, "removed_history": removed}
	if cleanup != "" {
		c.Header("Cache-Control", "no-store")
		response["router_cleanup_script"] = cleanup
	}
	c.JSON(http.StatusOK, response)
}
