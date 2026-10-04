package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
	"io"
	"mega-magic/internal/media/access"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/media/jellyfin"
	"mega-magic/internal/media/relay"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type MediaPlaybackStore interface {
	ResolveConnector(context.Context, string) (string, error)
	ResolveGuestMediaPass(context.Context, string, string, string) (string, error)
	PublicPublication(context.Context, string) (bridge.Publication, error)
	Connection(context.Context, string) (bridge.Connection, error)
	MediaPass(context.Context, string, string, string) (bridge.MediaPass, error)
	ActivateMediaPass(context.Context, string, string, string) (bridge.MediaPass, error)
	AuthorizeMediaPass(context.Context, string, string, string) (bridge.MediaPass, error)
}
type MediaPlaybackHandler struct {
	Store  MediaPlaybackStore
	Broker *relay.Broker
	Ready  bool
	key    []byte
}

func NewMediaPlaybackHandler(store MediaPlaybackStore, ready bool) *MediaPlaybackHandler {
	return &MediaPlaybackHandler{Store: store, Ready: ready, Broker: relay.New(), key: []byte(relay.Secret())}
}

type playbackSession struct {
	Publication string `json:"p"`
	Pass        string `json:"o"`
	Hash        string `json:"g"`
	Until       int64  `json:"e"`
}

