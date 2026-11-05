# Router trial for topology device controls

These change a live router. They stay refused until this trial passes on the lab hAP ax2 and the switch is set:

```
MM_NETWORK_CONTROLS_ENABLED=true
MM_DEVICE_CONTROLS_ENABLED=true
```

Use only the owner's hAP ax2, on its local network from the Mac, so a mistake can be undone in WinBox. Never use a customer's router or the friend's hAP ax3. The switch applies to every account on a server, so do not turn it on in production just to run this.

Needs: the hAP ax2 powered and linked to the Mac, one phone or PC to block, and one spare device on a LAN port. PoE steps also need a device powered by the router.

## Before starting

| Step | Pass when |
|---|---|
| Open Topology | Ports show the same up and down state as WinBox. Interface names have no `,bridge`. |
| Compare labels and models | Each matches WinBox. The Wi-Fi radios and their clients appear. |
| Take a backup | A fresh export is saved off the router. |

## Block a device with no Hotspot session

Use a device on a network with no Hotspot, because a Hotspot guest gets the guest action instead.

| Step | Pass when |
|---|---|
| Review, then apply, Block internet | It is the first static rule in `/ip firewall filter`, above FastTrack, with comment `mm-block-<MAC>`. |
| Browse on the device, including a page already open | No internet. An open connection stops too, which proves the tracked connections were cleared. |
| Reach another device on the same switch | Still works, as the review screen warned. Record it. |
| Remove the block | The rule is gone and the internet returns. |
| Block the upstream modem, or the router's own MAC | Refused, and nothing changes. |
| Repeat with hardware offload on and FastTrack on | Same result. |

## Port label

| Step | Pass when |
|---|---|
| Label a port, then clear it | WinBox shows each change. The page keeps loading. |
| Try a Burmese label | Refused with a clear message. |

## Turn a port off

Use the spare device's port, never the uplink or the port the Mac is on.

| Step | Pass when |
|---|---|
| Try the uplink port | Refused. |
| Review, then apply, Turn port off | The port shows off, the device loses its link, and the Keep panel counts down. |
| Press Revert now | The port comes back and the device reconnects. |
| Turn it off again and press nothing | The router turns it back on by itself after 5 minutes. |
| Turn it off again and press Keep | It stays off, and survives a page reload. Then turn it back on. |

## Restart PoE power

Only if the router can power a device on this model. If it can't, record that and leave this open for a PoE model.

| Step | Pass when |
|---|---|
| Open a port with PoE, and one without | Only the PoE port offers Restart power. |
| Review, then apply | The powered device really loses power and comes back. This is the step forum reports doubt. |
| Check `poe-out` afterwards | Unchanged. |

## Reserve and limit

| Step | Pass when |
|---|---|
| Reserve this IP for a device with a dynamic lease | The form is filled in. The lease becomes static, and the device keeps its address after reconnecting. |
| Limit speed | The queue targets that device, and a speed test agrees. |

## Reboot

This needs the server's approval path, so it can only run on a server with Telegram approval set up. Record it as not run otherwise.

| Step | Pass when |
|---|---|
| Reboot with approval | The page goes from waiting to the real result, and the history shows it. |
| Let the approval time out | The page says it was not done, and the router did not reboot. |

## Afterwards

Remove every test rule, label and reservation, compare against the backup, and record RouterOS version, board and date here. Passing this does not turn the switch on: that is a separate decision by the owner.
