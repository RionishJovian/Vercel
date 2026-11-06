package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/discoverystore"
)

// DiscoveryStore is everything discovery needs. It contains no router access by design: a discovery handler
// cannot read, log in to or configure a router.
type DiscoveryStore interface {
	Create(ctx context.Context, bid, userID, siteID string) (discoverystore.Created, error)
	Get(ctx context.Context, bid, id string) (discoverystore.View, error)
	Cancel(ctx context.Context, bid, id string) error
	Pair(ctx context.Context, code string) (discoverystore.Paired, error)
	AddObservations(ctx context.Context, token, sessionID string, in []discoverystore.ObservationIn) (int, int, error)
	Select(ctx context.Context, bid, id, deviceKey string) (discoverystore.Selection, error)
	Link(ctx context.Context, bid, id, deviceKey, deviceID string) error
}

type DiscoveryHandler struct{ store DiscoveryStore }

func NewDiscoveryHandler(s DiscoveryStore) *DiscoveryHandler { return &DiscoveryHandler{store: s} }

const maxDiscoveryBody = 32 << 10

var (
	discoveryUUID    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	deviceKeyPattern = regexp.MustCompile(`^(sw:[0-9a-f]{16}|mac:[0-9A-F]{2}(:[0-9A-F]{2}){5})$`)
	macPattern       = regexp.MustCompile(`^[0-9A-F]{2}(:[0-9A-F]{2}){5}$`)
)

func discoveryUnavailable(c *gin.Context, status int) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"error": "Discovery session unavailable. Start a new one and try again."})
}

// decodeStrict reads one bounded JSON object and refuses unknown fields and trailing data.
func decodeStrict(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDiscoveryBody)
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.More() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return false
	}
	return true
}

func sessionID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !discoveryUUID.MatchString(id) {
		discoveryUnavailable(c, http.StatusNotFound)
		return "", false
	}
	return id, true
}

// CreateSession: Owner/Admin starts a session and receives the one-time pairing code.
func (h *DiscoveryHandler) CreateSession(c *gin.Context) {
	var body struct {
		SiteID string `json:"site_id"`
	}
	if c.Request.ContentLength != 0 && !decodeStrict(c, &body) {
		return
	}
	if body.SiteID != "" && !discoveryUUID.MatchString(body.SiteID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site"})
		return
	}
	created, err := h.store.Create(c.Request.Context(), auth.GetBusinessID(c), auth.GetUserID(c), body.SiteID)
	switch {
	case errors.Is(err, discoverystore.ErrLiveSession):
		c.JSON(http.StatusConflict, gin.H{"error": "A discovery session is already open. Cancel it or wait for it to end."})
	case errors.Is(err, discoverystore.ErrBadSite):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start discovery"})
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, created)
	}
}

func (h *DiscoveryHandler) GetSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	v, err := h.store.Get(c.Request.Context(), auth.GetBusinessID(c), id)
	if err != nil {
		discoveryUnavailable(c, http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, v)
}

func (h *DiscoveryHandler) CancelSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	if err := h.store.Cancel(c.Request.Context(), auth.GetBusinessID(c), id); err != nil {
		discoveryUnavailable(c, http.StatusNotFound)
		return
	}
	c.Status(http.StatusNoContent)
}

