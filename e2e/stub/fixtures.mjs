// Loads the recorded upstream responses in internal/provider/*/testdata.
// A fixture is "<name>.headers" (curl -D format: status line + headers)
// plus an optional "<name>.json" body.
import { readFileSync } from "node:fs";
import { join } from "node:path";

/** Response headers worth replaying; the rest (date, cf-*, notes) is noise. */
const REPLAYED = new Set(["content-type", "cache-control", "location", "retry-after", "x-ratelimit-global"]);

/**
 * Parse a curl -D header dump.
 * @param {string} text
 * @returns {{ status: number, headers: Record<string, string> }}
 */
export function parseHeaders(text) {
  const lines = text.split(/\r?\n/);
  const match = /^HTTP\/[\d.]+ (\d{3})\b/.exec(lines[0] ?? "");
  if (!match) throw new Error(`fixture: bad status line ${JSON.stringify(lines[0])}`);
  const headers = {};
  for (const line of lines.slice(1)) {
    const i = line.indexOf(":");
    if (i <= 0) continue;
    const name = line.slice(0, i).trim().toLowerCase();
    if (REPLAYED.has(name)) headers[name] = line.slice(i + 1).trim();
  }
  return { status: Number(match[1]), headers };
}

function readOptional(path) {
  try {
    return readFileSync(path);
  } catch (err) {
    if (err && err.code === "ENOENT") return null;
    throw err;
  }
}

/**
 * Load one fixture. KOOK badge bodies are the literal "null" (not stored).
 * @param {string} root internal/provider (holds discord/testdata and kook/testdata)
 * @param {{ provider: string, name: string }} ref
 */
export function loadFixture(root, ref) {
  const base = join(root, ref.provider, "testdata", ref.name);
  const { status, headers } = parseHeaders(readFileSync(`${base}.headers`, "utf8"));
  const body = readOptional(`${base}.json`) ?? Buffer.from(ref.provider === "kook" ? "null" : "");
  return { status, headers, body };
}
