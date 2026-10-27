# ADR-0008: Identity through a self-hosted Zitadel, lastro as a resource server

**Status:** Proposed (supersedes ADR-0004 once accepted). The lastro side is built and stays off until `LASTRO_OIDC_ISSUER` is set; the spike with Zitadel decides.
**Date:** 2026-10-27
**Deciders:** Felipe

## Context

ADR-0004 put Cloudflare Access in front of lastro and had the app verify the Access JWT. That works for a browser, but not for the clients that come next:

- **A CLI on my laptop** (`lastro connect <url>`), talking to the API instead of the database.
- **An MCP server** that Claude connects to (see ADR-0009). MCP clients speak OAuth 2.1: they discover the authorization server from the resource, then get a token through a redirect. They cannot get through the Access login wall.
- **A phone app**, later, using Authorization Code with PKCE.

An identity provider in the cluster can also serve other services that speak OIDC (Argo CD, Grafana), so that is a bonus, not the reason. lastro's own clients justify it alone.

## Decision

### lastro is a resource server and knows nothing about the provider

- `platform/auth` verifies OIDC/OAuth JWTs from one configured issuer. It reads discovery and the JWKS (the key cache and refresh limit from `cfaccess.go` stay), accepts RS256 and ES256, and checks issuer, audience and the validity window.
- **Only the owner gets in.** The provider will hold other users of other services, so a valid token is not enough: its `sub` must be the owner's (`LASTRO_OIDC_OWNER_SUBJECT`). That works with any provider and names one exact user. In Zitadel, the project also checks that a user is granted on it before issuing a token, a second lock.
- **Permissions by client.** `Identity` gains `ClientID` and a permission (`read` or `write`) that lastro derives from the client (`azp`, then `client_id`). Writing is opt-in: the web session, the CLI's client and the clients in `LASTRO_OIDC_WRITE_CLIENTS` write; every other client, Claude's among them, only reads. The middleware lets `GET` through with `read` and asks `write` for every other method, so no module changes. MCP is the exception: every call is a `POST`, so `/mcp` takes any of the owner's bearer tokens and its tools are read-only by construction (ADR-0009).
- **The web app uses a BFF.** The server is a confidential client: `/auth/login`, `/auth/callback` and `/auth/logout` run Authorization Code with PKCE, `state` and `nonce`, and the browser only holds an `HttpOnly`, `SameSite=Lax` session cookie. No token reaches JavaScript.
- **The session is a sealed cookie**, not a table: the owner and an expiry, encrypted and authenticated with AES-256-GCM under `LASTRO_SESSION_KEY`, for 14 days. No migration and no `auth` schema. The cost is that one session cannot be revoked alone; rotating the key signs every session out, which is enough for one user. An unsafe request carrying a session must come from lastro's own origin, on top of `SameSite=Lax`.
- **Bearer tokens** for everything else: CLI, MCP and the future app.
- **Discovery for OAuth clients:** `/.well-known/oauth-protected-resource` and `/.well-known/oauth-protected-resource/mcp` (RFC 9728), and a `401` with `WWW-Authenticate: Bearer resource_metadata="…"`. The root document also names the CLI's public client (`lastro_cli_client_id`), so `lastro connect` needs only the server's URL.
- The middleware tries the session cookie, then the bearer token, then the Access JWT while Access is still configured. A bearer token that fails never falls through to the next way in. `LASTRO_AUTH_DISABLED` keeps its rule: dev only.
- Discovery happens on first use and is cached, so lastro starts while the provider is down.
- OAuth plumbing uses `golang.org/x/oauth2`. Writing PKCE and the code exchange by hand is the kind of code where a vetted library earns the dependency. JWT verification stays on the standard library, as today.

### The provider is Zitadel, on saradomin

- One Zitadel instance at `auth.ftm.dev.br`, through the saradomin Cloudflare tunnel, with its database on `postgres-main`, which is already backed up to R2.
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

### Why saradomin and not zamorak

- lastro is the main client. With the provider on another site, logging in to lastro needs both sites up. On saradomin it shares lastro's failure domain: if the house is down, lastro is down anyway.
- `postgres-main` already has a backup to R2. Zamorak's Postgres has none.
- The cost: zamorak services that move to Zitadel depend on the house. Few will, so that is accepted. Zamorak's Argo CD can stay behind Cloudflare Access.

### What stays out of the provider

- **Vaultwarden.** Its master password is the key that encrypts the vault, and SSO does not replace it. Vaultwarden is also where the provider's recovery codes and admin credentials live, so it cannot depend on the provider.
- Apps that have their own login and no OIDC (n8n's community edition, for one) are not worth a second login in front of them.

## Consequences

- Losing the provider's database means losing users, passkeys and clients, so Zitadel's database joins `postgres-main`'s backup and a restore is tested once.
- If Zitadel is down while lastro is up, nobody can start a new session. Existing sessions and unexpired tokens keep working, because the JWKS is cached and lastro sessions are not re-checked against the provider on every request.
- The break-glass for the provider (recovery codes, the instance admin) lives in Vaultwarden, on zamorak.
- lastro reaches Zitadel through its public URL. The issuer must match what tokens carry, so no internal shortcut.
- Once accepted:
  - ADR-0004 becomes Superseded.
  - `AGENTS.md`'s security section changes from "every route behind Cloudflare Access" to "every route behind `platform/auth`".
  - The Access application leaves gielinor's Terraform.
- Validate in the first M7 task:
  - JWT access tokens carry the project ID in `aud` and the roles claim.
  - The device flow works for `lastro-cli`.
  - Claude completes the flow with the pre-registered client.
