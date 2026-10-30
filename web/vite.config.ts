import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// In dev, Vite proxies /api and /auth to `lastrod serve`. Both ports come from
// the repo's .env (LASTRO_WEB_PORT and LASTRO_HTTP_ADDR), so each worktree runs
// its own pair. strictPort fails on a taken port instead of moving to the next
// one while still proxying to whichever API owns LASTRO_HTTP_ADDR.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, "..", "LASTRO_");
  const api = `http://localhost:${(env.LASTRO_HTTP_ADDR || ":8080").split(":").pop()}`;
  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      // "@/lib/utils" → src/lib/utils; "/src" is relative to the project root.
      alias: { "@": "/src" },
    },
    server: {
      port: Number(env.LASTRO_WEB_PORT || 5173),
      strictPort: true,
      proxy: { "/api": api, "/auth": api },
    },
    build: {
      outDir: "dist",
      emptyOutDir: true,
    },
  };
});
