# Signing in with Zitadel

How to set up the provider for lastro ([ADR-0008](adr/0008-identity-with-zitadel.md)) and connect the clients: the web app, the `lastro` CLI and Claude through MCP ([ADR-0009](adr/0009-mcp-server-for-claude.md)). lastro only verifies tokens, so everything here is configuration: nothing in the code names Zitadel.

Outside dev, lastro does not start without `LASTRO_OIDC_ISSUER`: the provider is the only way in.

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

## 4. Clients

- **CLI:** `lastro connect https://lastro.ftm.dev.br` shows a link and a code. Confirm it in the browser, and the token stays in `~/.config/lastro/config.json` (0600), refreshed by itself. `lastro whoami` shows who the server sees.
- **Claude:** Settings → Connectors → Add custom connector, with URL `https://lastro.ftm.dev.br/mcp`. Under Advanced settings, paste `lastro-claude`'s client ID and secret. Claude finds the provider through `/.well-known/oauth-protected-resource/mcp`. Removing `lastro-claude` in Zitadel cuts it off.

## What the spike found (2026-10-27)

Against the Zitadel on saradomin (v4.19.2), what ADR-0008 asked to confirm:

- JWT access tokens carry the project ID in `aud`, next to the client IDs of every application in the project. lastro only asks that the project ID be there.
- Access tokens have `client_id` but no `azp`; lastro reads `azp`, then `client_id`.
- The device flow works for `lastro-cli` as a public client, and `lastro whoami` shows `write`.
- The web login works in production. When the browser already has a Zitadel session, the login goes straight back without a page.

In the console:

- The web applications use the **CODE** method (client secret in Basic auth, which is what `golang.org/x/oauth2` sends first). The **PKCE** card makes a public client, which refuses the secret.
- `lastro-cli` is created from the **PKCE** card (auth method None) and then gets the **Device Code** and **Refresh Token** grant types. The **DEVICECODE** card sets Basic auth, which a CLI without a secret cannot use.
- A dev application with an `http://localhost` redirect needs **Development mode** on.
