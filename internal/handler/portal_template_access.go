package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

// Captive-portal login page templates. Two are free for every account; the other seven are Store add-ons, sold one by
// one (addon_key "portal_theme_<template id>") or as one bundle. An approved Store request is the entitlement, so the
// existing bank-reference and approval flow is the only purchase path. Platform administrators (DEVELOPER) are exempt.
const portalBundleAddonKey = "portal_themes_bundle"

var portalFreeTemplates = map[string]bool{"starlink_mm": true, "daylight": true}

var errPortalTemplateLocked = errors.New("this login page theme is a Store add-on")

func portalThemeAddonKey(id string) string { return "portal_theme_" + id }

// portalTemplatePremium is true for a known template that needs the add-on. The empty id (a portal saved before
// templates existed) keeps its original look for free and is never premium.
func portalTemplatePremium(id string) bool {
	return id != "" && routeros.ValidPortalTemplate(id) && !portalFreeTemplates[id]
}

// portalTemplateAllowed is the single entitlement rule. owned holds the approved add-on keys of the business.
func portalTemplateAllowed(id, role string, owned map[string]bool) bool {
	if !portalTemplatePremium(id) {
		return true
	}
	if role == string(model.RoleDeveloper) {
		return true
	}
	return owned[portalBundleAddonKey] || owned[portalThemeAddonKey(id)]
}

// ownedPortalAddons returns the approved theme add-on keys of one business (tenant transaction required).
func ownedPortalAddons(tx *sql.Tx, businessID string) (map[string]bool, error) {
	rows, err := tx.Query(`SELECT DISTINCT a.addon_key FROM store_addon_requests r JOIN store_addons a ON a.id=r.addon_id
		WHERE r.business_id=$1 AND r.status='approved' AND (a.addon_key=$2 OR left(a.addon_key,13)='portal_theme_')`, businessID, portalBundleAddonKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		owned[k] = true
	}
	return owned, rows.Err()
}

// TemplateAccess: GET /api/v1/portal/template-access. Tells the editor which templates this account may save.
func (h *PortalHandler) TemplateAccess(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	role := auth.GetUserRole(c)
	var owned map[string]bool
	if err := h.withTenantTx(c, businessID, func(tx *sql.Tx) (e error) { owned, e = ownedPortalAddons(tx, businessID); return }); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "template access unavailable"})
		return
	}
	unlocked := []string{}
	for _, t := range routeros.PortalTemplates() {
		if portalTemplateAllowed(t.ID, role, owned) {
			unlocked = append(unlocked, t.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"exempt": role == string(model.RoleDeveloper), "bundle": owned[portalBundleAddonKey], "unlocked": unlocked})
}
