# Discover Nearby Router: acceptance record, Stage A

Date: 2026-11-06. Scope: **discovery only**, on a real RouterBOARD, against a **local rehearsal server**. Nothing here touches production, and no router was enrolled. Discovery evidence and enrollment evidence are kept apart; there is **no enrollment evidence yet**.

## Status by evidence level

| Item | Implemented | Tested | Deployed | Hardware verified |
|---|---|---|---|---|
| MNDP parsing (helper) | yes | yes | no | **yes**, one hAP ax2, RouterOS 7.25, macOS |
| Helper pairing and upload over verified HTTPS | yes | yes | no | **yes**, against a local server (Stage A), not production |
| Discovery sessions backend (+ migrations) | yes | yes | no | **yes**, local server and real PostgreSQL |
| Console: automatic discovery section | yes | yes | no | **yes**, in a real browser against the local server |
| Select router and prefill the add-router form | yes | yes | no | **yes**, local |
| Registration and one-paste enrollment from a discovered router | existing flow | n/a | n/a | **not done** (Stage C) |
| Android Chrome, iPhone Safari | yes | unit tests | no | **not done** (Stage B) |
| Windows | yes | no | no | **not done** |
| Router never connected to Magic Hub | n/a | n/a | n/a | **not met**: this router had been registered; see below |

## Setup (rehearsal only)

- Router: MikroTik hAP ax2, board `C52iG-5HaxD2HaxD`, RouterOS 7.25 (2026-10-22 build). Its Magic Hub record was deleted in the app and the router was reset to its default configuration from WinBox on the same day, then connected to the Mac by Ethernet (Mac `192.168.88.5`, router `192.168.88.1`).
- Server: this branch built and run locally with `discovery.enabled: true`, against a throwaway PostgreSQL 17 built from `db/baseline.sql` plus **every** file in `db/migrations/` (including both discovery migrations; they apply cleanly on the real chain), connected as the restricted `mega_magic_runtime` role. NATS and Redis ran locally.
- TLS: the helper only talks HTTPS. A throwaway local CA and a small local TLS front were used with the helper's new `--ca-file` option; certificate verification stayed on.
- Console: the Vite dev server behind a local gateway that keeps the Host header, so the server's same-origin check was exercised as a browser sends it. A local test account was registered through the app's own registration endpoint; its credentials are not recorded here.

## Discovery evidence

1. **Announcements heard on a real network.** The helper heard the router's MNDP announcements and rejected only small non-announcement packets (WinBox's own requests). Parsed fields: identity `hAP`, board `C52iG-5HaxD2HaxD`, RouterOS `7.25 2026-10-22 14:12:00`, source `192.168.88.1`, interface `bridge/ether2`, MAC `48:A9:8A:D6:E2:01`.
2. **Real packet layout.** A raw capture showed big-endian TLV type and length, a 4-byte header, little-endian uptime, and TLVs `0x12` and `0x13` that are not in the public dissector. The parser tolerates them.
3. **The MAC is per interface.** The MAC in the announcement belonged to the sending interface (`...:06` over Wi-Fi earlier, `...:01` over Ethernet now). The device key (a one-way hash of the software id, `sw:e067288b1d7bb9ba`) was **identical** across the two interfaces and across the reset. The raw software id is never stored or sent.
4. **Wrong network hears nothing.** With the Mac on the home network behind the router, 70 seconds of listening heard nothing. This is expected: discovery depends on being on the router's own network.
5. **Port sharing.** WinBox holds UDP 5678, which stopped the first helper build from starting. The helper now shares the port and received announcements with WinBox open.
6. **Pairing and upload.** The console started a session and showed a one-time code with a countdown; `magic-discover --pair CODE` paired (the code is single use) and uploaded every 15 seconds. The session moved from open to paired in the console **by itself** (see "Found during this stage").
7. **Console card** (`docs/images/discovery-stage-a-candidate-card.jpg`): Device `MikroTik RouterBOARD`, Model `C52iG-5HaxD2HaxD`, Address `192.168.88.1`, RouterOS `7.25 2026-10-22 14:12:00`, Discovered by `MNDP`, Status **`Nearby, not verified`**. Nothing said Connected, Managed or Online.
8. **Select router** filled the add-router form (name `hAP`, the session's site, model and local address labelled not verified). **No router record was created** (the account still had 0 routers).
9. **No change to the router.** While the helper ran its only sockets were the UDP listener on 5678 and a TCP connection to the local server; none went to the router. The router's uptime kept counting (284 s, then 823 s about ten minutes later), so nothing restarted it. A before-and-after configuration export was **not** taken (it needs a login on the router).

## Found during this stage (and fixed)

- **Console polling never started.** `useSWR` evaluates a function-style `refreshInterval` once, before any data exists, so it returned 0 and the session never updated. Component tests with mocked answers could not see it. Fixed with a fixed interval switched by state, and two tests that depend on real polling (both fail on the old code).
- **Helper could not start while WinBox was open.** Fixed by sharing the port.
- **A time-of-day flake in an unrelated Go test** (a `02:00` substring check against timestamps) failed CI at 02:00 UTC; fixed separately.

## Not verified

- A router that has **never** connected to Magic Hub (this one was reset, but had been registered).
- Enrollment of a discovered router and its one-paste connection (Stage C; needs a release).
- Android Chrome and iPhone Safari (Stage B).
- Windows; other RouterOS versions and models; the router's default configuration after the reset was not inspected.
- A before-and-after router export.
