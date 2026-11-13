package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// StoreAddonHandler serves the Store add-on catalog (platform managed) and the
// purchase requests RouterBOARD owners raise against it. Payment is a bank
// transfer reference reviewed by the platform operator, as with app passes.
// Approving a router-slot request also grants one gateway unit in the same
// transaction (gateway_capacity_grants); other add-on kinds still record only
// the decision and are applied by the operator with the existing tools.
type StoreAddonHandler struct {
	db *database.DB
}

func NewStoreAddonHandler(db *database.DB) *StoreAddonHandler { return &StoreAddonHandler{db: db} }

// RegisterTenant adds the owner-facing routes to the signed-in business group.
func (h *StoreAddonHandler) RegisterTenant(g *gin.RouterGroup) {
	g.GET("/store/addons", h.List)
	g.POST("/store/addons/:id/requests", auth.RequireMinRole(model.RoleAdmin), h.CreateRequest)
}

// RegisterPlatform adds the platform-operator routes (the group must already
// require the DEVELOPER role).
func (h *StoreAddonHandler) RegisterPlatform(g *gin.RouterGroup) {
	g.GET("/store/addons", h.PlatformList)
	g.POST("/store/addons", h.PlatformCreate)
	g.PUT("/store/addons/:id", h.PlatformUpdate)
	g.GET("/store/requests", h.PlatformRequests)
	g.POST("/store/requests/:id/decision", h.PlatformDecide)
	g.POST("/store/requests/:id/fulfill", h.PlatformFulfill)
}

type storeAddon struct {
	ID            string `json:"id"`
	Key           string `json:"addon_key"`
	Kind          string `json:"kind"`
	NameEN        string `json:"name_en"`
	NameMY        string `json:"name_my"`
	DescriptionEN string `json:"description_en"`
	DescriptionMY string `json:"description_my"`
	PriceMMK      int    `json:"price_mmk"`
	Period        string `json:"period"`
	Active        bool   `json:"active"`
	SortOrder     int    `json:"sort_order"`
}

type storeAddonRequest struct {
	ID            string  `json:"id"`
	AddonID       string  `json:"addon_id"`
	AddonKey      string  `json:"addon_key"`
	NameEN        string  `json:"name_en"`
	NameMY        string  `json:"name_my"`
	BusinessID    string  `json:"business_id,omitempty"`
	Company       string  `json:"company_name,omitempty"`
	PriceMMK      int     `json:"price_mmk"`
	Period        string  `json:"period"`
	BankReference string  `json:"bank_reference"`
	Status        string  `json:"status"`
	DecisionNote  string  `json:"decision_note"`
	CreatedAt     string  `json:"created_at"`
	DecidedAt     *string `json:"decided_at,omitempty"`
	// Kind and Fulfillment keep payment approval apart from the capacity it
	// grants: a router slot is "fulfilled" only once its gateway grant exists.
	Kind        string `json:"kind"`
	Fulfillment string `json:"fulfillment"`
}

