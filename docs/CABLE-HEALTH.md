# Cable health

On the router page each wired port can show plain-language hints about its cable, from what the router itself reports. They appear under the port in the list and, on the port diagram, when you select a port; a port with a likely problem has an amber badge. All reads are fixed and read-only.

## What is read (per port, every poll)
- `/interface ethernet monitor <port> once`: status, negotiated rate and duplex, **auto-negotiation result** (`done`, `failed`, `incomplete`, `disabled`), `supported` modes (the fastest one is the port's top speed) and `link-partner-advertising` (the fastest the device at the other end offers). If this fuller read is refused, the shorter proven read is used instead, so a port never loses its link rate.
- `/interface ethernet print stats`: the physical receive error counters `rx-fcs-error`, `rx-align-error`, `rx-fragment`, `rx-runt`, `rx-carrier-error`, summed per port. A port that reports none of them has **no** error figure; it is never shown as having zero errors. Counters vary by model and the manual gives no thresholds, which is why the app looks at the **change** over time, not the total.

## The trend
The telemetry service keeps about five minutes of readings per port and reports how many errors were added in that window. A counter that goes down (a reboot or `reset-counters`) starts the history again, so a reset is never read as a burst. After a gap longer than the window there is no trend until two readings exist.

## The hints (rules are in `app/src/pages/routers/cableHealth.ts`, all tested)
| Situation | Hint | Flag |
|---|---|---|
| Link slower than the port's top speed, and the other end offers more | The cable or a connector is the likely cause | yes |
| Link slower than the port's top speed, but the other end tops out there | Linked at the fastest speed the connected device offers | no |
| Link slower, far end unknown | Linked below the top speed; cable or device may limit it | no |
| Link slower, auto-negotiation off | Speed is set by hand | no |
| Negotiation `failed` or `incomplete` | Could not agree a speed; try another cable or restart the device | yes |
| Half duplex | Usually a cable or settings problem | yes |
| Errors added in the last minutes | Still increasing: damaged cable or loose connector likely | yes |
| Errors counted earlier, none recently | Informational | no |
| Errors counted, no trend yet | Watching for new ones | no |

A port with no link or a disabled port is never treated as faulty. Nothing is said when there is no data.

## Not done on purpose
`/interface ethernet cable-test` is not run. The manual does not say whether it drops a live link and only some models support it, so it would have to be an explicit, confirmed action. That is a possible later step.

## Not yet verified on a real router
The extra monitor fields and the stats command follow the RouterOS 7.25 manual but have not run on a real RouterBOARD. `link-partner-advertising` is empty on SFP links by design. To check: read the router's telemetry JSON after deploy (`negotiation`, `max_rate_mbps`, `partner_max_mbps`, `phy_errors`).
