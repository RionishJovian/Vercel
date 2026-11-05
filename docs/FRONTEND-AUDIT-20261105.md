# Console page audit: 2026-11-05

Scope: all 52 console routes in `app/`, rendered at phone width (390 px) in English against a local stub API, plus the navigation and page code. The audit itself changed nothing. Items 1 to 3 were then built on `feat/console-cleanup`.

**Limits:** the stub used sample data and a platform-developer login, so empty and error states appear more than they would with real data, and a client sees fewer tiles. Burmese and wide desktop layouts were not measured. Burmese text usually runs longer, so the text figures are likely to be worse there.

## 1. Wrong titles

| Where | Problem | Fix |
|---|---|---|
| More tab | The tab says **More**, the page says **Account**, and a section inside it is also **Account**. | Title it **More**. |
| `#/profile` | The tile says **Your profile**, but the page title is the person's name. It is blank if the name is unset. | Title it **Your profile** and show the name below. |
| `#/router/map` | No page title at all. | Add **Network map**. |
| `#/network` and `#/network/wan` | The same page has three names: **Internet setup** (title), **Internet connections** (pill), **Internet uplinks** (section). | Use one name. |
| `#/vouchers` | **Internet vouchers** (title), **Vouchers** (tab), **Vouchers** (sub-tab). Jellyfin also has a **Vouchers** sub-tab. | Rename the sub-tab **Codes**. Keep Jellyfin's as **Media vouchers**. |
| Vouchers and Jellyfin | Both have a sub-tab named **Guest page**, for different portals. | **Internet guest page** and **Media guest page**. |
| `#/platform/*` | All six sub-pages share the title **Platform**. | Keep the title, but make the pill the clear location. |
| `#/team` | **Team (legacy)** is shown as a live page. | Rename or hide it (see section 3). |
| Network pages | A single task repeats its name: **VLANs** shows as title, card and section. | Show it once. |
| Browser tab | Always says "Mega Magic" on every page. | Set the page title per route. |

## 2. Dislocated pages

The tile a person taps, the tab that lights up and the Back link disagree.

| Page | Opened from | Tab lit | Back goes to |
|---|---|---|---|
| Revenue | More → Business | Home | Home |
| Internet setup | More → Operations | Router | Router |

More pages that belong to Router but sit under More: **Magic Hub, Topology, Devices, Managed devices, Fleet health, Backups**. They light the More tab and go Back to More, even though the Router page links to Hub and Topology.

- **Router sub-tabs:** the **Internet setup** sub-tab leaves the Router sub-tabs entirely, because it is a link to another page dressed as a tab.
- **Network pills:** **New network setup** sits in the same row as the sections, but it opens a different kind of page with no pills, so people lose their place.

## 3. Duplicates

| Same thing | Appears as |
|---|---|
| Internet setup | On the Router page 3 times (sub-tab, section of tiles, Router tools pill) and again in More. |
| Magic Hub | Router tools pill, More tile, and a chip on router cards. |
| Network picture | **Map** (`#/router/map`) and **Topology** (`#/topology`). |
| Router list | **Routers**, **Devices**, **Managed devices**, **Fleet health**, **Magic Hub** all list the same routers. |
| Router tools | **Router tools** section on the Router page, and a differently scoped **Router tools** card on router detail. |
| Incidents | **Incidents** (More) and **Incidents** (Platform), the same name for different scopes. |
| Delete record | Two pills at the top of the Router page, added for findability. Destructive actions should sit on the record. |
| Task descriptions | On network pages the card's description is repeated word for word under the section heading. |
| Icons | Internet setup and Topology share one icon. Devices and Managed devices share another. |
| More grid | 20+ tiles with no priority for a client. |

## 4. Explanations taking over the page

Measured on a phone: how far someone scrolls before the first usable control, and the share of the page's text that is explanation.

| Page | First control at | Explanation share |
|---|---|---|
| LAN and Hotspot | **1,857 px** (2.2 screens) | 73% |
| IP and DHCP | 1,354 px | 64% |
| PPPoE (Customer internet) | 1,115 px | 46% |
| Jellyfin guest page | 1,110 px | 56% |
| Internet guest page | 1,097 px | 64% |
| Internet connections | 958–1,048 px | 50–55% |
| Protection | 1,019 px | 64% |
| VLANs | 930 px | 67% |
| Fair share QoS | 935 px | 52% |
| Wi-Fi | 914 px | 64% |
| Revenue | no input on the page | 54% |
| Store | no input on the page | 65% |

Network pages are the worst. Before any form they stack: section pills, a warning, a Refresh button, an "About these settings" box, then a card with a paragraph for **every** task (10 on LAN and Hotspot), then the description again.

Other long passages sit permanently on Vouchers (`v.hint`), the guest page editors, Store, Revenue, Topology and Backups.

**The app already has the right tool:** `PageHead` keeps the page's explanation behind a **?** button. It is used on 16 pages, but not on the network, voucher, guest page, revenue or store pages. Only the Guide should read as prose.

## 5. Found along the way

- **Blank screen on any page error.** There is no error boundary. One unexpected reply blanked the whole app, tab bar included, and it stayed blank until a reload (seen on PPPoE accounts). A page failure should show an error inside that page.
- **Raw technical errors reach users.** For example: "Cannot read properties of undefined (reading 'pending_confirm')" in the red banner. The message came from the stub's empty reply, but the display path is real (`errMsg`).
- **Magic Dude and the tab bar cover content.** The floating button sits over tiles and form fields. Check that the bottom padding clears both.
- **Unclear labels.** "lan" and "wireguard" appear as raw values on cards.

## 6. Proposed order of work

1. **Stop the blank screen and raw errors.** Small, and affects every page.
2. **Titles** in section 1. Small, and no page moves.
3. **Explanations behind ?**: network pages first. One line per task instead of a paragraph, and the form directly after choosing. Then the guest page editors, Vouchers, Store, Revenue, Topology and Backups.
4. **Structure:** make Router the home of Hub, Internet setup, Map, Devices and Fleet health. Merge the router lists and Map with Topology. Fix Back and tab lighting. Old links keep working through redirects.
5. **Trim More** to what a client needs, and give duplicate icons their own.

Item 4 changes where people find things, so it needs the owner's agreement on the final layout first.

## 7. Proposed layout for item 4 (needs the owner's agreement)

The five tabs stay. Each page lives in one place, and its tab, its Back link and the way in all agree.

**Router tab**, as sub-tabs:

| Sub-tab | What it holds | Replaces |
|---|---|---|
| Routers | Router cards with health. Tapping one opens its detail page: connected devices, backups, tools and Delete. | Routers, Devices, Fleet health, Backups, the two Delete pills |
| Map | One network picture. | Map and Topology |
| Internet setup | The list of areas and New network setup. Stays inside the Router tab. Today this link opens directly on Internet connections, so this one changes what you see first. | Router tile section, Router tools pill and the More tile |
| Magic Hub | Site, connection and maintenance. | Magic Hub, Sites |
| Guests | Who is online now. | Guests under More |

**More tab** for a client: Store, Incidents, Security, Activity, Your profile, Guide, language and theme, Sign out. The platform team also sees Platform, Managed devices and the developer tools.

**Revenue** opens from Home only, so the lit tab and Back already agree.

Old links such as `#/hub`, `#/topology` and `#/devices` keep working and redirect to the new place. No data, permission or router behaviour changes.
