# Automatic Magic Hub management

Generate the script, paste the entire script into WinBox New Terminal within
30 minutes, then select **Finish connection**. The generated account accepts
traffic from the app's private address, 10.77.0.9, and uses a dedicated RSA SSH
key. Existing management accounts are available through the Advanced section.

The backend requires tenant administrator permissions and a recent authenticated
Hub handshake. It derives the destination from the signed Hub allocation, checks
the outbound CIDR policy, authenticates using the generated SSH key, and requires
the per-enrollment 256-bit proof and matching WireGuard public key from the router.
Only then does it pin the SSH host fingerprint and encrypt the SSH private key in
the existing device credential store. Future connections enforce that fingerprint.
This replaces manual LAN fingerprint entry with trust through the authenticated
WireGuard peer plus the locally pasted enrollment proof; it is not independent
verification over the LAN.

Pending scripts and keys remain in application memory for 30 minutes. A backend
restart before completion requires generating another script. A configured
WireGuard interface retains its identity when a script is regenerated; an existing
Hub allocation whose original private-key script was lost before installation
requires the existing enrollment-repair workflow. Verified management credentials
survive application restarts through encrypted database storage.

The script creates a dedicated account/group, imports only the public SSH key,
and places a tunnel-interface/input rule for TCP 22 from 10.77.0.9 ahead of input
drop rules. It enables a disabled SSH service with its service address restricted
to 10.77.0.9/32. For an already enabled SSH service, it preserves existing service
addresses and adds the app address when the list is restricted. A custom SSH port
requires Advanced setup. User ownership and existing managed firewall rules are
checked before replacing managed resources. WAN/LAN interfaces are not reset.

Software checks cover authorization, stale handshakes, destination restrictions,
existing tunnel preservation, SSH proof rejection, fingerprint capture and bounded
output. Physical RouterOS script execution, actual firewall ordering, Hub-to-router
SSH forwarding and subsequent voucher/portal management still require a MikroTik
network test.
