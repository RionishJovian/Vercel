package webfig

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName    = "__Host-mm_webfig"
	openPath      = "/__mm/open"
	recheckPeriod = 30 * time.Second
)

// Checker confirms a session may continue: the login is still valid, the role
// still qualifies, the switch still allows it, the router is still this tenant's
// at this address. Any error ends the session.
type Checker func(context.Context, Grant) error

// Proxy serves only the WebFig domain and its session subdomains. It has no route
// to the application, and that domain shares nothing with the application's.
type Proxy struct {
	host      string
	store     *Store
	check     Checker
	transport http.RoundTripper
}

func NewProxy(host string, store *Store, check Checker) *Proxy {
	return &Proxy{host: strings.ToLower(host), store: store, check: check, transport: newTransport()}
}

// newTransport has no response timeout on purpose: WebFig holds one /jsproxy
// request open until something changes, and ending it logs the user out. A
// session's own deadline bounds it instead. HTTPS upstreams use the system
// trust store and must present a certificate valid for the target address.
func newTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		DisableCompression:    true,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 0,
	}
}

// Handles reports whether a request belongs to this proxy.
func (p *Proxy) Handles(r *http.Request) bool {
	host := strings.ToLower(requestHost(r))
	return host == p.host || strings.HasSuffix(host, "."+p.host)
}

func requestHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h
	}
	return r.Host
}

// label is the session subdomain this request came in on. Empty for anything else.
func (p *Proxy) label(r *http.Request) string {
	label, found := strings.CutSuffix(strings.ToLower(requestHost(r)), "."+p.host)
	if !found || label == "" || strings.Contains(label, ".") {
		return ""
	}
	return label
}

// safePath refuses anything a router's web server could read as a different path
// than the allow-list saw, including when it is encoded.
func safePath(r *http.Request) bool {
	for _, value := range []string{r.URL.Path, r.URL.EscapedPath()} {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "\\") || strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%25") {
			return false
		}
		for _, segment := range strings.Split(value, "/") {
			if segment == "." || segment == ".." {
				return false
			}
		}
	}
	return true
}

// cleanRouterResponse keeps what WebFig needs from a router's reply and drops what
// could reach beyond this session's own origin. A router's owner controls this
// reply, so nothing in it is trusted.
func cleanRouterResponse(h http.Header) {
	cookies := (&http.Response{Header: h}).Cookies()
	h.Del("Set-Cookie")
	for _, c := range cookies {
		if c.Name == cookieName || strings.HasPrefix(c.Name, "__Host-") || strings.HasPrefix(c.Name, "__Secure-") {
			continue
		}
		c.Domain = ""
		c.Secure = true
		if line := c.String(); line != "" {
			h.Add("Set-Cookie", line)
		}
	}
	for _, name := range []string{"Service-Worker-Allowed", "Clear-Site-Data", "Alt-Svc", "Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Link"} {
		h.Del(name)
	}
}

// allowedPath lists what WebFig itself uses. Everything else on the router's web
// server, such as /rest, is not part of this feature.
func allowedPath(path string) bool {
	switch path {
	case "/", "/webfig", "/jsproxy", "/mikrotik_logo.png", "/favicon.png", "/favicon.ico", "/graphs":
		return true
	}
	for _, prefix := range []string{"/webfig/", "/jsproxy/", "/assets/", "/graphs/", "/skins/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func safetyHeaders(h http.Header) {
	h.Set("Strict-Transport-Security", "max-age=31536000")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "frame-ancestors 'none'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex, nofollow")
}

