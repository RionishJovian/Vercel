# Issue #192 — proxy boundary and shared-NAT protection

Status: source fix under review; **not deployed**. Main baseline: `e014fb3dc3c09d429dd356caae67143001ed4ec6`. No production configuration, customer data or RouterOS settings changed.

## Read-only production evidence — 2026-11-11 07:00–07:03 UTC

- Host `magic-app`: active host Caddy 2.11.7 routes the app to loopback port 18081. Its loaded HTTP server has no `trusted_proxies` or custom client-IP headers.
- Web Caddy 2.10.2 proxies `/api/*`, `/health`, `/ready` to `api:8080`; no proxy trust was configured. Inspected Compose peers: host gateway `172.18.0.1`, web `172.18.0.5`, API `172.18.0.4`. These are observations, **not stable allocations**.
- Two marked, read-only `/api/v1/version` requests originating from the Mac and VPS both logged Go `client_ip=172.18.0.5`. No guest-identifying data was retained. A real HotSpot-client observation is **NOT RUN**.
- API and web container revision labels both match the main baseline. `/ready` reported all dependencies OK; `/api/v1/version` still reported `build=development` (PR #191 supplies binary provenance).
- Production encryption-key presence was confirmed without exposing it. Network/address/management migration/guest/device/Wi-Fi gates were true. No gate was changed.

## Implementation contract

- Gin accepts only `X-Forwarded-For` from explicitly configured exact `server.trusted_proxy_ips` / `SERVER_TRUSTED_PROXY_IPS` (comma-separated). CIDRs, `private_ranges`, hostnames and `X-Real-IP` fallback are refused. Default remains loopback only. An unexpected/recreated proxy address fails closed to its socket peer.
- Web Caddy can trust one inspected `MM_EDGE_PROXY_IP`, with strict right-to-left parsing. Its safe default is loopback, not the whole Docker network. The reference Compose passes this variable; the existing VPS Compose is operator-owned and is not overwritten by deploy.
- One global API limiter covers public portal/media paths and `/api/v1/auth/`. Auth and guest budgets are separate. A server-signed, random, HTTPS-only/HttpOnly/SameSite=Lax rate cookie grants **no authentication, device ownership, free time or payment entitlement**. Cookies are scoped to `/api/v1/`; responses are `no-store`.
- Per-browser budget: 30 requests/minute. Aggregate source budgets: 600 auth and 6,000 guest requests/minute. Logout keeps its existing exception; per-account failed-login protections, cookie-origin checks, body bounds, media connector/relay limits and all handler authorization remain.
- Requests without a valid rate cookie consume the aggregate budget and receive a fresh rate cookie. Clients that deliberately omit cookies (including the existing media guest client) retain that behavior and are bounded by the aggregate cap. Cookie churn/spoofing cannot bypass that cap, but anonymous browser sessions are **not verified physical guests**. A malicious actor can still consume a shared aggregate allowance; these are abuse limits, not a promise of fair scheduling under attack.
- State is bounded to 16,384 live buckets and expires after one minute. Limiter counters are process-local and reset on API restart; valid signed cookies remain recognizable with the same signing key. Multi-replica/distributed enforcement is **not implemented**. Production is currently a single API instance.

## Validation

The previous source failed shared-NAT/session tests at the 31st cookieless request and accepted broad proxy CIDRs and unapproved `X-Real-IP`. The new focused auth/config/server tests pass with the race detector: 60 concurrent NAT browsers with 20 subsequent requests each; one abusive browser vs another; shared cross-device/portal/media budgets; missing, forged, expired and duplicate cookies; spoofed/absent/malformed forwarded headers; configured vs unexpected proxy peers; key absence; memory saturation/window recovery; Retry-After. Caddy `adapt` using the installed VPS web binary accepted the candidate via stdin; **no reload or config write occurred**.

These tests are application behavior evidence, not physical-router or capacity certification. Comprehensive combined checks and reviewer findings are recorded in the stabilization acceptance document.

## Release review prerequisites — do not auto-apply

1. Review the code, current CI and measured load. Reconfirm this single-instance budget against representative customer traffic. Keep existing per-account locks.
2. Independently establish and preserve stable controlled host/web peers (for example a reviewed dedicated proxy network with exact allocations). **Do not copy today's dynamic addresses into an enduring trust policy.** Review host edge headers; Internet clients may never choose their own trusted forwarding chain.
3. In a disposable production-shaped proxy chain, configure only those exact peers in web Caddy and Gin. Verify two independent source clients and a real owned HotSpot client, shared NAT with 50+ browsers, spoofed/absent headers, web/API restarts and 429 Retry-After. Actual Caddy forwarding-chain behavior remains a release acceptance check beyond syntax and Gin tests.
4. Obtain separate release approval for any VPS configuration change, main merge or production deployment. Save current Caddy/Compose/config and immutable image identities; use a validated DB backup and preserve compatible rollback images. No migration is required by #192.
5. After an approved rollout, re-read active Caddy config and container peers and repeat sanitized client-IP and normal-customer tests. If trust fails, do not broaden CIDRs; restore the reviewed config/image. Router settings remain outside this release.

References: [issue #192](https://github.com/RionishJovian/Vercel/issues/192), [Caddy forwarding-header behavior](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#headers), [Caddy strict proxy trust](https://caddyserver.com/docs/caddyfile/options#trusted-proxies), [Gin proxy configuration](https://gin-gonic.com/en/docs/server-config/trusted-proxies/).

## Follow-up disposable real-proxy acceptance — 2026-11-11

`TestRealCaddySharedNATAcceptance` passed with the race detector using official SHA-512-verified Caddy **2.11.7 edge** and **2.10.2 web** binaries, matching the observed VPS versions. Both proxies and the Go fixture bound only to loopback; no production/customer/router endpoint was contacted.

Proved through the actual two-proxy HTTP chain: distinct IPv4/IPv6 source readback; spoofed XFF/X-Real-IP refusal; 60 server-issued rate sessions sharing one IPv4 source with 20 requests each; a single session reaching 429 with Retry-After while another remains usable; web restart; API restart with signed-session recognition preserved and process-local counters reset. The fixture explicitly replays Secure cookies over local HTTP: **HTTPS browser/captive-browser cookie acceptance remains NOT RUN**, as do real HotSpot/RouterBOARD checks. Loopback peers model controlled hops; production stable Docker addressing is still a separate release prerequisite.

Initial fixture failures were caused by hostname-specific loopback Caddy sites not matching the preserved IPv6 Host; the web fixture now matches production's host-independent listener and both listeners remain restricted to loopback. No application fix was needed. The opt-in test skips unless `MM_CADDY_EDGE_BIN` and `MM_CADDY_WEB_BIN` point to verified local binaries; it never downloads or installs binaries automatically.
