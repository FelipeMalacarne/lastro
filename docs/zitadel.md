# Signing in with Zitadel

How to set up the provider for lastro ([ADR-0008](adr/0008-identity-with-zitadel.md)) and connect the clients: the web app, the `lastro` CLI and Claude through MCP ([ADR-0009](adr/0009-mcp-server-for-claude.md)). lastro only verifies tokens, so everything here is configuration: nothing in the code names Zitadel.

Until `LASTRO_OIDC_ISSUER` is set, none of this is on and Cloudflare Access keeps working as before.

## 1. The project

In the Zitadel console, create a project named `lastro`:

- Turn on **Check authorization on authentication**, so only users granted on the project get tokens for it. lastro checks the owner again by `sub`.
- Grant your user on the project (Authorizations).
- Write down the **project ID**. It becomes `LASTRO_OIDC_AUDIENCE`, and the scope `urn:zitadel:iam:org:project:id:<project ID>:aud` puts it in every token's `aud`.
- Write down your **user ID** (Users, your user). It becomes `LASTRO_OIDC_OWNER_SUBJECT`.

## 2. The applications

| Application | Type | Settings | Goes to |
|---|---|---|---|
| `lastro-web` | Web, code flow, client secret | redirect `https://lastro.ftm.dev.br/auth/callback`; post-logout redirect `https://lastro.ftm.dev.br/`; **User info inside ID token** on, so the session has the email | `LASTRO_OIDC_WEB_CLIENT_ID`, `LASTRO_OIDC_WEB_CLIENT_SECRET` |
| `lastro-cli` | Native, no secret | grant types **Device code** and **Refresh token**; access token type **JWT** | `LASTRO_OIDC_CLI_CLIENT_ID` |
| `lastro-claude` | Web, code flow with PKCE and a client secret | redirect `https://claude.ai/api/mcp/auth_callback` (and `https://claude.com/api/mcp/auth_callback`); grant type **Refresh token**; access token type **JWT** | the Claude connector's advanced settings |

Access tokens must be JWTs: lastro verifies them locally against the provider's keys and does not call introspection.

## 3. lastro's settings

In gielinor's SOPS Secret (`clusters/saradomin/apps/lastro/secrets.yaml`), the keys in [`secret.example.yaml`](../deploy/k8s/overlays/saradomin/secret.example.yaml):

```sh
LASTRO_PUBLIC_URL=https://lastro.ftm.dev.br
LASTRO_OIDC_ISSUER=https://auth.ftm.dev.br
LASTRO_OIDC_AUDIENCE=<project ID>
LASTRO_OIDC_OWNER_SUBJECT=<your user ID>
LASTRO_OIDC_SCOPES=urn:zitadel:iam:org:project:id:<project ID>:aud
LASTRO_OIDC_WEB_CLIENT_ID=<lastro-web>
LASTRO_OIDC_WEB_CLIENT_SECRET=<lastro-web secret>
LASTRO_SESSION_KEY=<openssl rand -base64 32>
LASTRO_OIDC_CLI_CLIENT_ID=<lastro-cli>
```

`lastro-claude` is not listed anywhere: a client lastro does not know only reads. List a client in `LASTRO_OIDC_WRITE_CLIENTS` only to let it write.

Rotating `LASTRO_SESSION_KEY` signs every web session out.

## 4. While Cloudflare Access is still in front

Both ways in work at once: a request with a session or a bearer token is checked against Zitadel, and one with only the Access JWT against Access. To try the web login before the switch, open `https://lastro.ftm.dev.br/auth/login`.

OAuth clients cannot get through Access's login page. Until the switch, the CLI and Claude need an Access **bypass** policy on `/mcp`, `/api/` with a bearer token and `/.well-known/oauth-protected-resource`, or they wait for the switch.

## 5. Clients

- **CLI:** `lastro connect https://lastro.ftm.dev.br` shows a link and a code. Confirm it in the browser, and the token stays in `~/.config/lastro/config.json` (0600), refreshed by itself. `lastro whoami` shows who the server sees.
- **Claude:** Settings → Connectors → Add custom connector, with URL `https://lastro.ftm.dev.br/mcp`. Under Advanced settings, paste `lastro-claude`'s client ID and secret. Claude finds the provider through `/.well-known/oauth-protected-resource/mcp`. Removing `lastro-claude` in Zitadel cuts it off.

## 6. The switch

Once the web login, the CLI and Claude work:

1. Remove `LASTRO_CF_ACCESS_TEAM_DOMAIN` and `LASTRO_CF_ACCESS_AUD`.
2. Remove the Access application from gielinor's `terraform/cloudflare/main.tf`.
3. Mark ADR-0004 as superseded.

The tunnel and the WAF stay.

## What the spike checks

Zitadel's gaps are in ADR-0008. Before the switch, confirm with the real provider:

- JWT access tokens carry the project ID in `aud` (with the scope above).
- The device flow works for `lastro-cli`.
- Claude completes the flow with the pre-registered client, with the secret method it uses.
