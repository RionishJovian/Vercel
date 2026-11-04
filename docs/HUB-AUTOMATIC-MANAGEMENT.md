# Automatic Magic Hub management

The console workflow is **Create → Paste → Check**. Authorize and generate one
complete script, paste it into WinBox New Terminal within 30 minutes, then select
**Check connection**. The wizard never calls the separate router-key bootstrap
endpoint: mixing that endpoint with quick enrollment creates incompatible keys.
The generated account accepts traffic from the configured app-side private /32
and uses a dedicated RSA SSH key. The user-facing setup has no manual SSH
credential form.

**Check existing connection** is available immediately, without generating or
pasting another script. A saved-management conflict also opens verification
directly. Opening the check step does not mark the script steps complete.
Only a `management_authenticated` server response confirms connection success.

The backend requires tenant administrator permissions and a recent authenticated
Hub handshake. It derives the destination from the signed Hub allocation, checks
the outbound CIDR policy, authenticates using the generated SSH key, and requires
the per-enrollment 256-bit proof and matching WireGuard public key from the router.
Only then does it pin the SSH host fingerprint and encrypt the SSH private key in
the existing device credential store. Future connections enforce that fingerprint.
This replaces manual LAN fingerprint entry with trust through the authenticated
WireGuard peer plus the locally pasted enrollment proof; it is not independent
verification over the LAN.

Pending scripts and keys have a 30-minute lifetime and use the encrypted pending
enrollment store in the configured production path, preserving an unexpired
attempt across backend restarts. Regeneration reuses the pending identity; it
does not rotate an established peer. Expired installation material requires the
explicit existing-tunnel repair workflow, which only works when the router still
has the original allocated key. Verified management credentials survive application
restarts through encrypted database storage.

An interface left by the former separate preparation script can have a different
key from the Hub allocation. Removing that step fixes new installations but does
not repair such a partial router automatically. Preserve the key-mismatch guard:
inspect the actual interface/public key and allocation before choosing recovery.
Do not delete the interface, reset the router or silently replace a key. A STOP
message means setup is incomplete; earlier statements in the script may already
have applied, so it is not proof that the router was unchanged.

The script creates a dedicated account/group, imports only the public SSH key,
and places a tunnel-interface/input rule for TCP 22 from the configured app /32 ahead of input
drop rules. It enables a disabled SSH service with its service address restricted
to that /32. For an already enabled SSH service, it preserves existing service
addresses and adds the app address when the list is restricted. A custom SSH port
requires operator-assisted configuration. User ownership and existing managed firewall rules are
checked before replacing managed resources. WAN/LAN interfaces are not reset.

Software checks cover authorization, stale handshakes, destination restrictions,
existing tunnel preservation, SSH proof rejection, fingerprint capture and bounded
output. Physical RouterOS script execution, actual firewall ordering, Hub-to-router
SSH forwarding and subsequent voucher/portal management still require a MikroTik
network test.
