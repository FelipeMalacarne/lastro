// A new version arrives with nothing on screen (canvas: "On the phone: lastro
// installed as an app"; ADR-0013). The server embeds the frontend, so a deploy
// changes the files index.html names under /assets/. An installed app can sit
// in memory for days, so each time it comes back to the foreground it fetches
// index.html and compares: after 10 minutes or more away it reloads at once;
// sooner, the next navigation loads the new version in full.

const longAway = 10 * 60 * 1000;

/** The build's files, as index.html names them. Empty on the dev server, which serves /src. */
function assets(doc: Document): string {
  return [...doc.querySelectorAll('script[src^="/assets/"], link[href^="/assets/"]')]
    .map((e) => e.getAttribute("src") ?? e.getAttribute("href"))
    .sort()
    .join(" ");
}

const running = assets(document);
let waiting = false;
let hiddenAt = 0;

async function newerOut(): Promise<boolean> {
  try {
    const r = await fetch("/", { cache: "no-store" });
    // Signed out, or the server is away: the next API call says so.
    if (!r.ok) return false;
    const latest = assets(new DOMParser().parseFromString(await r.text(), "text/html"));
    return latest !== "" && latest !== running;
  } catch {
    return false;
  }
}

if (running) {
  document.addEventListener("visibilitychange", async () => {
    if (document.visibilityState === "hidden") {
      hiddenAt = Date.now();
      return;
    }
    const away = hiddenAt > 0 && Date.now() - hiddenAt >= longAway;
    if (!waiting) waiting = await newerOut();
    if (waiting && away) location.reload();
  });
}

/** A newer version is out: the next navigation should load it in full. */
export function updateWaiting(): boolean {
  return waiting;
}
