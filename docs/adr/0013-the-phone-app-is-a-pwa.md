# ADR-0013: On the phone, lastro is an installed web app

**Status:** Accepted
**Date:** 2026-10-28
**Deciders:** Felipe

## Context

I read lastro mostly on the phone, in the browser. The screens were built for it (the tab bar, the `Sheet`, the safe areas), but a browser tab gets lost among the others, opens with the browser's bars, and has no icon. ADR-0008 left room for a native app later (`lastro-app`, Native with PKCE in Zitadel), and `PLAN.md` parked it.

A native app would mean drawing and building every screen again (React Native or Swift and Kotlin), redoing Cédula's components for it, and shipping each change through a store: a paid Apple developer account for the iPhone, builds that expire, and an API that has to keep answering older versions of the app. That is a lot for one user who mostly reads.

## Decision

- **lastro installs from the browser to the home screen, as a PWA.** `manifest.json` opens Dashboard in `standalone`, with the icon from the canvas (an l in a guilloché ring) in `web/public/icons`, rendered from `web/icons/icon.svg`. The status bar takes the paper of the theme in use. Android's navigation bar, under the tab bar, cannot follow the page: Chrome paints it from the manifest's colours, fixed when the app is installed (`theme_color` up to Android 14, the `background_color` behind a transparent bar from Android 15). They are the dark paper, so the bar matches the dark theme, and under the light theme it is a dark strip below the tab bar; the splash screen is dark too. `color_scheme_dark` (in the manifest spec since 2026) would let the bar follow the phone's dark mode, but on my Android it stayed light, so it waits until Chrome reads it.
- **Nothing leaves `platform/auth`.** The manifest and the icons sit behind the login like everything else. The page asks for the manifest with `crossorigin="use-credentials"`, because browsers fetch it without cookies otherwise; the icons go with the session.
- **The login does not change.** The installed app uses the same BFF and the same 14-day session cookie. On the iPhone the installed app keeps its own cookies, apart from Safari's, so it signs in once on its own.
- **No service worker.** Neither Chrome nor Safari needs one to install. Without one there is no second copy of the app to keep fresh, and no financial data stored on the phone.
- **A deploy is the update.** The binary carries the frontend and the API together, so they never disagree. `index.html` goes out with `no-cache`, and the files under `/assets/`, named by a hash of their content, with `private, max-age=31536000, immutable` (private, so Cloudflare keeps no copy of what is behind the login). An installed app can stay in memory for days, so when it comes back to the foreground it compares the files `index.html` names with the ones it runs: after 10 minutes or more away it reloads; sooner, the next navigation loads the new version in full. Nothing shows on screen.

## Options considered

- **Native app (React Native, or Swift and Kotlin):** widgets, Face ID, native push. Rejected for now: every screen twice, a store between me and each change, and versions of the API to keep alive.
- **Capacitor (the same React in a native shell):** keeps the screens, but the session cookie does not reach a WebView with another origin, so it needs the `lastro-app` client and bearer tokens, plus a store or live updates. Most of the cost of native for little of the gain.
- **PWA with a service worker:** offline and web push. Rejected for now: offline is not worth a cached copy of financial data, and the service worker holds the old version until every window closes.
- **PWA without a service worker:** chosen.

## Consequences

- A native app comes back only for something a PWA cannot do: a home screen widget with the month, or a lock with Face ID. The API already takes its tokens (ADR-0008).
- Web push becomes possible: the reason ADR-0007 gave against it, Cloudflare Access in the way, is gone. On the iPhone it needs the app installed (iOS 16.4 or later) and a service worker. The payload is end-to-end encrypted, so Apple and Google see less than Telegram does. It would be a new ADR, beside Telegram or instead of it.
- If Android's installer ever fetches the icons without the session, the fallback is to leave `/manifest.json` and `/icons/` outside the login (they hold nothing but a name and a picture), with `AGENTS.md`'s list of open routes updated.
