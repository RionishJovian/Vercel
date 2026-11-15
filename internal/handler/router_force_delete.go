package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// forceDeleteStep is one table the force delete must empty (for this device only) before the
// device row can go. scope is a WHERE clause over $1 (device_id) and $2 (business_id).
type forceDeleteStep struct {
	table string
	scope string
}

// forceDeleteEdge is one foreign key between two tables in the public schema.
type forceDeleteEdge struct {
	child, parent string
	childCols     []string
	parentCols    []string
	action        string // pg_constraint.confdeltype: a=no action r=restrict c=cascade n=set null d=set default
}

// forceDeleteSteps reads every foreign key from the live catalog and returns, children first,
// each table whose rows would stop the device row from being deleted: tables that point at
// devices (directly, or through other such tables) without ON DELETE CASCADE / SET NULL. Reading
// the catalog on every request means a table added by a later migration can never be missed.
// Vouchers, purchases and incidents are not returned: they only lose their device link
// (ON DELETE SET NULL) and stay as business records. A table that cascades from one of the
// returned tables is returned too when it has blockers of its own.
func forceDeleteSteps(q interface {
	Query(query string, args ...any) (*sql.Rows, error)
}) ([]forceDeleteStep, error) {
	rows, err := q.Query(`
SELECT cl.relname, pl.relname, c.confdeltype::text,
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(c.conkey) WITH ORDINALITY k(attnum,ord)
     JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=k.attnum),
  (SELECT array_agg(a.attname::text ORDER BY k.ord) FROM unnest(c.confkey) WITH ORDINALITY k(attnum,ord)
     JOIN pg_attribute a ON a.attrelid=c.confrelid AND a.attnum=k.attnum)
FROM pg_constraint c
JOIN pg_class cl ON cl.oid=c.conrelid
JOIN pg_class pl ON pl.oid=c.confrelid
WHERE c.contype='f' AND cl.relnamespace='public'::regnamespace AND pl.relnamespace='public'::regnamespace
  AND c.conrelid<>c.confrelid
ORDER BY cl.relname, c.conname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var edges []forceDeleteEdge
	for rows.Next() {
		var e forceDeleteEdge
		if err := rows.Scan(&e.child, &e.parent, &e.action, pq.Array(&e.childCols), pq.Array(&e.parentCols)); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return planForceDelete(edges), nil
}

// planForceDelete is the pure part of forceDeleteSteps (kept apart so it can be tested without a database).
func planForceDelete(edges []forceDeleteEdge) []forceDeleteStep {
	blocks := func(action string) bool { return action == "a" || action == "r" }
	inSet := map[string]bool{"devices": true}
	hasBlocker := func(table string) bool {
		for _, e := range edges {
			if e.parent == table && blocks(e.action) {
				return true
			}
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			if inSet[e.child] || !inSet[e.parent] {
				continue
			}
			if blocks(e.action) || (e.action == "c" && hasBlocker(e.child)) {
				inSet[e.child], changed = true, true
			}
		}
	}
	// Longest path from devices: a table is deleted only after everything below it.
	depth := map[string]int{"devices": 0}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			if inSet[e.child] && inSet[e.parent] {
				if pd, ok := depth[e.parent]; ok && depth[e.child] < pd+1 {
					depth[e.child], changed = pd+1, true
				}
			}
		}
	}
	scopes := map[string]string{}
	var scopeOf func(table string, seen map[string]bool) string
	scopeOf = func(table string, seen map[string]bool) string {
		if table == "devices" {
			return `device_id=$1 AND business_id=$2`
		}
		if cached, ok := scopes[table]; ok {
			return cached
		}
		seen[table] = true
		var parts []string
		for _, e := range edges {
			if e.child != table || !inSet[e.parent] || seen[e.parent] {
				continue
			}
			parts = append(parts, "("+quoteIdentList(e.childCols)+") IN (SELECT "+quoteIdentList(e.parentCols)+
				" FROM "+pq.QuoteIdentifier(e.parent)+" WHERE "+scopeOf(e.parent, seen)+")")
		}
		seen[table] = false
		scope := strings.Join(parts, " OR ")
		scopes[table] = scope
		return scope
	}
	var steps []forceDeleteStep
	for table := range inSet {
		if table == "devices" {
			continue
		}
		steps = append(steps, forceDeleteStep{table: table, scope: scopeOf(table, map[string]bool{})})
	}
	sort.Slice(steps, func(i, j int) bool {
		if depth[steps[i].table] != depth[steps[j].table] {
			return depth[steps[i].table] > depth[steps[j].table]
		}
		return steps[i].table < steps[j].table
	})
	return steps
}

func quoteIdentList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = pq.QuoteIdentifier(c)
	}
	return strings.Join(quoted, ",")
}

// describeDBError renders a failure for the server log with everything Postgres reports: the
// SQLSTATE, table, column, constraint and detail. The response to the browser stays generic.
func describeDBError(err error) string {
	var pg *pq.Error
	if errors.As(err, &pg) {
		return fmt.Sprintf("%v [sqlstate=%s table=%q column=%q constraint=%q detail=%q hint=%q where=%q]",
			err, pg.Code, pg.Table, pg.Column, pg.Constraint, pg.Detail, pg.Hint, pg.Where)
	}
	return err.Error()
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
			log.Printf("router force delete: begin failed for %s: %s", deviceID, describeDBError(err))
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`SET LOCAL lock_timeout='3s'`); err != nil {
			log.Printf("router force delete: lock_timeout failed for %s: %s", deviceID, describeDBError(err))
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		existing, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			log.Printf("router force delete: loading %s failed: %s", deviceID, describeDBError(err))
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

		steps, err := forceDeleteSteps(tx)
		if err != nil {
			log.Printf("router force delete: reading foreign keys failed for %s: %s", deviceID, describeDBError(err))
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		for _, step := range steps {
			// Table and column names come from the catalog and are quoted, never from the request.
			res, err := tx.Exec(`DELETE FROM `+pq.QuoteIdentifier(step.table)+` WHERE `+step.scope, deviceID, businessID)
			if err != nil {
				log.Printf("router force delete: clearing %s for %s failed: %s", step.table, deviceID, describeDBError(err))
				return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
			}
			if n, _ := res.RowsAffected(); n > 0 {
				removed[step.table] = n
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
			log.Printf("router force delete: audit failed for %s: %s", deviceID, describeDBError(err))
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}

		if err := h.deviceRepo.DeleteDeviceTXN(tx, deviceID); err != nil {
			var pg *pq.Error
			switch {
			case errors.As(err, &pg) && pg.Code == "55000" && pg.Message == "management_migration_pending":
				return &httpError{status: http.StatusConflict, msg: "Recover or cancel this router's management migration before deleting its record"}
			case errors.As(err, &pg) && pg.Code == "23503":
				log.Printf("router force delete: %s is still referenced: %s", deviceID, describeDBError(err))
				return &httpError{status: http.StatusConflict, msg: "This record is still linked to other history, so nothing was deleted."}
			default:
				log.Printf("router force delete: delete failed for %s: %s", deviceID, describeDBError(err))
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
		if err := tx.Commit(); err != nil {
			log.Printf("router force delete: commit failed for %s: %s", deviceID, describeDBError(err))
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		return nil
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
		var he *httpError
		if !errors.As(err, &he) {
			log.Printf("router force delete: unexpected error for %s: %s", deviceID, describeDBError(err))
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
