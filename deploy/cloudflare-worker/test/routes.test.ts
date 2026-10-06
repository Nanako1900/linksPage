import { describe, expect, it } from "vitest";
import { classify, NOT_FOUND_RENDER_PATH } from "../src/routes";

describe("classify", () => {
  const html = (renderPath: string, knownNotFound = false) => ({ kind: "html", renderPath, knownNotFound });
  const cases: ReadonlyArray<readonly [string, unknown]> = [
    ["/", html("/")],
    ["/privacy", html("/privacy")],
    ["/c/discord", html("/c/discord")],
    ["/c/a", html("/c/a")],
    [`/c/${"a".repeat(64)}`, html(`/c/${"a".repeat(64)}`)],
    [`/c/${"a".repeat(65)}`, html(NOT_FOUND_RENDER_PATH, true)],
    ["/c/-bad", html(NOT_FOUND_RENDER_PATH, true)],
    ["/c/Upper", html(NOT_FOUND_RENDER_PATH, true)],
    ["/c/", html(NOT_FOUND_RENDER_PATH, true)],
    ["/c/a/b", html(NOT_FOUND_RENDER_PATH, true)],
    ["/privacy/", html(NOT_FOUND_RENDER_PATH, true)],
    ["/nope", html(NOT_FOUND_RENDER_PATH, true)],
    ["/api/v1/public/bootstrap", { kind: "origin" }],
    ["/go/discord", { kind: "origin" }],
    ["/media/u/abc.webp", { kind: "origin" }],
    ["/healthz", { kind: "origin" }],
    ["/readyz", { kind: "origin" }],
    ["/favicon.ico", { kind: "origin" }],
    ["/robots.txt", { kind: "origin" }],
    ["/site.webmanifest", { kind: "origin" }],
    ["/admin", { kind: "origin" }],
    ["/admin/login", { kind: "origin" }],
    ["/assets/missing.js", { kind: "asset-miss" }],
    ["/fonts/x.woff2", { kind: "asset-miss" }],
    ["/ext/x.js", { kind: "asset-miss" }],
    ["/apix", html(NOT_FOUND_RENDER_PATH, true)],
  ];
  it.each(cases)("%s", (path, want) => {
    expect(classify(path)).toEqual(want);
  });
});
