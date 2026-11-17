package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Gateway capacity (owner decision 2026-11-13): one gateway is included with a
// client account; approved Store router-slot purchases and explicit platform
// grants add more gateways to the same account and site. The database function
// business_gateway_capacity() and the devices trigger are the authority; this
// file only reads them and records fulfilment.

var (
	// ErrPurchaseNotFulfillable is returned for a request that is not an approved
	// router-slot purchase.
	ErrPurchaseNotFulfillable = errors.New("purchase is not an approved router-slot request")
	// ErrPurchaseNotFound is returned when the purchase request does not exist.
	ErrPurchaseNotFound = errors.New("purchase request not found")
)

// GatewayGrant is one row of the append-only capacity ledger.
type GatewayGrant struct {
	ID             string     `json:"id"`
	Units          int        `json:"units"`
	Source         string     `json:"source"`
	StoreRequestID *string    `json:"store_request_id,omitempty"`
	AddonKey       *string    `json:"addon_key,omitempty"`
	ProductName    *string    `json:"product_name,omitempty"`
	Period         *string    `json:"period,omitempty"`
	PriceMMK       *int       `json:"price_mmk,omitempty"`
	TermPolicy     string     `json:"term_policy"`
	Reason         string     `json:"reason,omitempty"`
	GrantedAt      time.Time  `json:"granted_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
}

// RouterSlotPurchase is a Store router-slot request with its fulfilment state.
// Payment submission, approval and fulfilment are reported separately.
type RouterSlotPurchase struct {
	RequestID   string     `json:"request_id"`
	AddonKey    string     `json:"addon_key"`
	NameEN      string     `json:"name_en"`
	NameMY      string     `json:"name_my"`
	PriceMMK    int        `json:"price_mmk"`
	Period      string     `json:"period"`
	Status      string     `json:"status"`      // pending | approved | rejected (payment review)
	Fulfillment string     `json:"fulfillment"` // not_applicable | awaiting | fulfilled
	SubmittedAt time.Time  `json:"submitted_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
	GrantID     *string    `json:"grant_id,omitempty"`
}

// GatewayCapacity is the effective gateway allowance of one business.
type GatewayCapacity struct {
	Included   int                  `json:"included"`
	Legacy     int                  `json:"legacy_router_grants"`
	Purchased  int                  `json:"purchased"`
	Granted    int                  `json:"operator_granted"`
	Capacity   int                  `json:"capacity"`
	Used       int                  `json:"used"`
	Available  int                  `json:"available"`
	Exempt     bool                 `json:"exempt"`
	Over       bool                 `json:"over_capacity"`
	TermPolicy string               `json:"term_policy"`
	Grants     []GatewayGrant       `json:"grants"`
	Purchases  []RouterSlotPurchase `json:"purchases"`
}