func notice(w http.ResponseWriter, status int, title, body string) {
	safetyHeaders(w.Header())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><style>body{font:16px system-ui,sans-serif;margin:0;min-height:100vh;display:grid;place-items:center;background:#0c1f1a;color:#e8f2ee}main{max-width:26rem;padding:2rem;text-align:center}p{opacity:.8}</style></head><body><main><h1>%s</h1><p>%s</p></main></body></html>`, title, title, body)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	label := p.label(r)
	if r.URL.Path == openPath {
		p.open(w, r, label)
		return
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		notice(w, http.StatusUnauthorized, "Open WebFig from Mega Magic", "This page opens from the router's page in Mega Magic. You can close this tab and open it again there.")
		return
	}
	live, ok := p.store.Lookup(cookie.Value, label, recheckPeriod)
	if !ok {
		notice(w, http.StatusUnauthorized, "WebFig session ended", "WebFig sessions last one hour. Open it again from the router's page in Mega Magic.")
		return
	}
	if live.Due {
		if err := p.check(r.Context(), live.Grant); err != nil {
			p.store.Revoke(cookie.Value)
			notice(w, http.StatusForbidden, "WebFig session ended", "This access is no longer allowed. Open it again from the router's page in Mega Magic.")
			return
		}
	}
	// SameSite stops other sites sending the cookie; this also stops them writing.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if origin := r.Header.Get("Origin"); origin != "https://"+label+"."+p.host {
			notice(w, http.StatusForbidden, "Request refused", "This request did not come from WebFig.")
			return
		}
	}
	// A background script could outlive this session and rewrite later pages.
	if r.Header.Get("Service-Worker") != "" {
		notice(w, http.StatusForbidden, "Request refused", "This request is not part of WebFig.")
		return
	}
	if !safePath(r) || !allowedPath(r.URL.Path) {
		notice(w, http.StatusNotFound, "Not available", "Only WebFig is available here.")
		return
	}
	// The server's 30 second limits fit API calls, not WebFig's long-held request
	// or a file upload. The session's deadline replaces them for this request.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(live.Expires)
	_ = rc.SetWriteDeadline(live.Expires)
	ctx, cancel := context.WithDeadline(r.Context(), live.Expires)
	defer cancel()
	go func() {
		// A held request may be the only traffic in this session. Recheck it
		// independently of Lookup so other requests cannot postpone this check.
		ticker := time.NewTicker(recheckPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-live.Done:
				cancel()
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				// An unavailable authorization service must not leave a router
				// request running indefinitely either.
				checkCtx, stop := context.WithTimeout(ctx, recheckPeriod)
				err := p.check(checkCtx, live.Grant)
				stop()
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					p.store.Revoke(cookie.Value)
					cancel()
					return
				}
			}
		}
	}()
	p.reverse(live.Grant).ServeHTTP(w, r.WithContext(ctx))
}

func (p *Proxy) open(w http.ResponseWriter, r *http.Request, label string) {
	if r.Method != http.MethodGet {
		notice(w, http.StatusUnauthorized, "This link has expired", "Links to WebFig work once, for one minute. Open it again from the router's page in Mega Magic.")
		return
	}
	value, expires, ok := p.store.Redeem(r.URL.Query().Get("ticket"), label)
	if !ok {
		notice(w, http.StatusUnauthorized, "This link has expired", "Links to WebFig work once, for one minute. Open it again from the router's page in Mega Magic.")
		return
	}
	safetyHeaders(w.Header())
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: value, Path: "/", Expires: expires, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/webfig/", http.StatusFound)
}

func (p *Proxy) reverse(g Grant) *httputil.ReverseProxy {
	target := net.JoinHostPort(g.Host, strconv.Itoa(g.Port))
	return &httputil.ReverseProxy{
		Transport:     p.transport,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = g.Scheme, target, target
			forwardCookies(pr.Out)
			for _, name := range []string{"Authorization", "Origin", "Referer", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Via"} {
				pr.Out.Header.Del(name)
			}
		},
		ModifyResponse: func(res *http.Response) error {
			cleanRouterResponse(res.Header)
			safetyHeaders(res.Header)
			if location := res.Header.Get("Location"); location != "" {
				for _, prefix := range []string{"http://" + target, "https://" + target, "http://" + g.Host, "https://" + g.Host} {
					if rest, found := strings.CutPrefix(location, prefix); found {
						if rest == "" {
							rest = "/"
						}
						res.Header.Set("Location", rest)
						break
					}
				}
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, context.DeadlineExceeded) && req.Context().Err() != nil {
				notice(rw, http.StatusUnauthorized, "WebFig session ended", "WebFig sessions last one hour. Open it again from the router's page in Mega Magic.")
				return
			}
			log.Printf("webfig: device %s: %v", g.DeviceID, err)
			var certErr *tls.CertificateVerificationError
			if errors.As(err, &certErr) {
				notice(rw, http.StatusBadGateway, "Router certificate could not be verified", "HTTPS WebFig needs a trusted certificate valid for this router's Magic Hub address.")
				return
			}
			notice(rw, http.StatusBadGateway, "The router did not answer", "Magic Hub could not reach WebFig on this router just now. Check that the router is online, then try again.")
		},
	}
}

// forwardCookies passes the router's own cookies and drops the proxy's session.
func forwardCookies(out *http.Request) {
	cookies := out.Cookies()
	out.Header.Del("Cookie")
	for _, c := range cookies {
		if c.Name == cookieName {
			continue
		}
		out.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
}
