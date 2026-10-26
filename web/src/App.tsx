import { useEffect, useState } from "react";

type Me = { email: string };

// M0: only confirms the frontend can reach the authenticated API.
// Tailwind, shadcn/ui and the Cédula design system arrive in M1, with the Transactions screen.
export function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch("/api/v1/me")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setMe)
      .catch((e: Error) => setError(e.message));
  }, []);

  return (
    <main style={{ fontFamily: "system-ui, sans-serif", padding: "2rem" }}>
      <h1>lastro</h1>
      {me && <p>Signed in as {me.email}</p>}
      {error && <p>Could not reach the API: {error}</p>}
      {!me && !error && <p>Loading…</p>}
    </main>
  );
}