var (
	addonKeyPattern = regexp.MustCompile(`^[a-z0-9_]{3,40}$`)
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type storeAddonInput struct {
	Key           string `json:"addon_key"`
	Kind          string `json:"kind"`
	NameEN        string `json:"name_en"`
	NameMY        string `json:"name_my"`
	DescriptionEN string `json:"description_en"`
	DescriptionMY string `json:"description_my"`
	PriceMMK      *int   `json:"price_mmk"`
	Period        string `json:"period"`
	Active        *bool  `json:"active"`
	SortOrder     *int   `json:"sort_order"`
}

// validateStoreAddon checks a catalog item. It is pure so it can be unit tested.
func validateStoreAddon(in storeAddonInput) error {
	if !addonKeyPattern.MatchString(in.Key) {
		return errors.New("addon_key must be 3-40 characters: lower-case letters, digits and underscore")
	}
	switch in.Kind {
	case "router_slot", "server", "feature":
	default:
		return errors.New("kind must be router_slot, server or feature")
	}
	switch in.Period {
	case "one_time", "monthly", "annual":
	default:
		return errors.New("period must be one_time, monthly or annual")
	}
	for _, name := range []string{in.NameEN, in.NameMY} {
		if l := len([]rune(strings.TrimSpace(name))); l < 1 || l > 120 {
			return errors.New("names must be 1-120 characters in both languages")
		}
	}
	if len([]rune(in.DescriptionEN)) > 600 || len([]rune(in.DescriptionMY)) > 600 {
		return errors.New("descriptions must be at most 600 characters")
	}
	if in.PriceMMK == nil || *in.PriceMMK < 0 || *in.PriceMMK > 100000000 {
		return errors.New("price_mmk must be between 0 and 100,000,000")
	}
	return nil
}

func addonColumns(prefix string) string {
	cols := []string{"id", "addon_key", "kind", "name_en", "name_my", "description_en", "description_my", "price_mmk", "period", "active", "sort_order"}
	for i := range cols {
		cols[i] = prefix + cols[i]
	}
	return strings.Join(cols, ",")
}

func scanAddon(rows interface{ Scan(...any) error }) (storeAddon, error) {
	var a storeAddon
	err := rows.Scan(&a.ID, &a.Key, &a.Kind, &a.NameEN, &a.NameMY, &a.DescriptionEN, &a.DescriptionMY, &a.PriceMMK, &a.Period, &a.Active, &a.SortOrder)
	return a, err
}

const requestSelect = `SELECT r.id,r.addon_id,a.addon_key,a.name_en,a.name_my,r.business_id,COALESCE(b.company_name,''),r.price_mmk,r.period,r.bank_reference,r.status,r.decision_note,
 to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
 CASE WHEN r.decided_at IS NULL THEN NULL ELSE to_char(r.decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
 a.kind,CASE WHEN a.kind<>'router_slot' OR r.status<>'approved' THEN 'not_applicable' WHEN EXISTS(SELECT 1 FROM gateway_capacity_grants g WHERE g.store_request_id=r.id) THEN 'fulfilled' ELSE 'awaiting' END
 FROM store_addon_requests r JOIN store_addons a ON a.id=r.addon_id LEFT JOIN businesses b ON b.business_id=r.business_id `

// Tenant transactions run as the restricted role, which cannot read businesses,
// so owner-facing queries use this variant without that join.
const requestSelectTenant = `SELECT r.id,r.addon_id,a.addon_key,a.name_en,a.name_my,r.business_id,'',r.price_mmk,r.period,r.bank_reference,r.status,r.decision_note,
 to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
 CASE WHEN r.decided_at IS NULL THEN NULL ELSE to_char(r.decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
 a.kind,CASE WHEN a.kind<>'router_slot' OR r.status<>'approved' THEN 'not_applicable' WHEN EXISTS(SELECT 1 FROM gateway_capacity_grants g WHERE g.store_request_id=r.id) THEN 'fulfilled' ELSE 'awaiting' END
 FROM store_addon_requests r JOIN store_addons a ON a.id=r.addon_id `

func scanRequest(rows interface{ Scan(...any) error }, withCompany bool) (storeAddonRequest, error) {
	var r storeAddonRequest
	var decided sql.NullString
	err := rows.Scan(&r.ID, &r.AddonID, &r.AddonKey, &r.NameEN, &r.NameMY, &r.BusinessID, &r.Company, &r.PriceMMK, &r.Period, &r.BankReference, &r.Status, &r.DecisionNote, &r.CreatedAt, &decided, &r.Kind, &r.Fulfillment)
	if decided.Valid {
		s := decided.String
		r.DecidedAt = &s
	}
	if !withCompany {
		r.BusinessID, r.Company = "", ""
	}
	return r, err
}

// List returns the active catalog and this business's own requests.
func (h *StoreAddonHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	addons := []storeAddon{}
	requests := []storeAddonRequest{}
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT ` + addonColumns("") + ` FROM store_addons WHERE active ORDER BY sort_order,name_en`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAddon(rows)
			if err != nil {
				return err
			}
			addons = append(addons, a)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rr, err := tx.Query(requestSelectTenant+` WHERE r.business_id=$1 ORDER BY r.created_at DESC LIMIT 50`, biz)
		if err != nil {
			return err
		}
		defer rr.Close()
		for rr.Next() {
			r, err := scanRequest(rr, false)
			if err != nil {
				return err
			}
			requests = append(requests, r)
		}
		return rr.Err()
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store catalog unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"addons": addons, "requests": requests})
}

// CreateRequest records a purchase request for one add-on. It is idempotent on
// idempotency_key, and a business can have only one pending request per add-on.
func (h *StoreAddonHandler) CreateRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user := auth.GetBusinessID(c), auth.GetUserID(c)
	id := c.Param("id")
	var in struct {
		BankReference  string `json:"bank_reference"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if biz == "" || user == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !uuidPattern.MatchString(id) || c.ShouldBindJSON(&in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	in.BankReference = strings.TrimSpace(in.BankReference)
	if l := len([]rune(in.BankReference)); l < 4 || l > 80 || len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bank_reference must be 4-80 characters and idempotency_key 8-80"})
		return
	}
	var out storeAddonRequest
	status := http.StatusCreated
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		// An earlier request with this key is returned unchanged.
		var existingID, existingAddon string
		switch e := tx.QueryRow(`SELECT id,addon_id FROM store_addon_requests WHERE business_id=$1 AND idempotency_key=$2`, biz, in.IdempotencyKey).Scan(&existingID, &existingAddon); {
		case e == nil:
			if existingAddon != id {
				return &httpError{status: http.StatusConflict, msg: "idempotency_key was used for a different add-on"}
			}
			status = http.StatusOK
			r, e := scanRequest(tx.QueryRow(requestSelectTenant+` WHERE r.id=$1 AND r.business_id=$2`, existingID, biz), false)
			out = r
			return e
		case !errors.Is(e, sql.ErrNoRows):
			return e
		}
		var price int
		var period string
		if e := tx.QueryRow(`SELECT price_mmk,period FROM store_addons WHERE id=$1 AND active`, id).Scan(&price, &period); errors.Is(e, sql.ErrNoRows) {
			return &httpError{status: http.StatusNotFound, msg: "add-on not available"}
		} else if e != nil {
			return e
		}
		var pending bool
		if e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM store_addon_requests WHERE business_id=$1 AND addon_id=$2 AND status='pending')`, biz, id).Scan(&pending); e != nil {
			return e
		}
		if pending {
			return &httpError{status: http.StatusConflict, msg: "a request for this add-on is already waiting for review"}
		}
		var newID string
		if e := tx.QueryRow(`INSERT INTO store_addon_requests(business_id,user_id,addon_id,price_mmk,period,bank_reference,idempotency_key)
 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, biz, user, id, price, period, in.BankReference, in.IdempotencyKey).Scan(&newID); e != nil {
			return e
		}
		r, e := scanRequest(tx.QueryRow(requestSelectTenant+` WHERE r.id=$1 AND r.business_id=$2`, newID, biz), false)
		out = r
		return e
	})
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			c.JSON(he.status, gin.H{"error": he.msg})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store request could not be recorded"})
		return
	}
	c.JSON(status, gin.H{"request": out})
}

