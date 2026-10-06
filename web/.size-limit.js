// Budgets for the public entry (doc 8.1: JS ≤ 90 kB gz, CSS ≤ 15 kB gz).
// Files are resolved from the Vite manifest so shared chunks that the
// public entry imports statically are counted too.
import { readFileSync } from "node:fs";

const manifest = JSON.parse(readFileSync(new URL("./dist/.vite/manifest.json", import.meta.url), "utf8"));

function closure(key, seen = new Set()) {
  if (seen.has(key)) return seen;
  seen.add(key);
  for (const imp of manifest[key]?.imports ?? []) closure(imp, seen);
  return seen;
}

// Lazy chunks of the public entry (brand icons, QR encoder) and what
// they import, minus what the initial load already has.
function lazyClosure(initial) {
  const lazy = new Set();
  for (const key of initial) {
    for (const dyn of manifest[key]?.dynamicImports ?? []) closure(dyn, lazy);
  }
  return [...lazy].filter((k) => !initial.has(k));
}

const initial = closure("index.html");
const chunks = [...initial].map((k) => manifest[k]);
const js = chunks.map((c) => `dist/${c.file}`);
const css = chunks.flatMap((c) => c.css ?? []).map((f) => `dist/${f}`);
const lazyJs = lazyClosure(initial).map((k) => `dist/${manifest[k].file}`);

export default [
  { name: "public JS (gzip)", path: js, limit: "90 kB", gzip: true },
  { name: "public CSS (gzip)", path: css, limit: "15 kB", gzip: true },
  // Not in doc 8.1: keeps the on-demand chunks from growing unnoticed.
  { name: "public JS incl. lazy chunks (gzip)", path: [...js, ...lazyJs], limit: "110 kB", gzip: true },
];
