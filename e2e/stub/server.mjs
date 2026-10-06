// Stub Discord/KOOK upstream for E2E: replays internal/provider/*/testdata
// so the app under test never talks to the real services.
//
//   STUB_PORT      listen port (default 8090)
//   FIXTURE_ROOT   internal/provider (reads discord/testdata, kook/testdata)
//
// GET /__stub/requests returns the requests served so far (JSON), so
// tests can prove the app really used the stub.
import { createServer } from "node:http";
import { fileURLToPath } from "node:url";
import { loadFixture } from "./fixtures.mjs";
import { resolveRoute } from "./routes.mjs";

const MAX_LOG = 500;

/**
 * Build the request handler. Requests may arrive in absolute form (when the
 * app is configured with an HTTP proxy) or origin form.
 * @param {{ fixtureRoot: string }} opts
 */
export function createHandler({ fixtureRoot }) {
  const log = [];
  const handler = (req, res) => {
    const url = new URL(req.url ?? "/", "http://stub.invalid");
    if (url.pathname === "/__stub/requests") {
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify(log));
      return;
    }
    if (url.pathname === "/__stub/health") {
      res.writeHead(204);
      res.end();
      return;
    }
    const ref = resolveRoute(req.method ?? "GET", url);
    if (log.length < MAX_LOG)
      log.push({ method: req.method, path: url.pathname, query: url.search, fixture: ref?.name ?? null });
    if (!ref) {
      res.writeHead(404, { "content-type": "application/json" });
      res.end('{"message": "404: Not Found", "code": 0}');
      return;
    }
    let fx;
    try {
      fx = loadFixture(fixtureRoot, ref);
    } catch (err) {
      res.writeHead(500, { "content-type": "text/plain" });
      res.end(`fixture error: ${err instanceof Error ? err.message : String(err)}`);
      return;
    }
    res.writeHead(fx.status, { ...fx.headers, "content-length": String(fx.body.length) });
    res.end(fx.body);
  };
  return { handler, log };
}

function main() {
  const port = Number(process.env.STUB_PORT ?? "8090");
  const fixtureRoot = process.env.FIXTURE_ROOT ?? fileURLToPath(new URL("../../internal/provider", import.meta.url));
  const { handler } = createHandler({ fixtureRoot });
  const server = createServer(handler);
  server.listen(port, "0.0.0.0", () => {
    process.stdout.write(`stub upstream listening on :${port} (fixtures: ${fixtureRoot})\n`);
  });
  const stop = () => server.close(() => process.exit(0));
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