// ---- platform operator ----

func (h *StoreAddonHandler) PlatformList(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rows, err := h.db.Query(`SELECT ` + addonColumns("") + ` FROM store_addons ORDER BY sort_order,name_en`)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store catalog unavailable"})
		return
	}
	defer rows.Close()
	addons := []storeAddon{}
	for rows.Next() {
		a, err := scanAddon(rows)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store catalog unavailable"})
			return
		}
		addons = append(addons, a)
	}
	c.JSON(http.StatusOK, gin.H{"addons": addons})
}

func (h *StoreAddonHandler) PlatformCreate(c *gin.Context) {
	var in storeAddonInput
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := validateStoreAddon(in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	active, order := true, 100
	if in.Active != nil {
		active = *in.Active
	}
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	var exists bool
	if err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM store_addons WHERE addon_key=$1)`, in.Key).Scan(&exists); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store catalog unavailable"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "an add-on with this key already exists"})
		return
	}
	row := h.db.QueryRow(`INSERT INTO store_addons(addon_key,kind,name_en,name_my,description_en,description_my,price_mmk,period,active,sort_order)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+addonColumns(""),
		in.Key, in.Kind, strings.TrimSpace(in.NameEN), strings.TrimSpace(in.NameMY), in.DescriptionEN, in.DescriptionMY, *in.PriceMMK, in.Period, active, order)
	a, err := scanAddon(row)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "add-on could not be saved"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"addon": a})
}