func (h *MediaPlaybackHandler) sessionPath(c *gin.Context) string {
	return "/api/v1/media-playback/" + c.Param("id") + "/" + c.Param("passId") + "/"
}
func (h *MediaPlaybackHandler) signature(data string) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// A separate, path-scoped HttpOnly guest cookie supports native byte-range
// players without putting credentials in URLs. Staff cookies are ignored.
func (h *MediaPlaybackHandler) Session(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.Ready {
		c.Status(503)
		return
	}
	hash, e := bridge.Hash(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	if !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") || e != nil {
		c.Status(401)
		return
	}
	bid, e := h.Store.ResolveGuestMediaPass(c.Request.Context(), c.Param("id"), c.Param("passId"), hash)
	if e != nil {
		c.Status(404)
		return
	}
	pass, e := h.Store.MediaPass(c.Request.Context(), bid, c.Param("passId"), hash)
	if e != nil || pass.State != "paid" || pass.DurationSeconds < 1 || pass.DurationSeconds > 30*24*3600 || pass.ExpiresAt != nil && !pass.ExpiresAt.After(time.Now()) {
		c.Status(403)
		return
	}
	pub, e := h.Store.PublicPublication(c.Request.Context(), c.Param("id"))
	if e != nil || len(pub.Design.Libraries) == 0 {
		c.JSON(409, gin.H{"error": "The owner has not selected a playable library."})
		return
	}
	until := time.Now().Add(time.Duration(pass.DurationSeconds) * time.Second)
	if pass.ExpiresAt != nil {
		until = *pass.ExpiresAt
	}
	data, _ := json.Marshal(playbackSession{c.Param("id"), c.Param("passId"), hash, until.Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(data)
	http.SetCookie(c.Writer, &http.Cookie{Name: "mm_media_guest", Value: encoded + "." + h.signature(encoded), Path: h.sessionPath(c), Secure: c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: until, MaxAge: int(time.Until(until).Seconds())})
	c.JSON(200, gin.H{"libraries": pub.Design.Libraries})
}
func (h *MediaPlaybackHandler) guest(c *gin.Context) (string, string, error) {
	if !h.Ready {
		return "", "", access.ErrDenied
	}
	cookie, e := c.Cookie("mm_media_guest")
	if e != nil {
		return "", "", access.ErrDenied
	}
	parts := strings.Split(cookie, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(h.signature(parts[0]))) {
		return "", "", access.ErrDenied
	}
	data, e := base64.RawURLEncoding.DecodeString(parts[0])
	var session playbackSession
	if e != nil || json.Unmarshal(data, &session) != nil || session.Publication != c.Param("id") || session.Pass != c.Param("passId") || session.Until <= time.Now().Unix() {
		return "", "", access.ErrDenied
	}
	bid, e := h.Store.ResolveGuestMediaPass(c.Request.Context(), session.Publication, session.Pass, session.Hash)
	return bid, session.Hash, e
}
func (h *MediaPlaybackHandler) selected(ctx context.Context, publication, library string) error {
	pub, e := h.Store.PublicPublication(ctx, publication)
	if e != nil {
		return access.ErrDenied
	}
	for _, id := range pub.Design.Libraries {
		if id == library {
			return nil
		}
	}
	return access.ErrDenied
}
func (h *MediaPlaybackHandler) check(ctx context.Context, bid, publication, pass, hash, library string, activated bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if h.selected(ctx, publication, library) != nil {
		return access.ErrDenied
	}
	connection, e := h.Store.Connection(ctx, bid)
	if e != nil || connection.Revoked != nil || connection.TokenExpires == nil || !connection.TokenExpires.After(time.Now()) {
		return access.ErrDenied
	}
	if activated {
		_, e = h.Store.AuthorizeMediaPass(ctx, bid, pass, hash)
	} else {
		var p bridge.MediaPass
		p, e = h.Store.MediaPass(ctx, bid, pass, hash)
		if p.State != "paid" || p.ExpiresAt != nil && !p.ExpiresAt.After(time.Now()) {
			return access.ErrDenied
		}
	}
	return e
}
func (h *MediaPlaybackHandler) Catalog(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	bid, hash, e := h.guest(c)
	if e != nil {
		c.Status(403)
		return
	}
	library := c.Param("library")
	start, e := strconv.Atoi(c.DefaultQuery("start", "0"))
	if e != nil || start < 0 || start > 100000 {
		c.Status(400)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	check := func(ctx context.Context) error {
		return h.check(ctx, bid, c.Param("id"), c.Param("passId"), hash, library, false)
	}
	response, finish, e := h.Broker.Request(ctx, bid, relay.Task{Operation: "catalog", Library: library, Start: start, Expires: time.Now().Add(15 * time.Second)}, check)
	if e != nil {
		c.JSON(503, gin.H{"error": "Keep the media computer and connector running, then retry."})
		return
	}
	defer finish()
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	var items []jellyfin.CatalogItem
	if e != nil || len(data) > 1<<20 || response.Type != "application/json" || json.Unmarshal(data, &items) != nil || !jellyfin.ValidateCatalog(items) {
		c.Status(502)
		return
	}
	c.JSON(200, gin.H{"items": items, "start": start, "has_more": len(items) == 100})
}

// Available proves a selected library can be read before soliciting a bank
// transfer. This is availability, not payment or playback authorization.
func (h *MediaPlaybackHandler) Available(ctx context.Context, bid, publication string) bool {
	if !h.Ready {
		return false
	}
	pub, e := h.Store.PublicPublication(ctx, publication)
	if e != nil || len(pub.Design.Libraries) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for _, library := range pub.Design.Libraries {
		check := func(ctx context.Context) error {
			if h.selected(ctx, publication, library) != nil {
				return access.ErrDenied
			}
			connection, e := h.Store.Connection(ctx, bid)
			if e != nil || connection.Revoked != nil || connection.TokenExpires == nil || !connection.TokenExpires.After(time.Now()) {
				return access.ErrDenied
			}
			return nil
		}
		response, finish, e := h.Broker.Request(ctx, bid, relay.Task{Operation: "catalog", Library: library, Expires: time.Now().Add(15 * time.Second)}, check)
		if e != nil {
			return false
		}
		data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		finish()
		var items []jellyfin.CatalogItem
		if e != nil || len(data) > 1<<20 || response.Type != "application/json" || json.Unmarshal(data, &items) != nil || !jellyfin.ValidateCatalog(items) {
			return false
		}
		if len(items) > 0 {
			return true
		}
	}
	return false
}

func (h *MediaPlaybackHandler) Stream(c *gin.Context) {
	// This endpoint must never proxy media. Guests play from the owner PC on
	// the business LAN; the cloud handles management and authorization only.
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusGone, gin.H{"error": "Playback is available only on the business local network.", "code": "local_playback_required"})
}

func (h *MediaPlaybackHandler) connector(c *gin.Context) (string, string, error) {
	value := c.GetHeader("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return "", "", access.ErrDenied
	}
	hash, e := bridge.Hash(strings.TrimPrefix(value, "Bearer "))
	if e != nil {
		return "", "", e
	}
	bid, e := h.Store.ResolveConnector(c.Request.Context(), hash)
	return bid, hash, e
}
func (h *MediaPlaybackHandler) Poll(c *gin.Context) {
	bid, hash, e := h.connector(c)
	if e != nil {
		c.Status(401)
		return
	}
	task, ok := h.Broker.Poll(bid, hash)
	if !ok {
		c.Status(204)
		return
	}
	c.JSON(200, task)
}
func (h *MediaPlaybackHandler) Check(c *gin.Context) {
	bid, hash, e := h.connector(c)
	if e != nil || h.Broker.Check(c.Request.Context(), bid, hash, c.Param("task")) != nil {
		c.Status(403)
		return
	}
	c.Status(204)
}

type socketBody struct {
	io.Reader
	socket *websocket.Conn
	stream io.ReadCloser
}

func (b socketBody) Close() error { _ = b.socket.CloseNow(); return b.stream.Close() }
func (h *MediaPlaybackHandler) Socket(c *gin.Context) {
	bid, hash, e := h.connector(c)
	if e != nil {
		c.Status(401)
		return
	}
	ctx, e := h.Broker.Context(bid, hash, c.Param("task"))
	if e != nil || h.Broker.Check(ctx, bid, hash, c.Param("task")) != nil {
		c.Status(403)
		return
	}
	// The guest-owned task context survives HTTP hijacking and cancels the socket
	// at expiry/disconnect. No detached, unbounded background stream is created.
	socket, e := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		return
	}
	defer socket.CloseNow()
	socket.SetReadLimit(1024)
	headerCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	var metadata relay.Metadata
	e = wsjson.Read(headerCtx, socket, &metadata)
	stop()
	typ, _, te := mime.ParseMediaType(metadata.ContentType)
	if e != nil || te != nil || metadata.ContentLength < 0 || metadata.ContentLength > 1<<20 || metadata.Status != 200 || typ != "application/json" || metadata.ContentRange != "" {
		return
	}
	stream := websocket.NetConn(ctx, socket, websocket.MessageBinary)
	socket.SetReadLimit(64 << 10)
	body := socketBody{Reader: io.LimitReader(stream, metadata.ContentLength), socket: socket, stream: stream}
	_ = h.Broker.Deliver(ctx, bid, hash, c.Param("task"), relay.Response{Status: metadata.Status, Type: typ, Length: metadata.ContentLength, Range: metadata.ContentRange, Body: body})
}