// Pair: the helper trades the code for a token. Its only identity is the code; a supplied business id is never read.
func (h *DiscoveryHandler) Pair(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"pairing_code"`
	}
	if !decodeStrict(c, &body) {
		return
	}
	p, err := h.store.Pair(c.Request.Context(), body.Code)
	if err != nil || p.SessionID != id {
		discoveryUnavailable(c, http.StatusUnauthorized)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, p)
}

type observationBody struct {
	DeviceKey  string   `json:"device_key"`
	MACs       []string `json:"macs"`
	Identity   string   `json:"identity"`
	Board      string   `json:"board"`
	Platform   string   `json:"platform"`
	Version    string   `json:"version"`
	Interface  string   `json:"interface"`
	ClaimedIP  string   `json:"claimed_ip"`
	SourceIP   string   `json:"source_ip"`
	IPMismatch bool     `json:"ip_mismatch"`
	IPChanged  bool     `json:"ip_changed"`
	HeardCount int      `json:"heard_count"`
	Status     string   `json:"status"`
}

func cleanText(s string) (string, bool) {
	if len(s) > 64 || !utf8.ValidString(s) || strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == ' ' || r == ' ' }) >= 0 {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func privateIPv4(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() || !a.IsPrivate() {
		return "", false
	}
	return a.String(), true
}

// validateObservation rejects, never repairs: the helper is treated as untrusted input.
func validateObservation(o observationBody) (discoverystore.ObservationIn, bool) {
	var out discoverystore.ObservationIn
	if !deviceKeyPattern.MatchString(o.DeviceKey) || len(o.MACs) < 1 || len(o.MACs) > 8 || o.HeardCount < 1 || o.HeardCount > 100000 {
		return out, false
	}
	if o.Status != "" && o.Status != "nearby_unverified" {
		return out, false // a helper may never claim a device is verified, connected or managed
	}
	for _, m := range o.MACs {
		if !macPattern.MatchString(m) {
			return out, false
		}
	}
	var ok [5]bool
	out.Identity, ok[0] = cleanText(o.Identity)
	out.Board, ok[1] = cleanText(o.Board)
	out.Platform, ok[2] = cleanText(o.Platform)
	out.Version, ok[3] = cleanText(o.Version)
	out.Interface, ok[4] = cleanText(o.Interface)
	for _, v := range ok {
		if !v {
			return out, false
		}
	}
	var good1, good2 bool
	out.ClaimedIP, good1 = privateIPv4(o.ClaimedIP)
	out.SourceIP, good2 = privateIPv4(o.SourceIP)
	if !good1 || !good2 {
		return out, false
	}
	out.DeviceKey, out.MACs, out.IPMismatch, out.IPChanged, out.HeardCount = o.DeviceKey, o.MACs, o.IPMismatch, o.IPChanged, o.HeardCount
	return out, true
}

// Observations: a paired helper uploads what it heard. Nothing here is ever treated as verified.
func (h *DiscoveryHandler) Observations(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	raw := c.GetHeader("Authorization")
	token, found := strings.CutPrefix(raw, "Bearer ")
	if !found {
		discoveryUnavailable(c, http.StatusUnauthorized)
		return
	}
	var body struct {
		Observations []observationBody `json:"observations"`
	}
	if !decodeStrict(c, &body) {
		return
	}
	if len(body.Observations) > discoverystore.MaxObservations {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many observations"})
		return
	}
	in := make([]discoverystore.ObservationIn, 0, len(body.Observations))
	for _, o := range body.Observations {
		v, ok := validateObservation(o)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid observation"})
			return
		}
		in = append(in, v)
	}
	accepted, dropped, err := h.store.AddObservations(c.Request.Context(), token, id, in)
	switch {
	case errors.Is(err, discoverystore.ErrTooFast):
		c.Header("Retry-After", "2")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "uploading too often"})
	case err != nil:
		discoveryUnavailable(c, http.StatusUnauthorized)
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"accepted": accepted, "dropped": dropped})
	}
}

// SelectCandidate records the owner's choice. It registers nothing: the console continues with the existing add-router flow.
func (h *DiscoveryHandler) SelectCandidate(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var body struct {
		DeviceKey string `json:"device_key"`
	}
	if !decodeStrict(c, &body) || !deviceKeyPattern.MatchString(body.DeviceKey) {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid candidate"})
		}
		return
	}
	sel, err := h.store.Select(c.Request.Context(), auth.GetBusinessID(c), id, body.DeviceKey)
	if err != nil {
		discoveryUnavailable(c, http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, sel)
}

// LinkRouter remembers which registered router a selected candidate became. The router itself is created by the existing flow.
func (h *DiscoveryHandler) LinkRouter(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var body struct {
		DeviceKey string `json:"device_key"`
		RouterID  string `json:"router_id"`
	}
	if !decodeStrict(c, &body) || !deviceKeyPattern.MatchString(body.DeviceKey) || !discoveryUUID.MatchString(body.RouterID) {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid link"})
		}
		return
	}
	switch err := h.store.Link(c.Request.Context(), auth.GetBusinessID(c), id, body.DeviceKey, body.RouterID); {
	case errors.Is(err, discoverystore.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "This device is already registered as another router."})
	case err != nil:
		discoveryUnavailable(c, http.StatusNotFound)
	default:
		c.Status(http.StatusNoContent)
	}
}
