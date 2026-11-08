# N19 — Site-wide SSID synchronization implementation plan

**Decision:** One MikroTik gateway per SME site; MikroMagic changes a site SSID once across supported APs. Third-party controllers are optional integration paths, not a product requirement. The gateway alone cannot configure third-party AP firmware through CAPsMAN.

## Gates, in order

| Step | Deliverable | Exit criteria |
| --- | --- | --- |
| 1 | Read-only AP capability/evidence contract | Tenant/site-bound identity, model/firmware, authenticated adapter, separate fresh read/write/readback/rollback evidence. No writes. |
| 2 | MikroTik radio inventory | Modern WiFi CAPsMAN vs legacy CAPsMAN detection; managed CAP/radio/SSID observations and explicit unavailable states. RouterOS commands read-only. |
| 3 | Desired-state and preview | Site-scoped Guest/Staff SSID revision, per-AP mapping, security/VLAN/HotSpot compatibility, disruption estimate and diff; no apply on absent evidence. |
| 4 | Owner-approved per-AP apply | Review/hash fencing, owner-only authorization, backup as appropriate, single AP at a time, post-write readback, safe rollback or Recovery Required, audit. |
| 5 | Third-party adapter certification | Verify actual documented API, authorization, model/firmware, read/write/readback/rollback and site management path on disposable device; unsupported stays monitoring-only. |
| 6 | Mixed-site rollout | Honest per-AP Verified/Pending/Unsupported/Failed statuses, newest-offline-revision wins, drift resolution, no guest HotSpot interruption without approval, real-hardware acceptance. |

## Step 1 scope (current PR)

Create pure backend evidence contract at `internal/ssid`. `ReadyForReview` is **not** write authorization; `WriteAllowed` remains false. No API, DNS, Wi-Fi or router writes are added. Follow-on adapter callers must source evidence from authenticated site-scoped integrations, never from model names, MACs, gateway neighbor entries or arbitrary HTTP clients.

## Operational safeguards

- Do not assume fixed Ruijie, iKuai, TP-Link, Huawei or MikroTik models.
- Unsupported equipment stays visible and working; no covert browser automation or guessed API calls to APs.
- Existing in-flight DNS firewall/Quick Connect fixes are independent release gates.
- CI and RouterOS CHR tests are not real mixed-vendor hardware acceptance; document each separately.
- Do not merge or deploy a write adapter without explicit owner approval gating, verified backup/recovery strategy, and real compatible test-device proof.

## Sources / canonical requirement

- [Notion N19 — Managed devices](https://www.notion.so/3f8489e3dbb6810d9261e2bd3acbab64)
- [MikroTik modern CAPsMAN](https://help.mikrotik.com/docs/spaces/ROS/pages/224559120/WiFi#WiFi-CAPsMAN)
- [MikroTik legacy CAPsMAN](https://help.mikrotik.com/docs/spaces/ROS/pages/1409149/CAPsMAN)