// GatewayCapacityTXN reads the effective capacity, usage, ledger and router-slot
// purchases for a business. Run it inside the caller's tenant transaction.
func GatewayCapacityTXN(tx *sql.Tx, businessID string) (*GatewayCapacity, error) {
	g := &GatewayCapacity{Included: 1, TermPolicy: "not_enforced", Grants: []GatewayGrant{}, Purchases: []RouterSlotPurchase{}}
	if err := tx.QueryRow(`SELECT
		 business_gateway_capacity($1),
		 COALESCE((SELECT GREATEST(routers-1,0) FROM device_allowances WHERE business_id=$1),0),
		 (SELECT count(*) FROM devices WHERE business_id=$1 AND deployment_layer='MAIN_GATEWAY'),
		 EXISTS(SELECT 1 FROM users WHERE business_id=$1 AND is_active AND user_role='DEVELOPER')`, businessID).
		Scan(&g.Capacity, &g.Legacy, &g.Used, &g.Exempt); err != nil {
		return nil, fmt.Errorf("gateway capacity: %w", err)
	}
	rows, err := tx.Query(`SELECT id,units,source,store_request_id,addon_key,product_name_en,period,price_mmk,term_policy,reason,granted_at,revoked_at
		FROM gateway_capacity_grants WHERE business_id=$1 ORDER BY granted_at,id`, businessID)
	if err != nil {
		return nil, fmt.Errorf("gateway grants: %w", err)
	}
	for rows.Next() {
		var x GatewayGrant
		var req, key, name, period sql.NullString
		var price sql.NullInt64
		var revoked sql.NullTime
		if err := rows.Scan(&x.ID, &x.Units, &x.Source, &req, &key, &name, &period, &price, &x.TermPolicy, &x.Reason, &x.GrantedAt, &revoked); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan gateway grant: %w", err)
		}
		x.StoreRequestID, x.AddonKey, x.ProductName, x.Period = nullable(req), nullable(key), nullable(name), nullable(period)
		if price.Valid {
			p := int(price.Int64)
			x.PriceMMK = &p
		}
		if revoked.Valid {
			x.RevokedAt = &revoked.Time
		} else if x.Source == "store_purchase" {
			g.Purchased += x.Units
		} else {
			g.Granted += x.Units
		}
		g.Grants = append(g.Grants, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	g.Purchases, err = routerSlotPurchasesTXN(tx, `r.business_id=$1`, businessID)
	if err != nil {
		return nil, err
	}
	g.Available = g.Capacity - g.Used
	if g.Available < 0 {
		g.Over, g.Available = true, 0
	}
	return g, nil
}

func nullable(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func routerSlotPurchasesTXN(tx *sql.Tx, where string, args ...any) ([]RouterSlotPurchase, error) {
	rows, err := tx.Query(`SELECT r.id,a.addon_key,a.name_en,a.name_my,r.price_mmk,r.period,r.status,r.created_at,r.decided_at,g.id
		FROM store_addon_requests r JOIN store_addons a ON a.id=r.addon_id
		LEFT JOIN gateway_capacity_grants g ON g.store_request_id=r.id
		WHERE a.kind='router_slot' AND `+where+` ORDER BY r.created_at DESC LIMIT 50`, args...)
	if err != nil {
		return nil, fmt.Errorf("router-slot purchases: %w", err)
	}
	defer rows.Close()
	out := []RouterSlotPurchase{}
	for rows.Next() {
		var p RouterSlotPurchase
		var decided sql.NullTime
		var grant sql.NullString
		if err := rows.Scan(&p.RequestID, &p.AddonKey, &p.NameEN, &p.NameMY, &p.PriceMMK, &p.Period, &p.Status, &p.SubmittedAt, &decided, &grant); err != nil {
			return nil, fmt.Errorf("scan router-slot purchase: %w", err)
		}
		if decided.Valid {
			p.DecidedAt = &decided.Time
		}
		p.GrantID = nullable(grant)
		switch {
		case p.Status != "approved":
			p.Fulfillment = "not_applicable"
		case p.GrantID != nil:
			p.Fulfillment = "fulfilled"
		default:
			p.Fulfillment = "awaiting"
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FulfillRouterSlotTXN adds one gateway unit for an approved router-slot
// purchase, exactly once. A repeated call returns the existing grant with
// created=false. Run it in a platform (non-tenant) transaction.
func FulfillRouterSlotTXN(tx *sql.Tx, requestID, actor string) (grantID string, created bool, err error) {
	var biz, status, kind string
	err = tx.QueryRow(`SELECT r.business_id,r.status,a.kind FROM store_addon_requests r JOIN store_addons a ON a.id=r.addon_id WHERE r.id=$1 FOR UPDATE OF r`, requestID).Scan(&biz, &status, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrPurchaseNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("lock purchase: %w", err)
	}
	if status != "approved" || kind != "router_slot" {
		return "", false, ErrPurchaseNotFulfillable
	}
	err = tx.QueryRow(`INSERT INTO gateway_capacity_grants(business_id,units,source,store_request_id,granted_by,reason)
		VALUES($1,1,'store_purchase',$2,NULLIF($3,'')::uuid,'Store router-slot purchase')
		ON CONFLICT (store_request_id) DO NOTHING RETURNING id`, biz, requestID, actor).Scan(&grantID)
	if err == nil {
		return grantID, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("record gateway grant: %w", err)
	}
	if err = tx.QueryRow(`SELECT id FROM gateway_capacity_grants WHERE store_request_id=$1`, requestID).Scan(&grantID); err != nil {
		return "", false, fmt.Errorf("read gateway grant: %w", err)
	}
	return grantID, false, nil
}

// GatewayEnrollmentAllowedTXN reports whether a gateway may start a NEW Hub
// enrollment: it must be within the business's gateway capacity, counted in
// registration order. Existing saved connections are never revoked by this check.
func GatewayEnrollmentAllowedTXN(tx *sql.Tx, businessID, deviceID string) (bool, error) {
	var allowed bool
	err := tx.QueryRow(`WITH ranked AS (
		  SELECT device_id, row_number() OVER (ORDER BY created_at, device_id) AS n
		    FROM devices WHERE business_id=$1 AND deployment_layer='MAIN_GATEWAY')
		SELECT EXISTS(SELECT 1 FROM users WHERE business_id=$1 AND is_active AND user_role='DEVELOPER')
		    OR COALESCE((SELECT n FROM ranked WHERE device_id=$2),0) BETWEEN 1 AND business_gateway_capacity($1)`, businessID, deviceID).Scan(&allowed)
	return allowed, err
}
