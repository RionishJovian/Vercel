# Guest access device workflow

The Internet Setup device workspace now has Monitor, Guest access and Config modes. Guest access connects the existing site/router selection to guest Wi-Fi and Hotspot settings, voucher issuance and live session monitoring. The application keeps its existing themes and Go/web stack.

Select a site and RouterBOARD, open Guest access, review the reported DHCP/Hotspot services, then use Open vouchers or View live guests. Those pages reuse the selected authorized router. The voucher inventory's device filter and creation target stay synchronized. Selecting a conflicting site clears the creation target rather than silently retaining another site's router. Back to device workspace restores the selected mode.

The workflow reads the existing `/sites`, `/routers`, `/fleet/health`, router network inventory, capabilities and voucher inventory APIs. Voucher creation continues to use `/guest-vouchers`. Codes are business records until the existing redemption service provisions and verifies access. The new UI does not add a provisioning endpoint, broaden write permissions or change RouterOS enforcement. DHCP/Hotspot records sharing an interface indicate reported configuration, not proof of successful client access. Session snapshots and activated voucher records remain distinct.

Unavailable reads are labelled unavailable; truncated voucher reads are labelled incomplete. Stale asynchronous voucher responses are discarded after changing device. No voucher code, secret or customer identity is added to browser storage; only existing site/router context and workspace mode are remembered.

Validation: the 10 related Node test scripts cover device/site isolation, lifecycle counts, creation/stock target synchronization, unavailable reads, late responses, escaping, role/write gates, review locking, guest actions and existing layout contracts. A local browser preview with synthetic API data verified the workflow, one synthetic issuance receipt and return navigation. Production deployment, actual payment and a new physical-router acceptance run are separate from this source change.
