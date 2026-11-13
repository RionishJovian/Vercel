// Package mmpay provides an unregistered, sandbox-only MyanMyanPay adapter.
// It never changes a guest entitlement, calls a router or records settlement.
package mmpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const maxBody = 64 << 10

type Config struct {
	BaseURL        string
	AppID          string
	PublishableKey string
	SecretKey      string
}

type Client struct {
	config Config
	http   *http.Client
	nonce  atomic.Int64
}

// NewSandbox requires both test keys. Live endpoints are deliberately absent.
// BaseURL is supplied by the provider dashboard, never by an anonymous request.
func NewSandbox(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("mmpay: an HTTPS provider base URL is required")
	}
	if !validID(cfg.AppID) || !strings.HasPrefix(cfg.PublishableKey, "pk_test_") || !strings.HasPrefix(cfg.SecretKey, "sk_test_") || len(cfg.PublishableKey) <= 8 || len(cfg.SecretKey) <= 8 || strings.ContainsAny(cfg.PublishableKey+cfg.SecretKey, "\r\n\t ") {
		return nil, errors.New("mmpay: app ID and sandbox keys are required; live keys are refused")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{config: cfg, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Payment struct {
	OrderID          string `json:"orderId"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
	QR               string `json:"qr,omitempty"`
	VendorQRRefID    string `json:"vendorQrRefId,omitempty"`
	TransactionRefID string `json:"transactionRefId,omitempty"`
}

// Create uses the server-resolved whole-kyat price, never a browser-supplied price.
// A network error is an unknown outcome: inquire with the SAME order ID.
func (c *Client) Create(ctx context.Context, orderID string, amount int64, callbackURL string) (Payment, error) {
	if !validID(orderID) || amount < 1 || amount > 1000000000 {
		return Payment{}, errors.New("mmpay: invalid order or whole-kyat amount")
	}
	u, err := url.Parse(callbackURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return Payment{}, errors.New("mmpay: HTTPS callback URL required")
	}
	return c.exchange(ctx, "create", orderID, amount, callbackURL)
}

func (c *Client) Get(ctx context.Context, orderID string) (Payment, error) {
	if !validID(orderID) {
		return Payment{}, errors.New("mmpay: invalid order")
	}
	return c.exchange(ctx, "get", orderID, 0, "")
}

func (c *Client) exchange(ctx context.Context, operation, orderID string, amount int64, callbackURL string) (Payment, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	nonce := c.nextNonce()
	// Handshake token is local to this operation. Shared mutable tokens can attach
	// one concurrent customer's payment to another customer's handshake.
	var handshake struct {
		Token string `json:"token"`
	}
	if err := c.post(ctx, "handshake", struct {
		OrderID string `json:"orderId"`
		Nonce   string `json:"nonce"`
	}{orderID, nonce}, c.nextNonce(), "", &handshake); err != nil {
		return Payment{}, err
	}
	if handshake.Token == "" {
		return Payment{}, errors.New("mmpay: missing handshake token")
	}
	var payload any = struct {
		OrderID string `json:"orderId"`
		Nonce   string `json:"nonce"`
	}{orderID, nonce}
	if operation == "create" {
		payload = struct {
			AppID       string `json:"appId"`
			Nonce       string `json:"nonce"`
			Amount      int64  `json:"amount"`
			OrderID     string `json:"orderId"`
			CallbackURL string `json:"callbackUrl"`
			Currency    string `json:"currency"`
		}{c.config.AppID, nonce, amount, orderID, callbackURL, "MMK"}
	}
	var payment Payment
	if err := c.post(ctx, operation, payload, nonce, handshake.Token, &payment); err != nil {
		return Payment{}, err
	}
	// The published inquiry response omits currency; this adapter only creates
	// MMK orders. Reject an explicit different currency, normalize an omission.
	if operation == "get" && payment.Currency == "" {
		payment.Currency = "MMK"
	}
	if payment.OrderID != orderID || payment.Currency != "MMK" || payment.Amount < 1 || !validStatus(payment.Status) || (operation == "create" && (payment.Amount != amount || payment.Status != "PENDING" || payment.QR == "" || payment.VendorQRRefID == "")) {
		return Payment{}, errors.New("mmpay: payment response does not match the order")
	}
	return payment, nil
}

func (c *Client) post(ctx context.Context, operation string, payload any, nonce, token string, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("mmpay: cannot encode request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/payments/sandbox-"+operation, bytes.NewReader(body))
	if err != nil {
		return errors.New("mmpay: cannot construct request")
	}
	req.Header.Set("Authorization", "Bearer "+c.config.PublishableKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mmpay-Nonce", nonce)
	req.Header.Set("X-Mmpay-Signature", signature(c.config.SecretKey, nonce, body))
	if token != "" {
		req.Header.Set("X-Mmpay-Btoken", token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mmpay: sandbox-%s transport failed; outcome unknown", operation)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("mmpay: sandbox-%s HTTP %d", operation, res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil || len(data) > maxBody || json.Unmarshal(data, result) != nil {
		return errors.New("mmpay: invalid provider response")
	}
	return nil
}

func (c *Client) nextNonce() string {
	for {
		old := c.nonce.Load()
		next := time.Now().UnixMilli()
		if next <= old {
			next = old + 1
		}
		if c.nonce.CompareAndSwap(old, next) {
			return strconv.FormatInt(next, 10)
		}
	}
}

func signature(key, nonce string, body []byte) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(nonce + "."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func validID(s string) bool {
	return s != "" && len(s) <= 128 && !strings.ContainsAny(s, "\x00\r\n\t ")
}
func validStatus(s string) bool {
	switch s {
	case "PENDING", "SUCCESS", "FAILED", "REFUNDED", "CANCELLED", "EXPIRED":
		return true
	}
	return false
}
