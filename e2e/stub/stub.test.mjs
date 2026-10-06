import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { loadFixture, parseHeaders } from "./fixtures.mjs";
import { resolveRoute } from "./routes.mjs";
import { createHandler } from "./server.mjs";

const FIXTURES = fileURLToPath(new URL("../../internal/provider", import.meta.url));

test("parseHeaders keeps the status and replayed headers only", () => {
  const cases = [
    {
      name: "discord 200",
      text: "HTTP/2 200\nX-Fixture-Note: x\ncontent-type: application/json\ncache-control: public, max-age=300\nserver: cloudflare\n",
      status: 200,
      headers: { "content-type": "application/json", "cache-control": "public, max-age=300" },
    },
    {
      name: "kook 302 with CRLF",
      text: "HTTP/1.1 302 Found\r\nLocation: https://img.shields.io/static/v1?label=a:b\r\n\r\n",
      status: 302,
      headers: { location: "https://img.shields.io/static/v1?label=a:b" },
    },
  ];
  for (const c of cases) {
    const got = parseHeaders(c.text);
    assert.equal(got.status, c.status, c.name);
    assert.deepEqual(got.headers, c.headers, c.name);
  }
  assert.throws(() => parseHeaders("garbage"), /bad status line/);
});

test("resolveRoute maps every seeded ID to its fixture", () => {
  const u = (p) => new URL(p, "http://stub");
  const cases = [
    ["/api/guilds/1114391825336250432/widget.json", "widget_ok"],
    ["/api/guilds/662267976984297473/widget.json", "widget_disabled"],
    ["/api/guilds/100000000000000000/widget.json", "widget_unknown_guild"],
    ["/api/guilds/123/widget.json", "widget_unknown_guild"],
    ["/api/v10/invites/KwdRuAkT?with_counts=true", "invite_ok"],
    ["/api/v10/invites/nope?with_counts=true", "invite_unknown"],
    ["/api/v3/badge/guild?guild_id=5417470909511807&style=0", "badge_public_style0"],
    ["/api/v3/badge/guild?guild_id=5417470909511807&style=2", "badge_public_style2"],
    ["/api/v3/badge/guild?guild_id=5417470909511807&style=9", "badge_public_style0"],
    ["/api/v3/badge/guild?guild_id=5417470909511807", "badge_public_style0"],
    ["/api/v3/badge/guild?guild_id=1&style=0", "badge_not_public_style0"],
    ["/api/v3/badge/guild?guild_id=1&style=2", "badge_not_public_style2"],
  ];
  for (const [path, want] of cases) assert.equal(resolveRoute("GET", u(path))?.name, want, path);
  assert.equal(resolveRoute("GET", u("/elsewhere")), null);
  assert.equal(resolveRoute("POST", u("/api/guilds/1/widget.json")), null);
});

test("loadFixture reads real fixtures and defaults KOOK bodies to null", () => {
  const widget = loadFixture(FIXTURES, { provider: "discord", name: "widget_ok" });
  assert.equal(widget.status, 200);
  assert.equal(JSON.parse(widget.body.toString()).id, "1114391825336250432");
  const badge = loadFixture(FIXTURES, { provider: "kook", name: "badge_public_style0" });
  assert.equal(badge.status, 302);
  assert.match(badge.headers.location, /^https:\/\/img\.shields\.io\/static\/v1\?/);
  assert.equal(badge.body.toString(), "null");
  assert.throws(() => loadFixture(FIXTURES, { provider: "discord", name: "missing" }), /ENOENT/);
});

test("loadFixture rethrows read errors other than a missing body", () => {
  const dir = mkdtempSync(join(tmpdir(), "stub-"));
  mkdirSync(join(dir, "discord", "testdata"), { recursive: true });
  writeFileSync(join(dir, "discord", "testdata", "x.headers"), "HTTP/2 200\n");
  mkdirSync(join(dir, "discord", "testdata", "x.json")); // EISDIR when read as a file
  assert.throws(() => loadFixture(dir, { provider: "discord", name: "x" }), /EISDIR/);
  writeFileSync(join(dir, "discord", "testdata", "empty.headers"), "HTTP/2 204\n");
  assert.equal(loadFixture(dir, { provider: "discord", name: "empty" }).body.length, 0);
});

async function withServer(fixtureRoot, fn) {
  const { handler, log } = createHandler({ fixtureRoot });
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  try {
    await fn(`http://127.0.0.1:${port}`, log);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

test("handler replays fixtures, logs requests and reports errors", async () => {
  await withServer(FIXTURES, async (base, log) => {
    const health = await fetch(`${base}/__stub/health`);
    assert.equal(health.status, 204);

    const widget = await fetch(`${base}/api/guilds/662267976984297473/widget.json`);
    assert.equal(widget.status, 403);
    assert.equal(widget.headers.get("cache-control"), "public, max-age=300, s-maxage=300");
    assert.equal((await widget.json()).code, 50004);

    const badge = await fetch(`${base}/api/v3/badge/guild?guild_id=5417470909511807&style=2`, { redirect: "manual" });
    assert.equal(badge.status, 302);
    assert.match(badge.headers.get("location") ?? "", /label=10350%2F107345\+ONLINE/);

    const missing = await fetch(`${base}/nope`);
    assert.equal(missing.status, 404);

    const seen = await (await fetch(`${base}/__stub/requests`)).json();
    assert.deepEqual(
      seen.map((r) => r.fixture),
      ["widget_disabled", "badge_public_style2", null],
    );
    assert.equal(log.length, 3);
  });

  const broken = mkdtempSync(join(tmpdir(), "stub-"));
  await withServer(broken, async (base) => {
    const res = await fetch(`${base}/api/v10/invites/KwdRuAkT`);
    assert.equal(res.status, 500);
    assert.match(await res.text(), /fixture error: .*ENOENT/);
  });
});
