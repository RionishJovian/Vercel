package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/broadband"
	"mega-magic/internal/database"
	"mega-magic/internal/secretbox"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type broadbandRouter interface {
	ApplyBroadband(context.Context, string, string, broadband.Spec) (broadband.Observation, error)
	ReadBroadband(context.Context, string, string, string, string) (broadband.Observation, error)
}

func (h *SecurityHandler) EnableBroadbandSecrets(box *secretbox.Box) { h.broadbandBox = box }

type broadbandInput struct {
	OperationID string `json:"operation_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Profile     string `json:"profile"`
	PeriodDays  int    `json:"period_days"`
	AmountMMK   int64  `json:"amount_mmk"`
	Confirm     bool   `json:"confirm"`
}

// broadbandWritesEnabled keeps paid PPPoE changes off by default. These scripts
// run unattended on customer routers and can disconnect paying subscribers, so
// they have their own switch, set only after a real router trial has passed
// (docs/BROADBAND-PHYSICAL-TRIAL-20261104.md). The general router-write switch
// must be on as well. Reading saved records and synchronizing stay available.
func broadbandWritesEnabled() bool {
	return os.Getenv("MM_NETWORK_CONTROLS_ENABLED") == "true" && os.Getenv("MM_BROADBAND_WRITES_ENABLED") == "true"
}

// Broadband handles only explicit paid-account operations. No login, observation
// or Store purchase can create an implicit payment or entitlement here.
func (h *SecurityHandler) Broadband(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, device := auth.GetBusinessID(c), c.Param("id")
	if biz == "" {
		c.Status(401)
		return
	}
	if !broadband.UUID.MatchString(device) {
		c.Status(404)
		return
	}
	command := c.Param("command")
	if command == "" {
		command = "accounts"
	}
	var in broadbandInput
	if c.Request.Method == http.MethodPost && command != "sync" {
		if !broadbandWritesEnabled() {
			c.JSON(503, gin.H{"error": "Paid PPPoE changes are switched off until they pass a real router trial", "code": "broadband_writes_off"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		if c.ShouldBindJSON(&in) != nil || !in.Confirm {
			c.JSON(400, gin.H{"error": "Review and confirm this subscriber change"})
			return
		}
	}
	if h.db == nil {
		c.JSON(503, gin.H{"error": "Broadband storage unavailable"})
		return
	}
	if !h.deviceExists(c, biz, device) {
		return
	}
	if command == "accounts" && c.Request.Method == http.MethodGet {
		var accounts []database.BroadbandAccount
		e := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
			var e error
			accounts, e = database.BroadbandRepository{}.ListTXN(tx, biz, device)
			return e
		})
		if e != nil {
			c.JSON(503, gin.H{"error": "Broadband schema is not available; existing services are unchanged"})
			return
		}
		c.JSON(200, gin.H{"accounts": accounts, "write_enabled": broadbandWritesEnabled()})
		return
	}
	controller, ok := h.quickConfig.(broadbandRouter)
	if !ok {
		c.JSON(503, gin.H{"error": "Connect Magic Hub management before managing broadband accounts"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Second)
	defer cancel()
	if command == "sync" && c.Request.Method == http.MethodPost {
		n, e := h.SyncBroadbandRouter(ctx, biz, device)
		if e != nil {
			c.JSON(502, gin.H{"error": "Some router observations could not be synchronized; retained evidence is unchanged for those accounts", "synchronized": n})
			return
		}
		c.JSON(200, gin.H{"synchronized": n})
		return
	}
	if c.Request.Method != http.MethodPost || h.broadbandBox == nil {
		c.JSON(503, gin.H{"error": "Broadband provisioning unavailable"})
		return
	}
	repo := database.BroadbandRepository{}
	var operation database.BroadbandOperation
	e := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		if command == "retry" {
			if !broadband.UUID.MatchString(c.Param("operationId")) {
				return broadband.ErrState
			}
			var e error
			operation, e = repo.OperationTXN(tx, biz, device, c.Param("operationId"))
			return e
		}
		if !broadband.UUID.MatchString(in.OperationID) || in.AmountMMK < 0 || in.AmountMMK > 1000000000 {
			return broadband.ErrState
		}
		action, account := command, c.Param("accountId")
		if command == "accounts" {
			action = "create"
			account = in.OperationID
		}
		if !broadband.UUID.MatchString(account) {
			return broadband.ErrState
		}
		if action == "create" || action == "renew" {
			if !broadband.ValidPeriod(int64(in.PeriodDays) * 86400) {
				return broadband.ErrState
			}
		} else if action != "suspend" && action != "resume" {
			return broadband.ErrState
		} else if in.AmountMMK != 0 || in.PeriodDays != 0 {
			return broadband.ErrState
		}
		if action == "create" {
			if !broadbandName(in.Username) || !broadband.ValidProfileName(in.Profile) || len(in.Password) < 10 || len(in.Password) > 128 || strings.ContainsAny(in.Password, "\r\n\x00") {
				return broadband.ErrState
			}
			if _, e := tx.Exec(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES($1,$2,$3,$4,$5) ON CONFLICT(business_id,id) DO NOTHING`, account, biz, device, in.Username, in.Profile); e != nil {
				return e
			}
		}
		// Serialize before reading current state or constructing the encrypted
		// payload. A second request must use the winning persisted operation.
		var lockedAccount string
		if e := tx.QueryRow(`SELECT id::text FROM broadband_accounts WHERE business_id=$1 AND device_id=$2 AND id=$3 AND NOT retired FOR UPDATE`, biz, device, account).Scan(&lockedAccount); e != nil {
			return e
		}
		accounts, e := repo.ListTXN(tx, biz, device)
		if e != nil {
			return e
		}
		var chosen *database.BroadbandAccount
		for i := range accounts {
			if accounts[i].ID == account {
				chosen = &accounts[i]
				break
			}
		}
		if chosen == nil {
			return sql.ErrNoRows
		}
		if action == "create" && (chosen.Username != in.Username || chosen.Profile != in.Profile) {
			return database.ErrBroadbandConflict
		}
		// Exclude passwords from the request digest: storing a fast password hash
		// would provide an unnecessary offline password-guessing oracle in DB dumps.
		hashIn := in
		hashIn.Password = ""
		hashIn.Confirm = false
		raw, _ := json.Marshal(hashIn)
		sum := sha256.Sum256(append([]byte(action+":"+account+":"), raw...))
		hash := hex.EncodeToString(sum[:])
		existing, e := repo.OperationTXN(tx, biz, device, in.OperationID)
		if e == nil {
			if existing.RequestHash != hash || existing.AccountID != account || existing.Kind != action {
				return database.ErrBroadbandConflict
			}
			if action == "create" {
				sealed, e := h.broadbandBox.Open(existing.Payload)
				if e != nil {
					return e
				}
				var prior broadband.Spec
				if json.Unmarshal([]byte(sealed), &prior) != nil || subtle.ConstantTimeCompare([]byte(prior.Password), []byte(in.Password)) != 1 {
					return database.ErrBroadbandConflict
				}
			}
			operation = existing
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if action != "create" && chosen.RouterOperation == "" {
			return database.ErrBroadbandConflict
		}
		spec := broadband.Spec{AccountID: account, OperationID: in.OperationID, ExpectedOperation: chosen.RouterOperation, Action: action, Username: chosen.Username, Profile: chosen.Profile, PeriodSeconds: int64(in.PeriodDays) * 86400, Password: in.Password}
		raw, e = json.Marshal(spec)
		if e != nil {
			return e
		}
		sealed, e := h.broadbandBox.Seal(string(raw))
		if e != nil {
			return e
		}
		operation = database.BroadbandOperation{ID: in.OperationID, AccountID: account, Kind: action, AmountMMK: in.AmountMMK, PeriodDays: in.PeriodDays, RequestHash: hash, Payload: sealed}
		return repo.ReserveTXN(tx, biz, device, operation)
	})
	if e != nil {
		c.JSON(409, gin.H{"error": "Subscriber request conflicts with existing state, is invalid, or is pending recovery; refresh before retrying"})
		return
	}
	if operation.State == "applied" {
		c.JSON(200, gin.H{"operation": operation})
		return
	}
	if operation.State == "rejected" {
		c.JSON(422, gin.H{"error": "Subscriber was not installed; choose another username or plan", "rejected": true})
		return
	}
	raw, e := h.broadbandBox.Open(operation.Payload)
	if e != nil {
		c.JSON(503, gin.H{"error": "Saved subscriber operation cannot be decrypted"})
		return
	}
	var spec broadband.Spec
	if json.Unmarshal([]byte(raw), &spec) != nil {
		c.Status(503)
		return
	}
	observed, e := controller.ApplyBroadband(ctx, biz, device, spec)
	if errors.Is(e, broadband.ErrRejected) && operation.Kind == "create" {
		if rejectErr := runInTenant(c, h.db, biz, func(tx *sql.Tx) error { return repo.RejectCreateTXN(tx, biz, device, operation.ID) }); rejectErr == nil {
			c.JSON(422, gin.H{"error": "Subscriber was not installed; choose another username or PPP plan", "rejected": true})
			return
		}
	}
	if e == nil {
		e = runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
			return recordBroadbandObservation(tx, biz, device, spec.AccountID, observed)
		})
	}
	if e != nil {
		c.JSON(502, gin.H{"error": "Router application is not confirmed. Use Retry saved operation; do not create a replacement payment", "operation_id": operation.ID, "pending": true})
		return
	}
	operation.State = "applied"
	c.JSON(200, gin.H{"operation": operation, "observation": observed})
}
func broadbandName(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !(alnum || i > 0 && strings.ContainsRune("_.:-", r)) {
			return false
		}
	}
	return true
}

