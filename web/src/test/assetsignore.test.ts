/// <reference types="node" />
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Vite copies public/ into dist/, so the Worker's Static Assets upload
// (assets.directory = web/dist) skips these entries.
// Vitest runs from web/ (the package root).
const file = join(process.cwd(), "public", ".assetsignore");

describe(".assetsignore", () => {
  const entries = readFileSync(file, "utf8")
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l !== "" && !l.startsWith("#"));

  it.each([".vite/", "admin/"])("keeps %s off the edge", (entry) => {
    expect(entries).toContain(entry);
  });

  it("never hides the public entry or its assets", () => {
    expect(entries.some((e) => e === "assets/" || e === "index.html" || e === "*")).toBe(false);
  });
});
