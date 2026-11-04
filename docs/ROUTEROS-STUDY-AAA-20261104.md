# RouterOS study: REST API, Hotspot, captive portal, RADIUS, User Manager, PPP

Read 2026-11-04 from the official manual (RouterOS 7.26 pages). Continues `ROUTEROS-STUDY-20261104.md`.
This is a manual study. Nothing here was run on a router. Check the router's real version and output before relying on a property. Code references are to the current `main`.

## Findings that affect the app now

### 1. The PPPoE scripts use a reserved variable name (blocker)
Source: [scripting](https://manual.mikrotik.com/docs/developer-guides/scripting/): "All built-in RouterOS properties are reserved names", and inside `where` such a name "resolves to the property of the currently printed item instead of your variable". The manual's example `{:local type "ether1";/interface print where name=$type;}` prints nothing.

`internal/provisioner/routeros/broadband_scripts.go` declares `:local name` and then filters with `where name=$name`. Going by the manual, `$name` reads each record's own name, so the filter matches every record:
- `/ppp active remove [find where name=$name service=pppoe]` would disconnect **every** PPPoE session on the router, not just the expired subscriber's. That includes subscribers the app does not manage.
- `[:len [/ppp active find where name=$name service=pppoe]] > 0` would be true whenever anyone is connected, so an unused account is denied, which triggers the remove above. The guard runs every 15 seconds.
- In the on-up hook, `[/ppp secret find where name=$name]` matches every secret, so with two or more secrets `[:len $u] != 1` exits before first use is recorded.

Fix: rename the variable (for example `bbUser`). The older voucher guard already avoids this with `$loginUser`.

### 2. Device mode can block Hotspot and Scheduler (new product gap)
Source: [device mode](https://manual.mikrotik.com/docs/system-information-and-utilities/device-mode/).

| Mode | Factory default on | Blocks (relevant) |
|---|---|---|
| `home` | home routers | Hotspot, Scheduler, Fetch, Bandwidth Test, Proxy, RoMon |
| `basic` | other devices | Hotspot, Bandwidth Test, Proxy |
| `advanced` | CCR, 1100 series, and older devices that upgraded | none of these |

The error is `failure: not allowed by device-mode`. Changing a mode needs `/system/device-mode/update` plus a reset or mode button press (or power cut) within 5 minutes, so the app cannot do it remotely. A `mode=` change replaces the whole device-mode configuration except what the same command sets. `hotspot=yes` or `scheduler=yes` changes a single feature.

The app has no device-mode check. Vouchers need Hotspot and guards need Scheduler, so a new router could fail partway through setup. It should read `/system/device-mode/print` before setup and tell the owner what to press. Which mode each real model ships in needs checking on a router, and so does the exact `scheduler` flag name.

### 3. Multi-line SSH commands (already reported, now confirmed)
Source: [SSH](https://manual.mikrotik.com/docs/management-tools/ssh/): multi-line commands "fail on older RouterOS versions" and "RouterOS 7.25 and later execute them". Scripting: each CLI line is its own scope. `broadband.go` renew/suspend/resume embed `bbClockSource`, which contains raw newlines. Stored scripts and hooks are fine because `routerOSQuoted` escapes newlines.

### 4. Answered on a real router
The questions the manual left open were answered on a CHR. See [CHR lab](CHR-LAB.md), which also lists the app bugs this found.

## REST API
Source: [REST API](https://manual.mikrotik.com/docs/developer-guides/rest-api/).
- JSON over `www-ssl` or `www`, same menus and permissions as the console. GET=print, PUT=add, PATCH=set, DELETE=remove, POST=any command. Group needs `api` and `rest-api`.
- Limits: 60 s per request; one user's requests run one at a time; no streaming; every reply value is a string; no CORS, so server-side only.
- Useful: `.proplist` and `.query`, `count-only`, `/console/inspect` (discovers commands on that router version), `/execute` for background jobs, JSON error bodies with `detail`.
- Restrict with `/ip/service available-from` and `/user address`. `rest-plain=no` turns off REST over HTTP. The login is cached for 10 minutes; group policy changes apply when it renews.

**For the app:** real bugs came from parsing `print` text whose layout varies by menu. Reads over REST through the Hub tunnel would remove that, and could use one extra read-only user. It needs the `www` or `www-ssl` service, the group and firewall changes in the enrollment script, and a new test on a real router. Treat it as a later read-only pilot, not a rewrite.

The binary API (8728/8729) has `listen` for pushed changes, which REST cannot do, but needs its own client.

## Hotspot and captive portal
Sources: [Hotspot](https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/), [customisation](https://manual.mikrotik.com/docs/authentication-authorization-accounting/hotspot-captive-portal/hotspot-customisation/).
- **Login methods:** `cookie` (must be combined), `http-chap`, `http-pap`, `https` (needs a certificate), `mac`, `trial`, `mac-cookie`. The app's wizard uses `http-chap,mac`.
- **Limits on a user:** `limit-uptime`, `limit-bytes-in/out/total`. Reaching one disconnects the user.
- **User profile:** `rate-limit` (rx = client upload, tx = client download), `shared-users` (default 1), `session-timeout`, `idle-timeout`, `address-list`.
- **Scripts:** on-login and on-logout get `$user`, `$username`, `$address`, `$"mac-address"`, `$interface`. on-logout also gets `$cause` and counters such as `$"bytes-total"`.
- **Pages:** `login.html`, `alogin.html`, `status.html`, `logout.html`, `error.html`, `redirect.html`, `md5.js`, plus `rlogin`, `rstatus`, `flogin`, `fstatus`, `flogout`, `radvert`, and `errors.txt` for translatable messages. Subfolders give languages through `target`. A missing page falls back to `redirect.html` or 404.
- **CHAP:** the hash is MD5 of chap-id + password + chap-challenge.
- **External pages:** add the server to the walled garden; a router page posts MAC, IP and links; the external server returns the client to the router's login. Some built-in links are hard-coded HTTP.
- **Captive portal API (RFC 7710):** an `api.json` template with `captive`, `user-portal-url` (from `$(link-login-only)`), optional `seconds-remaining` and `bytes-remaining`, `can-extend-session`. Phones can show the portal and time left without a redirect. The page gives no setup steps; the DHCP page URL I tried returned 404, so this is unfinished.
- **Walled garden:** `/ip/hotspot/walled-garden` is HTTP(S) by host and path; `/walled-garden/ip` is by address, protocol, port, and its `dst-host` adds a dynamic entry. The app uses `/ip` with `dst-host` for the portal host. The manual does not say which menu suits HTTPS, so confirm on a router.
- **HTTPS:** `https-redirect` sends users visiting https:// to login, and the certificate name mismatch often shows browser errors.
- **Multi-WAN:** Hotspot uses the web proxy and "only the default routing table", so PCC is not valid, and mangle can override local delivery (fix with `/routing/settings` or `/routing/rule`). The app's balancing already keeps ECMP in the main table for this reason (`network_plan.go:304`), so this matches.
- **IPv4 only**, because it relies on NAT rules.
- **Addresses per MAC** defaults to 2.
- **MAC cookie** stores username and password per MAC. It is removed on user logout, admin reset, traffic limit and session timeout. The app already revokes `/ip hotspot cookie` in its recovery and disable paths.

**FastTrack:** the [queues page](https://manual.mikrotik.com/docs/firewall-and-quality-of-service/queues/) says FastTrack bypasses simple queues "and they will not work". The queues page does not cover the dynamic queues Hotspot and PPP rate limits create, so speed tiers still need a measured test on a router.

## RADIUS
Source: [RADIUS](https://manual.mikrotik.com/docs/authentication-authorization-accounting/radius/).
- Consulted only when no local user matches. Sent attributes override the profile; anything not sent falls back to it.
- `/radius`: `service`, `address`, `secret`, ports 1812/1813, `timeout` 1100 ms, `protocol` udp or radsec, `require-message-auth` default `yes-for-request-resp`. A server that does not send Message-Authenticator may be refused, so the wizard should warn about this.
- Service switches: `/ip/hotspot/profile use-radius`, `/ppp/aaa use-radius`, `/user/aaa use-radius`.
- CoA and Disconnect: `/radius/incoming accept=yes`, port 1700. CoA can change `Mikrotik-Rate-Limit`, recv/xmit limits, `Mikrotik-Group`, `Filter-Id`, timeouts. It cannot change IP, pool or routes. POD is not supported.
- Key attributes: `Session-Timeout`, `Idle-Timeout`, `Mikrotik-Rate-Limit` (highest precedence), `Mikrotik-Recv-Limit`/`Xmit-Limit`, `Framed-Pool`, `Filter-Id`.
- Diagnosis: `/radius/monitor`. With CHAP, a wrong secret shows as rising `bad-replies`.

## User Manager
Source: [User Manager](https://manual.mikrotik.com/docs/authentication-authorization-accounting/user-manager/).
- Separate package, not on SMIPS. RADIUS server on the router itself (`/radius add address=127.0.0.1`). SQLite on flash. Active sessions limited by licence level (no numbers given).
- Profile: `price`, `validity`, `starts-when=assigned|first-auth`. Limitation: `download-limit`, `upload-limit`, `transfer-limit`, `uptime-limit`, reset intervals, rate limits. `add-batch-users`, `generate-voucher`, `generate-report`, sessions, `.umb` backups. PayPal is the only payment provider.
- Limits need RADIUS accounting and interim updates; disconnecting needs `/radius/incoming accept=yes`.
- The page documents no REST or API of its own, and gives no Hotspot setup steps. The REST page says REST covers the same menus as the console, so `/rest/user-manager/...` should work, but that needs testing. This corrects my earlier note, which treated it as CLI and web only.

**For the app:** `starts-when=first-auth` plus `validity` is what the hand-written PPPoE guard and first-use scripts rebuild: first-use validity, expiry and disconnect handled on the router with the app offline. Before fixing the custom scripts further, compare it on a test router. Costs: extra package, licence session limit, flash database, `only-one` ignored under RADIUS (use `shared-users`), and it does not fit SMIPS models.

## PPP and PPPoE
Sources: [PPP AAA](https://manual.mikrotik.com/docs/authentication-authorization-accounting/ppp-aaa/), [PPPoE](https://manual.mikrotik.com/docs/virtual-private-networks/pppoe/).
- Secret beats profile beats default profile. `only-one` is ignored with RADIUS.
- on-up and on-down variables: `user`, `local-address`, `remote-address`, `caller-id`, `called-id`, `interface` (an ID, not a name).
- `/ppp/active/print stats` gives `bytes` and `packets`: the first figure is what the router transmitted (client download). `parseBroadbandSession` reads it in that order, which matches.
- MTU/MRU max 1492; clamp MSS (`change-tcp-mss`, IPv4 only). No IP or DHCP on the PPPoE interface. `keepalive-timeout` default 10 s; `one-session-per-host` prevents stale sessions.

## Management account and services
Sources: [User](https://manual.mikrotik.com/docs/authentication-authorization-accounting/user/), [services](https://manual.mikrotik.com/docs/system-information-and-utilities/services/), [API](https://manual.mikrotik.com/docs/developer-guides/api/), [scheduler](https://manual.mikrotik.com/docs/system-information-and-utilities/scheduler/).
- Default `read` group includes reboot and sensitive; use a custom group. Once an SSH key is assigned, password SSH is refused for that user.
- `/ip/service available-from` must include the management address before saving, or the next login is refused. Blocked clients complete the TCP handshake and are then closed.
- Scheduler: `start-time=startup` with an interval first runs one interval after startup. Startup runs come before interfaces and DHCP. Global variables are shared between scripts of the same user.

## Not studied yet
Certificates and ACME, Dot1X, DHCP options for the captive portal (need the right page), firewall details beyond Codex's notes, and how the queues Hotspot and PPP create actually order.
