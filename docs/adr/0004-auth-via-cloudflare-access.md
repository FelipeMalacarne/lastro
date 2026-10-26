# ADR-0004: Authentication delegated to Cloudflare Access

**Status:** Accepted
**Date:** 2026-10-26
**Deciders:** Felipe

## Context

The app holds financial data and must be reachable from outside the house. Building its own login (passwords, sessions, 2FA) is work and attack surface for a single user.

## Decision

External access goes through Cloudflare Tunnel, with a Cloudflare Access application in front. The app **also** verifies the Access JWT on every request (`platform/auth`): RS256 signature against the team's public keys, issuer, audience (AUD tag) and validity window. A direct request to the service that bypasses the tunnel is therefore rejected.

- Only `/healthz` and `/readyz` are outside authentication, because the kubelet calls them directly.
- `LASTRO_AUTH_DISABLED=true` turns verification off, but the app refuses to start that way outside `LASTRO_ENV=dev`.

## Options considered

- **Own login:** more code, more risk, no benefit. Rejected.
- **LAN or VPN only:** safe, but no easy access from the phone on the go.
- **Cloudflare Access plus verification in the app:** login, 2FA and access policy live in Access; the app only checks the token. Chosen.

## Consequences

- If Cloudflare Access is down, the app is unreachable from outside. At home it can be reached through the cluster in dev mode.
- JWT verification uses only the standard library, to avoid dependencies; it is tested against tampered payloads, wrong audience, wrong issuer, expiry and `alg: none`.
