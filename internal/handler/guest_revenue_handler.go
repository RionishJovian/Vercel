package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

// GuestRevenue aggregates the entire tenant ledger, not the paginated inbox.
// Legacy billing purchases grant business entitlements and are deliberately
// excluded from hotspot revenue. Approval is not bank settlement verification.
func (h *GuestWifiHandler) GuestRevenue(c *gin.Context) {
	var result json.RawMessage
	err := h.withTenantTx(c, auth.GetBusinessID(c), func(tx *sql.Tx) error {
		return tx.QueryRow(guestRevenueSQL, auth.GetBusinessID(c)).Scan(&result)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "guest revenue unavailable"})
		return
	}
	c.Data(http.StatusOK, "application/json", result)
}

const guestRevenueSQL = `WITH ledger AS (
 SELECT plan_key, amount, status,
  (COALESCE(decided_at,created_at) AT TIME ZONE 'Asia/Yangon')::date AS sale_day
 FROM guest_purchase_requests WHERE business_id=$1
), clock AS (SELECT (now() AT TIME ZONE 'Asia/Yangon')::date AS today),
days AS (
 SELECT d::date AS day, COALESCE(SUM(l.amount),0) AS amount
 FROM clock CROSS JOIN generate_series(today-6,today,interval '1 day') AS d
 LEFT JOIN ledger l ON l.status='approved' AND l.sale_day=d::date
 GROUP BY d
), plans AS (
 SELECT plan_key AS key, COUNT(*) AS requests,
 COUNT(*) FILTER(WHERE status='approved') AS approved,
 COALESCE(SUM(amount) FILTER(WHERE status='approved'),0) AS revenue
 FROM ledger GROUP BY plan_key
)
SELECT jsonb_build_object(
 'timezone','Asia/Yangon','source','guest-wifi','settlement_verified',false,
 'all_time',COALESCE(SUM(amount) FILTER(WHERE status='approved'),0),
 'today',COALESCE(SUM(amount) FILTER(WHERE status='approved' AND sale_day=(SELECT today FROM clock)),0),
 'last7',COALESCE(SUM(amount) FILTER(WHERE status='approved' AND sale_day BETWEEN (SELECT today-6 FROM clock) AND (SELECT today FROM clock)),0),
 'last30',COALESCE(SUM(amount) FILTER(WHERE status='approved' AND sale_day BETWEEN (SELECT today-29 FROM clock) AND (SELECT today FROM clock)),0),
 'total',COUNT(*),'approved',COUNT(*) FILTER(WHERE status='approved'),
 'pending',COUNT(*) FILTER(WHERE status='pending'),'rejected',COUNT(*) FILTER(WHERE status='rejected'),
 'days',COALESCE((SELECT jsonb_agg(jsonb_build_object('key',day::text,'amount',amount) ORDER BY day) FROM days),'[]'::jsonb),
 'plans',COALESCE((SELECT jsonb_agg(to_jsonb(plans) ORDER BY revenue DESC,key) FROM plans),'[]'::jsonb)
) FROM ledger`
