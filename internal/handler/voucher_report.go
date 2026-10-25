package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

// VoucherReport describes recorded guest vouchers, not cash receipts. A durable
// first-use reservation precedes router activation; neither proves cash receipt
// or of a currently connected guest. Business subscription passes are excluded.
func (h *VoucherHandler) VoucherReport(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guest voucher report unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var result json.RawMessage
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, voucherReportSQL, businessID).Scan(&result)
	})
	if err != nil {
		// In particular, a deployment without the guest-grant schema must not
		// look like an empty inventory or expose internal database diagnostics.
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "guest voucher report unavailable"})
		return
	}
	c.Data(http.StatusOK, "application/json", result)
}

const voucherReportSQL = `WITH inventory AS (
 SELECT v.device_id, COALESCE(d.hardware_name,'Registered gateway') AS gateway_name,
 v.price_mmk, g.first_used_at, g.activated_at,
 (g.activated_at AT TIME ZONE 'Asia/Yangon')::date AS activation_day,
 CASE
  WHEN v.status IN ('revoked','banned') OR g.state='revoked' THEN 'revoked'
  WHEN v.status='expired' OR g.state='expired' OR g.expires_at<=now() THEN 'expired'
  WHEN v.status='used' AND g.state='active' THEN 'activated'
  WHEN v.status='used' AND g.state='provisioning' THEN 'reserved'
  WHEN v.status IN ('unused','active') AND g.state='unused' THEN 'available'
  ELSE 'other'
 END AS state
 FROM guest_voucher_grants g
 JOIN vouchers v ON v.id=g.voucher_id AND v.business_id=g.business_id AND v.device_id=g.device_id
 LEFT JOIN devices d ON d.device_id=g.device_id AND d.business_id=g.business_id
 WHERE v.business_id=$1 AND g.business_id=$1 AND v.plan_key='guest-internet'
), clock AS (SELECT (now() AT TIME ZONE 'Asia/Yangon')::date AS today), gateways AS (
 SELECT device_id, gateway_name, COUNT(*) AS recorded_count,
 COALESCE(SUM(price_mmk),0) AS recorded_face_value,
 COUNT(*) FILTER(WHERE first_used_at IS NOT NULL) AS first_use_count,
 COUNT(*) FILTER(WHERE state='available') AS available_count,
 COUNT(*) FILTER(WHERE state='reserved') AS reserved_count,
 COUNT(*) FILTER(WHERE state='activated') AS activated_count,
 COUNT(*) FILTER(WHERE state='expired') AS expired_count,
 COUNT(*) FILTER(WHERE state='revoked') AS revoked_count,
 COUNT(*) FILTER(WHERE state='other') AS other_count,
 COUNT(*) FILTER(WHERE activated_at IS NOT NULL) AS activation_count,
 COALESCE(SUM(price_mmk) FILTER(WHERE activated_at IS NOT NULL),0) AS activation_value
 FROM inventory GROUP BY device_id,gateway_name
)
SELECT jsonb_build_object(
 'source','guest-internet-vouchers','as_of',now(),'timezone','Asia/Yangon',
 'cash_sales_recorded',false,'confirmed_sales_count',NULL,'cash_revenue_mmk',NULL,
 'recorded_count',COUNT(*),'recorded_face_value',COALESCE(SUM(price_mmk),0),
 'first_use_count',COUNT(*) FILTER(WHERE first_used_at IS NOT NULL),
 'available_count',COUNT(*) FILTER(WHERE state='available'),
 'reserved_count',COUNT(*) FILTER(WHERE state='reserved'),
 'activated_count',COUNT(*) FILTER(WHERE state='activated'),
 'expired_count',COUNT(*) FILTER(WHERE state='expired'),
 'revoked_count',COUNT(*) FILTER(WHERE state='revoked'),
 'other_count',COUNT(*) FILTER(WHERE state='other'),
 'activation_count',COUNT(*) FILTER(WHERE activated_at IS NOT NULL),
 'activation_time_unknown_count',COUNT(*) FILTER(WHERE first_used_at IS NOT NULL AND activated_at IS NULL),
 'activation_value',COALESCE(SUM(price_mmk) FILTER(WHERE activated_at IS NOT NULL),0),
 'activation_today',COALESCE(SUM(price_mmk) FILTER(WHERE activated_at IS NOT NULL AND activation_day=(SELECT today FROM clock)),0),
 'activation_last7',COALESCE(SUM(price_mmk) FILTER(WHERE activated_at IS NOT NULL AND activation_day BETWEEN (SELECT today-6 FROM clock) AND (SELECT today FROM clock)),0),
 'activation_last30',COALESCE(SUM(price_mmk) FILTER(WHERE activated_at IS NOT NULL AND activation_day BETWEEN (SELECT today-29 FROM clock) AND (SELECT today FROM clock)),0),
 'gateways',COALESCE((SELECT jsonb_agg(to_jsonb(gateways) ORDER BY gateway_name,device_id) FROM gateways),'[]'::jsonb)
) FROM inventory`