// SyncBroadbandRouter is read-only on the router. Failures preserve prior
// observations; duplicate markers acknowledge an operation only once.
func (h *SecurityHandler) SyncBroadbandRouter(ctx context.Context, biz, device string) (int, error) {
	controller, ok := h.quickConfig.(broadbandRouter)
	if !ok {
		return 0, errors.New("broadband reader unavailable")
	}
	repo := database.BroadbandRepository{}
	var accounts []database.BroadbandAccount
	tx, e := h.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	e = database.WithTenant(tx, biz, func() error { var e error; accounts, e = repo.ListTXN(tx, biz, device); return e })
	if e != nil {
		return 0, e
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	oldestObservedFirst(accounts)
	n := 0
	var failures []error
	for _, a := range accounts {
		if e = ctx.Err(); e != nil {
			return n, e
		}
		observed, err := controller.ReadBroadband(ctx, biz, device, a.ID, a.Username)
		if err == nil {
			var t *sql.Tx
			t, err = h.db.BeginTx(ctx, nil)
			if err == nil {
				err = database.WithTenant(t, biz, func() error {
					return recordBroadbandObservation(t, biz, device, a.ID, observed)
				})
				if err == nil {
					err = t.Commit()
				}
				_ = t.Rollback()
			}
		}
		if err != nil {
			failures = append(failures, errors.New("subscriber observation unavailable"))
		} else {
			n++
		}
	}
	return n, errors.Join(failures...)
}

// The caller's time budget can end mid-list. Oldest reading first, so a router with many
// subscribers still reaches all of them over successive passes.
func oldestObservedFirst(accounts []database.BroadbandAccount) {
	sort.SliceStable(accounts, func(i, j int) bool {
		a, b := accounts[i].ObservedAt, accounts[j].ObservedAt
		return a == nil && b != nil || a != nil && b != nil && a.Before(*b)
	})
}

func recordBroadbandObservation(tx *sql.Tx, biz, device, account string, o broadband.Observation) error {
	repo := database.BroadbandRepository{}
	at := time.Unix(o.ObservedAt, 0).UTC()
	if e := repo.ObserveTXN(tx, biz, device, account, o.OperationID, o.State, at); e != nil {
		return e
	}
	if o.UsageAvailable {
		// ObserveTXN deliberately ignores delayed/superseded operation reads.
		// Their traffic samples must not replace the winning operation's sample.
		var current bool
		if e := tx.QueryRow(`SELECT router_operation=$4 AND observed_at=$5 FROM broadband_accounts WHERE business_id=$1 AND device_id=$2 AND id=$3`, biz, device, account, o.OperationID, at).Scan(&current); e != nil {
			return e
		}
		if !current {
			return nil
		}
		return repo.ObserveSessionTXN(tx, biz, device, account, o.Session, at)
	}
	return nil
}

func (h *SecurityHandler) BroadbandReport(c *gin.Context) {
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.Status(401)
		return
	}
	c.Header("Cache-Control", "no-store")
	var data json.RawMessage
	e := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT jsonb_build_object('source','pppoe-recorded-payments','amount_mmk',COALESCE(sum(amount_mmk),0),
 'payment_count',count(*),'today_mmk',COALESCE(sum(amount_mmk) FILTER(WHERE (created_at AT TIME ZONE 'Asia/Yangon')::date=(now() AT TIME ZONE 'Asia/Yangon')::date),0),
 'pending_count',(SELECT count(*) FROM broadband_operations WHERE business_id=$1 AND state='pending' AND kind IN ('create','renew')))
 FROM broadband_operations WHERE business_id=$1 AND state='applied' AND kind IN ('create','renew')`, biz).Scan(&data)
	})
	if e != nil {
		c.JSON(503, gin.H{"error": "Broadband report unavailable"})
		return
	}
	c.Data(200, "application/json", data)
}
