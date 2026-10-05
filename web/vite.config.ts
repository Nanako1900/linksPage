/// <reference types="vitest/config" />
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// Inline boot script rendered by the Go server (resolves data-appearance).
const bootScriptDir = resolve(import.meta.dirname, "../internal/webui/static");

/**
 * Dev server only: mimic what the Go renderer does around the entries so
 * dark mode can be developed locally, and serve the admin entry for
 * /admin and /admin/* (Vite's SPA fallback would serve the public shell).
 */
function devShell(): Plugin {
  return {
    name: "linkspage-dev-shell",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        const path = req.url?.split("?")[0] ?? "";
        const isAdminRoute = path === "/admin" || (path.startsWith("/admin/") && !path.includes("."));
        if (isAdminRoute) req.url = "/admin/index.html";
        next();
      });
    },
    transformIndexHtml: {
      order: "pre",
      // Only shells that declare data-appearance (the public one), like Go.
      handler: (html) =>
        html.includes("data-appearance=")
          ? [
              {
                tag: "script",
                children: readFileSync(resolve(bootScriptDir, "boot.js"), "utf8"),
                injectTo: "head-prepend",
              },
            ]
          : [],
    },
  };
}

// The Go server proxies nothing: in development Vite forwards these
// prefixes to the Go server on :8080.
const backend = "http://127.0.0.1:8080";
const proxied = ["/api", "/go", "/media", "/healthz", "/readyz"];

export default defineConfig({
  base: "/",
  plugins: [react(), tailwindcss(), devShell()],
  build: {
    // Go renders the HTML itself from this manifest (keys "index.html"
    // and "admin/index.html"); it never serves dist/index.html.
    manifest: true,
    assetsDir: "assets",
    target: ["chrome99", "safari15.4"],
    rollupOptions: {
      input: {
        public: resolve(import.meta.dirname, "index.html"),
        admin: resolve(import.meta.dirname, "admin/index.html"),
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: Object.fromEntries(proxied.map((p) => [p, { target: backend, changeOrigin: false }])),
    // boot.js is read by the dev shell and by the boot.js parity test.
    fs: { allow: [".", bootScriptDir] },
  },
  test: {
    environment: "jsdom",
    restoreMocks: true,
    unstubGlobals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      include: ["src/shared/**", "src/public/**"],
      exclude: ["src/shared/api/gen/**", "src/**/*.test.{ts,tsx}", "src/public/main.tsx"],
      thresholds: { lines: 80, functions: 80, branches: 80, statements: 80 },
    },
  },
});
