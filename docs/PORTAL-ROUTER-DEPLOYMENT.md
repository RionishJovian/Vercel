# Portal deployment to a router

The portal editor now separates **Save** (hosted branding, access methods and
plans) from **Deploy to router** (install the HotSpot entrypoint). Select a
gateway, choose its existing HotSpot server profile, review the read-only plan,
then explicitly deploy. Existing Magic Hub management credentials are reused;
users do not enter SSH credentials again.

ADMIN/OWNER/DEVELOPER routes use the existing tenant, subscription, CSRF and
private pinned-SSH controls. The preview digest binds business, device, profile,
version and existing login content. Changed state requires a fresh preview.
No migration is needed.

Deployment clones the full existing HTML directory to a new `mm-portal-*`
directory (under `flash/` when present), writes a small HTTPS redirect carrying
the device ID and RouterOS client MAC/IP, and switches only the selected profile.
Other profiles and original files stay untouched. A verified copy of the original
login page is retained inside the new persistent directory and reported as the
backup path. A failed switch restores and reads back the
original directory and removes only a newly added managed access rule. Staged
copies are retained for inspection; they are not automatically deleted. On
routers whose original directory is RAM-backed, that directory does not survive
a reboot; the new deployed directory and login-page backup are flash-backed.

The access rule allows TCP 443 to `mikromagic.com`, scoped to the selected
HotSpot server. RouterOS resolves the hostname into IP access rules; shared CDN
addresses may also serve other hosts. This is disclosed in the deployment
review, not represented as a hostname-exclusive TLS restriction. No wildcard
banking allowlist, guest bypass, public management port or WAN changes are made.

Prerequisites are a completed management connection, an existing HotSpot login
page, MAC authentication for app-managed guests, a profile used by exactly one
enabled HotSpot server, and RouterOS support for `/file copy`. External HTML
overrides fail closed. Unsupported file operations leave the active profile
unchanged. Readback verifies configuration/files, not actual guest Internet
access: test a guest device after applying. Physical verification remains
pending until hardware is available.

Once the entrypoint is deployed, saved branding/plans load from the hosted
portal and need no repeated router upload. External branding image hosts may
require separate operator-reviewed access rules; this deployment permits only
the app host.

References: [RouterOS file operations](https://help.mikrotik.com/docs/spaces/ROS/pages/2555971/Files)
and [HotSpot profiles and walled garden](https://help.mikrotik.com/docs/spaces/ROS/pages/56459266/HotSpot+-+Captive+portal).
