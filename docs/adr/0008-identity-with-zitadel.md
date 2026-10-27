# ADR-0008: Identity through a self-hosted Zitadel, lastro as a resource server

**Status:** Proposed (supersedes ADR-0004 once accepted)
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

ADR-0004 put Cloudflare Access in front of lastro and had the app verify the Access JWT. That works for a browser, but not for the clients that come next:

- **A CLI on my laptop** (`lastro connect <url>`), talking to the API instead of the database.
- **An MCP server** that Claude connects to (see ADR-0009). MCP clients speak OAuth 2.1: they discover the authorization server from the resource, then get a token through a redirect. They cannot get through the Access login wall.
- **A phone app**, later, using Authorization Code with PKCE.

I also want one identity provider for the services of both clusters (Argo CD, Vaultwarden, n8n, Grafana…), not one login per app.

## Decision

### lastro is a resource server and knows nothing about the provider

- `platform/auth` verifies OIDC/OAuth JWTs from one configured issuer. It reads discovery and the JWKS (the key cache and refresh limit from `cfaccess.go` stay), accepts RS256 and ES256, and checks issuer, audience and the validity window.
- **Only the owner gets in.** The provider will hold other users of other services, so a valid token is not enough: the token must carry lastro's `owner` project role, and the project only issues tokens to users granted on it.
- **Permissions by client.** `Identity` gains `ClientID` and a permission (`read` or `write`) that lastro derives from the client (`azp`/`client_id`) through config: the web app and the CLI write, the MCP client reads. The middleware lets `GET` through with `read` and asks `write` for every other method, so no module changes.
- **The web app uses a BFF.** The server is a confidential client: `/auth/login`, `/auth/callback` and `/auth/logout` run Authorization Code with PKCE, and the browser only holds an `HttpOnly`, `SameSite=Lax` session cookie. Sessions live in Postgres, in an `auth` schema. No token reaches JavaScript.
- **Bearer tokens** for everything else: CLI, MCP and the future app.
- **Discovery for OAuth clients:** `/.well-known/oauth-protected-resource` (RFC 9728), and a `401` with `WWW-Authenticate: Bearer resource_metadata="…"`.
- The middleware tries the session cookie, then the bearer token. `LASTRO_AUTH_DISABLED` keeps its rule: dev only.
- OAuth plumbing uses `golang.org/x/oauth2`. Writing PKCE and the code exchange by hand is the kind of code where a vetted library earns the dependency. JWT verification stays on the standard library, as today.

### The provider is Zitadel, on zamorak

- One Zitadel instance for both clusters, at `auth.ftm.dev.br`, through the zamorak Cloudflare tunnel.
- lastro is a Zitadel project with JWT access tokens and these applications:
  - `lastro-web`: Web, confidential, code with PKCE.
  - `lastro-cli`: Native, device code (RFC 8628) and refresh tokens.
  - `lastro-claude`: Web, pre-registered for Claude's connector (redirect `https://claude.ai/api/mcp/auth_callback`), with its client ID and secret typed into the connector's advanced settings.
  - `lastro-app`: Native, PKCE, later.
- Login with a passkey; self-registration off.
- Zitadel and its configuration are code in gielinor, like the rest of the cluster.

### Cloudflare Access leaves lastro

The tunnel stays (no open port at home), and so does Cloudflare's WAF and rate limiting. The Access application for lastro is removed, because OAuth clients cannot pass through it.

## Options considered

- **Keep Cloudflare Access** (and its OIDC/MCP features): no new infrastructure, but bad for the CLI and the app, and the authorization server stays a vendor's. Rejected.
- **Keycloak:** the most complete and mature option, with experimental CIMD and established DCR for MCP. But it is a JVM with a heavier footprint and admin, for one user. It is the fallback if Zitadel falls short, and switching only touches configuration.
- **Authentik:** good homelab experience and forward auth. With the provider on Cloudflare's tunnel, Access can use Zitadel as its identity source for apps without OIDC, which covers that gap. Not chosen.
- **Pocket ID / Authelia:** light, but missing the device flow or client management I need.
- **lastro as its own authorization server:** the attack surface ADR-0004 already refused. Rejected.
- **Zitadel:** Go core and Postgres only, the database the clusters already run. Certified OIDC, with device code, PKCE, passkeys, roles and a management API. Chosen.

### Zitadel's gaps, and why they do not block

- **Dynamic Client Registration** (RFC 7591) only landed in mid-2026 and is off by default. **CIMD** is not released yet. Claude accepts a pre-registered client, which is the right shape for a single user anyway.
- **Resource indicators** (RFC 8707): Zitadel ignores the `resource` parameter. lastro checks the project ID in `aud` instead, which is enough with a single resource server.
- **Login v2 is a separate container** (TypeScript) since v4, so the core is Go but the deploy is two workloads.
- Zitadel has had fast major versions with breaking changes (v3 dropped CockroachDB, v4 split the login). Upgrades get read before they get applied.
- The licence is AGPL since v3, which costs nothing for self-hosting.

### Why zamorak and not saradomin

- The provider must stay up when what it protects is down. Zamorak is in the cloud, away from home power and internet. When the house goes dark, the apps on zamorak still log in.
- Zamorak already runs the shared services (Argo CD, Vaultwarden, n8n).
- Financial data stays at home. Zamorak only holds identities.

## Consequences

- **Zamorak's Postgres needs a backup before Zitadel goes there.** Today only saradomin's `postgres-main` is backed up to R2. Losing the provider's database means losing users, passkeys and clients.
- If zamorak is down, nobody can start a new session in lastro. Existing sessions and unexpired tokens keep working, because the JWKS is cached. lastro sessions are not re-checked against the provider on every request.
- If the house is down, lastro is down, as it is today.
- lastro reaches Zitadel through its public URL. The issuer must match what tokens carry, so no internal shortcut.
- Once accepted:
  - ADR-0004 becomes Superseded.
  - `AGENTS.md`'s security section changes from "every route behind Cloudflare Access" to "every route behind `platform/auth`".
  - The Access application leaves gielinor's Terraform.
- Validate in the first M7 task:
  - JWT access tokens carry the project ID in `aud` and the roles claim.
  - The device flow works for `lastro-cli`.
  - Claude completes the flow with the pre-registered client.
  - Images exist for zamorak's architecture.