func (h *StoreAddonHandler) PlatformUpdate(c *gin.Context) {
	id := c.Param("id")
	var in storeAddonInput
	if !uuidPattern.MatchString(id) || c.ShouldBindJSON(&in) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := validateStoreAddon(in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	active, order := true, 100
	if in.Active != nil {
		active = *in.Active
	}
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	row := h.db.QueryRow(`UPDATE store_addons SET addon_key=$2,kind=$3,name_en=$4,name_my=$5,description_en=$6,description_my=$7,price_mmk=$8,period=$9,active=$10,sort_order=$11,updated_at=clock_timestamp()
 WHERE id=$1 RETURNING `+addonColumns(""),
		id, in.Key, in.Kind, strings.TrimSpace(in.NameEN), strings.TrimSpace(in.NameMY), in.DescriptionEN, in.DescriptionMY, *in.PriceMMK, in.Period, active, order)
	a, err := scanAddon(row)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "add-on not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "add-on could not be saved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"addon": a})
}

func (h *StoreAddonHandler) PlatformRequests(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status := c.DefaultQuery("status", "pending")
	if status != "pending" && status != "approved" && status != "rejected" && status != "all" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be pending, approved, rejected or all"})
		return
	}
	query := requestSelect
	args := []any{}
	if status != "all" {
		query += ` WHERE r.status=$1`
		args = append(args, status)
	}
	rows, err := h.db.Query(query+` ORDER BY r.created_at DESC LIMIT 100`, args...)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store requests unavailable"})
		return
	}
	defer rows.Close()
	out := []storeAddonRequest{}
	for rows.Next() {
		r, err := scanRequest(rows, true)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "store requests unavailable"})
			return
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, gin.H{"requests": out})
}

func (h *StoreAddonHandler) PlatformDecide(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !uuidPattern.MatchString(id) || c.ShouldBindJSON(&in) != nil || (in.Decision != "approve" && in.Decision != "reject") || len([]rune(in.Note)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision must be approve or reject and note at most 500 characters"})
		return
	}
	status := "approved"
	if in.Decision == "reject" {
		status = "rejected"
	}
	// The decision and, for an approved router slot, its gateway grant commit
	// together: approval never leaves capacity to a separate manual step, and a
	// repeated approval cannot grant twice (the update is guarded on pending and
	// the grant is unique per purchase).
	actor := auth.GetUserID(c)
	err := func() error {
		tx, err := h.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var kind string
		err = tx.QueryRow(`UPDATE store_addon_requests r SET status=$2,decision_note=$3,decided_by=$4,decided_at=clock_timestamp()
			FROM store_addons a WHERE r.id=$1 AND r.status='pending' AND a.id=r.addon_id RETURNING a.kind`,
			id, status, strings.TrimSpace(in.Note), actor).Scan(&kind)
		if errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if e := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM store_addon_requests WHERE id=$1)`, id).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				return &httpError{status: http.StatusNotFound, msg: "request not found"}
			}
			return &httpError{status: http.StatusConflict, msg: "this request was already decided"}
		}
		if err != nil {
			return err
		}
		if status == "approved" && kind == "router_slot" {
			if _, _, err = database.FulfillRouterSlotTXN(tx, id, actor); err != nil {
				return err
			}
		}
		return tx.Commit()
	}()
	var he *httpError
	if errors.As(err, &he) {
		c.JSON(he.status, gin.H{"error": he.msg})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "decision could not be recorded; nothing was changed"})
		return
	}
	r, err := scanRequest(h.db.QueryRow(requestSelect+` WHERE r.id=$1`, id), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": status})
		return
	}
	c.JSON(http.StatusOK, gin.H{"request": r})
}

// PlatformFulfill grants the gateway unit of a router-slot purchase that was
// approved before automatic fulfilment existed (or whose fulfilment must be
// retried). It is idempotent: a second call returns the same grant. Operators
// should first confirm the capacity was not already raised by hand.
func (h *StoreAddonHandler) PlatformFulfill(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}
	var grant string
	var created bool
	err := func() error {
		tx, err := h.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if grant, created, err = database.FulfillRouterSlotTXN(tx, id, auth.GetUserID(c)); err != nil {
			return err
		}
		return tx.Commit()
	}()
	switch {
	case errors.Is(err, database.ErrPurchaseNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
	case errors.Is(err, database.ErrPurchaseNotFulfillable):
		c.JSON(http.StatusConflict, gin.H{"error": "Only an approved router-slot purchase adds a gateway", "code": "not_fulfillable"})
	case err != nil:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "fulfilment could not be recorded; nothing was changed"})
	default:
		c.JSON(http.StatusOK, gin.H{"grant_id": grant, "created": created, "fulfillment": "fulfilled"})
	}
}
