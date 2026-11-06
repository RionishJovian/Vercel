# WebFig Cloud Access

Opens a customer's own RouterOS WebFig through Magic Hub, in its own browser tab. The router checks its own password; the app never sees it.

**State on 2026-11-06:** off in production. Signing in through it has not been tested on a real router.

## Switches

| Variable | Values | Effect |
|---|---|---|
| `MM_WEBFIG_ACCESS` | unset, `open`, `locked` | Unset or anything else is off. `open` lets ADMIN and above use it. `locked` shows it as locked to customers; the check for a paid key or pass goes in `webfig.Entitled`. |
| `MM_WEBFIG_HOST` | a domain, such as `routers.example` | Required. Sessions open on random subdomains of it. |

## Hosting it

The domain must be **separate from the console's**. Anything under `megamagic.tech` is refused and the feature stays off.

A router's owner decides what the proxy gets back, for example by forwarding the Hub's connection to their own server. So every reply is treated as hostile:

- Each session gets its own random subdomain, which makes it its own browser origin. One session's page can't touch another's, even in the same browser.
- A separate domain stops that page setting cookies on the console or counting as the same site.
- The proxy removes cookies that target other hosts or its own session, and headers such as `Service-Worker-Allowed`. It refuses background-script requests and paths that could leave the allow-list.

To serve it, you need:

1. Wildcard DNS: `*.routers.example` pointing at the app server.
2. A wildcard TLS certificate. Caddy can only get one with the DNS challenge, which needs a DNS provider plugin and API token.
3. A Caddy site for the domain and its subdomains that forwards to the API and keeps the original `Host`.

None of this is set up yet.

## Before turning it on

- Sign in through it on a real router, and use a page or two in WebFig.
- On a router with WebFig off, review, allow, use and remove the access, and check `/ip service` afterwards.
- The Magic Hub disconnect script removes the firewall rule but leaves the Hub in the web service's allowed addresses. If this feature turned `www` on, it stays on but is limited to the Hub, which can no longer reach it.
