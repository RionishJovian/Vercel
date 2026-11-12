package mmpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func config() Config {
	return Config{BaseURL: "https://sandbox.example", AppID: "app-test", PublishableKey: "pk_test_fixture", SecretKey: "sk_test_fixture"}
}

func sandbox(t *testing.T, fn http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewTLSServer(fn)
	t.Cleanup(s.Close)
	cfg := config()
	cfg.BaseURL = s.URL
	c, err := NewSandbox(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Trust only this test server's certificate and retain redirect rejection.
	c.http.Transport = s.Client().Transport
	return c
}

func TestSandboxRefusesLiveKeysAndUnsafeURLs(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.PublishableKey = "pk_live_secret" },
		func(c *Config) { c.SecretKey = "sk_live_secret" },
		func(c *Config) { c.BaseURL = "http://sandbox.example" },
		func(c *Config) { c.BaseURL = "https://user:pass@sandbox.example" },
		func(c *Config) { c.BaseURL = "https://sandbox.example?token=secret" },
		func(c *Config) { c.AppID = "" },
		func(c *Config) { c.SecretKey = "sk_test_" },
	} {
		cfg := config()
		mutate(&cfg)
		if _, err := NewSandbox(cfg); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestConcurrentOrdersKeepTheirOwnHandshake(t *testing.T) {
	var calls atomic.Int64
	var nonces sync.Map
	c := sandbox(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		nonce := r.Header.Get("X-Mmpay-Nonce")
		if _, exists := nonces.LoadOrStore(nonce, true); exists {
			t.Error("reused header nonce")
		}
		// Independent protocol fixture, not the implementation signing function.
		m := hmac.New(sha256.New, []byte("sk_test_fixture"))
		m.Write([]byte(nonce + "."))
		m.Write(b)
		if r.Header.Get("X-Mmpay-Signature") != hex.EncodeToString(m.Sum(nil)) || r.Header.Get("Authorization") != "Bearer pk_test_fixture" {
			t.Error("invalid signed request")
		}
		var p struct {
			OrderID  string `json:"orderId"`
			Nonce    string `json:"nonce"`
			Amount   int64  `json:"amount"`
			AppID    string `json:"appId"`
			Currency string `json:"currency"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/payments/sandbox-handshake":
			if r.Header.Get("X-Mmpay-Btoken") != "" {
				t.Error("handshake leaked token")
			}
			json.NewEncoder(w).Encode(map[string]string{"token": "bound-to-" + p.OrderID})
		case "/payments/sandbox-create", "/payments/sandbox-get":
			if r.Header.Get("X-Mmpay-Btoken") != "bound-to-"+p.OrderID {
				t.Error("wrong order's handshake token")
			}
			if p.Nonce != nonce {
				t.Error("operation body and header nonce differ")
			}
			if strings.HasSuffix(r.URL.Path, "create") && (p.AppID != "app-test" || p.Currency != "MMK" || p.Amount != 1000) {
				t.Error("wrong payment payload")
			}
			// Inquiry fixtures omit currency, matching the published get response.
			if strings.HasSuffix(r.URL.Path, "get") {
				fmt.Fprintf(w, `{"orderId":%q,"amount":1000,"status":"SUCCESS","transactionRefId":"tx-1"}`, p.OrderID)
			} else {
				json.NewEncoder(w).Encode(Payment{OrderID: p.OrderID, Amount: 1000, Currency: "MMK", Status: "PENDING", QR: "sandbox-only", VendorQRRefID: "qr-" + p.OrderID})
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("order-%d", i)
			if _, err := c.Create(context.Background(), id, 1000, "https://merchant.example/callback"); err != nil {
				t.Error(err)
			}
			p, err := c.Get(context.Background(), id)
			if err != nil || p.Currency != "MMK" || p.Status != "SUCCESS" {
				t.Errorf("inquiry: %+v, %v", p, err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 80 {
		t.Errorf("calls=%d", calls.Load())
	}
}

func TestHandshakeFailureNeverCreatesPaymentOrLeaksBody(t *testing.T) {
	for _, response := range []string{"http-error", "{}", "bad-json", strings.Repeat("x", maxBody+1)} {
		t.Run(fmt.Sprint(len(response), response[:min(len(response), 10)]), func(t *testing.T) {
			var calls int
			c := sandbox(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if response == "http-error" {
					w.WriteHeader(401)
					fmt.Fprint(w, "SECRET PROVIDER ERROR")
				} else {
					fmt.Fprint(w, response)
				}
			})
			_, err := c.Create(context.Background(), "order", 1000, "https://merchant.example/cb")
			if err == nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("provider body leaked")
			}
		})
	}
}

func TestCreateRejectsMismatchedResponse(t *testing.T) {
	for _, response := range []string{
		`{"orderId":"another-order","amount":1000,"currency":"MMK","status":"PENDING","qr":"qr","vendorQrRefId":"ref"}`,
		`{"orderId":"order","amount":999,"currency":"MMK","status":"PENDING","qr":"qr","vendorQrRefId":"ref"}`,
		`{"orderId":"order","amount":1000,"currency":"USD","status":"PENDING","qr":"qr","vendorQrRefId":"ref"}`,
		`{"orderId":"order","amount":1000,"currency":"MMK","status":"SUCCESS","qr":"qr","vendorQrRefId":"ref"}`,
		`{"orderId":"order","amount":1000,"currency":"MMK","status":"PENDING"}`,
	} {
		c := sandbox(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "handshake") {
				fmt.Fprint(w, `{"token":"token"}`)
			} else {
				fmt.Fprint(w, response)
			}
		})
		if _, err := c.Create(context.Background(), "order", 1000, "https://merchant.example/cb"); err == nil {
			t.Fatal("mismatched response accepted")
		}
	}
}

func TestCancelledRequestAndInvalidInputDoNotSend(t *testing.T) {
	var calls atomic.Int64
	c := sandbox(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	for _, amount := range []int64{0, -1, 1000000001} {
		if _, err := c.Create(context.Background(), "order", amount, "https://merchant.example/cb"); err == nil {
			t.Fatal("invalid amount accepted")
		}
	}
	if _, err := c.Create(context.Background(), "order", 1000, "http://merchant.example/cb"); err == nil {
		t.Fatal("unsafe callback accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Get(ctx, "order"); err == nil {
		t.Fatal("cancelled request succeeded")
	}
	if calls.Load() != 0 {
		t.Fatal("unexpected provider call")
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var forwarded atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer target.Close()
	c := sandbox(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) })
	if _, err := c.Get(context.Background(), "order"); err == nil {
		t.Fatal("redirect accepted")
	}
	if forwarded.Load() != 0 {
		t.Fatal("credentials forwarded")
	}
}

func TestVerifyCallbackRejectsTamperingAndWrongOrder(t *testing.T) {
	c, _ := NewSandbox(config())
	body := []byte(`{"orderId":"order-1","amount":1000,"currency":"MMK","status":"SUCCESS","vendorQrRefId":"qr-1","transactionRefId":"tx-1"}`)
	const nonce = "1700000000000"
	// Frozen HMAC fixture generated independently with Node's createHmac.
	const signed = "99e9a998c3627d4572365dc8404aff6458b1363db69510b1eaea1fde743df62b"
	expected := Expected{OrderID: "order-1", Amount: 1000, VendorQRRefID: "qr-1"}
	if e, err := c.VerifyCallback(body, nonce, signed, expected); err != nil || e.TransactionRefID != "tx-1" {
		t.Fatalf("valid fixture: %+v %v", e, err)
	}
	for _, change := range []func() ([]byte, string, string, Expected){
		func() ([]byte, string, string, Expected) { return append(body, ' '), nonce, signed, expected },
		func() ([]byte, string, string, Expected) { return body, "different", signed, expected },
		func() ([]byte, string, string, Expected) { return body, nonce, "bad", expected },
		func() ([]byte, string, string, Expected) {
			e := expected
			e.OrderID = "another-order"
			return body, nonce, signed, e
		},
		func() ([]byte, string, string, Expected) {
			e := expected
			e.Amount = 500
			return body, nonce, signed, e
		},
		func() ([]byte, string, string, Expected) {
			e := expected
			e.VendorQRRefID = "another-qr"
			return body, nonce, signed, e
		},
	} {
		b, n, s, e := change()
		if _, err := c.VerifyCallback(b, n, s, e); err == nil {
			t.Fatal("tampered callback accepted")
		}
	}
	otherCfg := config()
	otherCfg.SecretKey = "sk_test_other_merchant"
	other, _ := NewSandbox(otherCfg)
	if _, err := other.VerifyCallback(body, nonce, signed, expected); err == nil {
		t.Fatal("different merchant accepted")
	}
}

func TestSignedButInvalidCallbackFailsClosed(t *testing.T) {
	c, _ := NewSandbox(config())
	expected := Expected{"order-1", 1000, "qr-1"}
	for _, body := range []string{
		`{"orderId":"order-1","amount":1000,"currency":"USD","status":"SUCCESS","vendorQrRefId":"qr-1","transactionRefId":"tx-1"}`,
		`{"orderId":"order-1","amount":1000.1,"currency":"MMK","status":"SUCCESS","vendorQrRefId":"qr-1","transactionRefId":"tx-1"}`,
		`{"orderId":"order-1","amount":1000,"currency":"MMK","status":"SUCCESS","vendorQrRefId":"qr-1"}`,
		`{"orderId":"order-1","amount":1000,"currency":"MMK","status":"PAID","vendorQrRefId":"qr-1","transactionRefId":"tx-1"}`,
		`not-json`, strings.Repeat("x", maxBody+1),
	} {
		if _, err := c.VerifyCallback([]byte(body), "nonce", signature("sk_test_fixture", "nonce", []byte(body)), expected); err == nil {
			t.Fatal("invalid callback accepted")
		}
	}
}
