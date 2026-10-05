import { describe, expect, it } from "vitest";
import { matchRoute } from "./route";

describe("matchRoute", () => {
  it.each([
    ["/", { kind: "home" }],
    ["/privacy", { kind: "privacy" }],
    ["/c/my-server", { kind: "community", slug: "my-server" }],
    ["/c/Bad_Slug", { kind: "notFound" }],
    ["/c/a/b", { kind: "notFound" }],
    ["/c/", { kind: "notFound" }],
    ["/elsewhere", { kind: "notFound" }],
  ])("%s", (path, want) => {
    expect(matchRoute(path)).toEqual(want);
  });
});
