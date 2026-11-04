# Router-local Hotspot and PPPoE controlled trial

Prepared 2026-11-04. **Not executed.** Owner reported the physical router unavailable.
Software release at preparation: `174510cf0cfcc274e19a333c0e20f26871292784`.
Recheck current release and router state when execution becomes possible.

## Scope and prerequisites

Use the owner's spare router and dedicated test clients/accounts. The current
registered hAP ax3 Dev has no verified saved management connection or recent Hub
handshake in the latest recorded check; do not assume that older check is current.
Start with read-only discovery of model, RouterOS version, clock/NTP, Hub access,
existing Hotspot and PPPoE service/address plans, and available local recovery.
The app installs subscribers into an existing usable PPPoE service. It does not
configure arbitrary factory-default WAN, NTP or PPPoE service settings in this batch.

Keep a private pre-trial configuration backup and inventory of existing users,
profiles, scripts and schedulers. Do not print passwords, configuration secrets,
session cookies or credentials into evidence. Record only the test identities,
router/business IDs, operation IDs, relevant timestamps and redacted state.
Use the app's explicit review/confirmation controls for account changes.

Do not stop the shared production app to simulate an outage. Isolate only the
spare router/test clients from the central management/authentication dependencies,
while retaining WAN access and local recovery. Define the exact isolation and
restoration steps after observing the real topology. Verify the isolation rather
than inferring an outage from a closed browser tab. Existing customer traffic,
other tenants, Store, Jellyfin and unrelated router settings are outside this trial.

## Turning writes on for the trial

Paid PPPoE writes are off by default. Set `MM_BROADBAND_WRITES_ENABLED=true` for the trial only, on the deployment serving the spare router. It applies to every tenant on that deployment, so tell no client until the trial passes, and remove it again if the trial is paused. Check `/system/device-mode/print` first: `home` mode blocks Scheduler and Hotspot.

## Evidence register

Every row starts **pending**. Record actual results, timestamps and evidence;
software tests and a successful configuration API reply do not replace the
client login/traffic checks below.

| Case | Procedure and expected result | Status |
|---|---|---|
| Whole-router safety | With two dedicated test subscribers connected plus one unmanaged test PPPoE user, let one managed account expire. Only that account disconnects; the other two stay up. Repeat for an unused account with another user online: nobody is disconnected. | Pending |
| Script types | Read the guard with `/system script run` and the log. Confirm a valid account stays enabled across several 15-second passes (`:toarray` element comparisons work), and that `/system scheduler` `run-count` grows with no script errors. | Pending |
| Flash writes | Over a few hours, confirm the guard only writes when an account changes state. Note anything unexpected in `/system resource` write counters. | Pending |
| Captive portal on phones | On an Android and an iPhone test client, confirm the portal opens by itself and shows time left. The manual describes the `api.json` template but gives no setup steps. Check that the deployed HotSpot folder contains `api.json` and what DHCP actually sends. | Pending |
| Baseline and coexistence | Observe existing Hotspot behavior, install a dedicated PPPoE test subscriber through the app, and verify both clients authenticate and pass traffic concurrently. Capture relevant before/after state; existing profiles, hooks and customer users remain intact. | Pending |
| First-use validity | Install a new account without connecting it. Its period remains unstarted. Connect the intended PPPoE client; original first-use and expiry become anchored once. Reconnect without renewing; the same anchors remain. | Pending |
| Hotspot outage authentication | Preinstall a fresh test voucher, isolate the central services, then authenticate a new Hotspot client using the local portal. Confirm internet traffic, original activation evidence and applicable local limits. No open/free or shared-account fallback appears. | Pending |
| PPPoE outage authentication | With a preinstalled unused subscriber and central isolation in place, establish a fresh PPPoE connection. Confirm traffic and locally retained first-use/expiry evidence. Disconnect/reconnect; the paid period does not restart. | Pending |
| Local expiry | Let the purchased test period expire while the central services remain unreachable. Confirm the secret disables and the active PPPoE session disconnects. Reauthentication fails. Confirm Hotspot expiry/limits independently, using that voucher's actual configured policy. | Pending |
| App recovery | Restore the isolated path. Synchronize existing records and confirm original first-use/expiry, expired state and test identities are retained. Repeat synchronization; totals and payment counts do not increase. | Pending |
| Early and unused renewal | Renew unused and active test accounts through the app. Unused renewal adds prepaid days without starting the clock. Early active renewal extends the existing expiry once. Confirm exact operation identity and amount in the saved records. | Pending |
| Late renewal | Renew an expired test subscriber. The added period starts at successful router application; original first-use remains. Traffic resumes only after the intended renewal is applied. | Pending |
| Lost-reply recovery | Interrupt only a test operation's response under controlled conditions. Inspect its persisted pending/applied identity, then use the same saved operation to recover. Expiry extends once and Revenue records one payment. Do not create a replacement payment to recover an uncertain operation. | Pending |
| Suspension | Suspend an active test subscriber; confirm disconnect and denied login. Renew while suspended; it remains suspended. Resume separately; expiry is unchanged by resume. | Pending |
| Reboot and trusted time | Reboot only the spare router with local recovery available. Observe clock/NTP initialization and valid/expired account behavior after trusted time is established. Test any clock-regression scenario only in a separate isolated lab, because clock changes affect other services. | Pending |
| Missing or altered owned record | In a dedicated isolated fixture, confirm missing/altered enforcement is reported as uncertain and does not reset paid validity or acknowledge an unverified operation. Never use an existing customer account for this destructive fault scenario. | Pending |
| Module and tenant boundaries | Recheck the owner's Revenue, Store and separate Jellyfin workflow surfaces; compare unrelated records to the baseline. Tenant isolation is covered by software integration tests; any physical multi-tenant trial requires separately owned lab contexts, never foreign customer IDs. | Pending |

The production purchase minimum is one 24-hour day. Do not shorten production
validation, move the shared router clock forward or claim expiry acceptance from
a synthetic short-period test. Schedule the real expiry observation accordingly.
The PPPoE guard's nominal interval is 15 seconds plus router execution delay;
record the observed disable/disconnect delay rather than claiming a hard deadline.

## Result interpretation and completion

- Revenue represents applied owner-recorded create/renew payments. Logins,
  sampled traffic and repeated reads do not create payments. Record separately
  any real test payment entered; never represent a synthetic payment as settlement.
- Session bytes are observed maxima, not a durable offline journal. Sessions
  entirely between polls and counters lost at reboot cannot be reconstructed by
  this implementation. Do not treat this trial as acceptance for volume billing.
- A failed case remains open with its actual evidence. Restore the controlled
  isolation and preserve original customer settings. Suspend dedicated managed
  test accounts through the app when finished; retain their audit/payment records.
  Do not erase the shared ledger or remove shared enforcement to clean up a trial.
- Distinguish deployed software, observed router configuration and actual client
  authentication/traffic/outage results in the final acceptance record.

Operating details: [Router-local broadband accounts](ROUTER-LOCAL-BROADBAND.md).
