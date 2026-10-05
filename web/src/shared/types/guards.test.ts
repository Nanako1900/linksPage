import { describe, expect, it } from "vitest";
import fixture from "../../test/fixtures/public-page.json";
import { isBlock, isCommunity, isLiveDTO, isLiveView, isPublicPage, isPublicSite } from "./guards";
import type { LiveDTO, PublicPage } from "./public";

/** Fresh deep copy of the canonical fixture. */
function sample(): PublicPage {
  return structuredClone(fixture) as unknown as PublicPage;
}

const DELETE = Symbol("delete");

/** Returns a deep copy of the fixture with the value at path replaced (or deleted). */
function edited(path: ReadonlyArray<string | number>, value: unknown): unknown {
  const root: unknown = structuredClone(fixture);
  let node = root as Record<string | number, unknown>;
  for (const key of path.slice(0, -1)) node = node[key] as Record<string | number, unknown>;
  const last = path[path.length - 1] as string | number;
  if (value === DELETE) delete node[last];
  else node[last] = value;
  return root;
}

const DISCORD = "01920000-0000-7000-8000-000000000001";

describe("canonical fixture", () => {
  it("passes the PublicPage guard", () => {
    expect(isPublicPage(fixture)).toBe(true);
  });

  it("covers every card state and block kind", () => {
    const p = sample();
    const states = new Set(Object.values(p.communities).map((c) => c.live.state));
    for (const s of ["live", "stale", "degraded", "static", "qr-only", "unavailable"]) {
      expect(states.has(s as never)).toBe(true);
    }
    expect(new Set(p.blocks.map((b) => b.kind)).size).toBe(5);
  });

  it("narrows blocks as a discriminated union", () => {
    const heading = sample().blocks.find((b) => b.kind === "heading");
    expect(heading?.kind === "heading" ? heading.count : undefined).toBe(8);
  });
});

describe("isPublicPage rejects", () => {
  const link = "01920000-0000-7000-8000-000000000201";
  const cases: Array<[string, ReadonlyArray<string | number>, unknown]> = [
    ["missing revision", ["revision"], DELETE],
    ["bad page", ["page"], { id: "1", slug: "default" }],
    ["null blocks", ["blocks"], null],
    ["unknown block kind", ["blocks", 0, "kind"], "video"],
    ["heading without text", ["blocks", 0, "text"], DELETE],
    ["community block without ref", ["blocks", 1, "communityId"], DELETE],
    ["social row ids", ["blocks", 12, "linkIds"], [1]],
    ["bad state", ["communities", DISCORD, "live", "state"], "ok"],
    ["bad user status", ["communities", DISCORD, "live", "users", 0, "status"], "away"],
    ["omitted nullable", ["communities", DISCORD, "qr"], DELETE],
    ["community key mismatch", ["communities", DISCORD, "id"], "other"],
    ["bad embed", ["communities", DISCORD, "embed"], { kind: "youtube", src: "x" }],
    ["bad appearance", ["site", "appearance"], "neon"],
    ["bad theme", ["site", "theme", "dark", "accent"], DELETE],
    ["bad copy", ["site", "copy"], { en: { openInBrowser: 1 } }],
    ["bad link icon", ["links", link, "icon"], { kind: "emoji" }],
    ["bad platform", ["platforms", "discord", "needsExternalBrowser"], "yes"],
    ["bad nextBoundary", ["nextBoundary"], 0],
    // URL shapes (defence in depth).
    ["javascript embed", ["communities", DISCORD, "embed", "src"], "javascript:alert(1)"],
    ["same-origin embed", ["communities", DISCORD, "embed", "src"], "/admin"],
    [
      "other widget host",
      ["communities", DISCORD, "embed", "src"],
      "https://evil.example/widget?id=1114391825336250432",
    ],
    ["protocol-relative icon", ["communities", DISCORD, "icon", "url"], "//evil.example/a.png"],
    ["backslash icon", ["communities", DISCORD, "icon", "url"], "/\\evil.example/a.png"],
    ["absolute avatar", ["communities", DISCORD, "live", "users", 0, "avatarUrl"], "https://evil.example/a.png"],
    ["joinUrl outside /go", ["communities", DISCORD, "live", "joinUrl"], "https://evil.example"],
    ["javascript joinUrl", ["communities", DISCORD, "live", "joinUrl"], "javascript:alert(1)"],
    ["javascript inviteUrl", ["communities", DISCORD, "inviteUrl"], "javascript:alert(1)"],
    ["relative sharePath", ["communities", DISCORD, "sharePath"], "c/discord"],
    ["javascript link url", ["links", link, "url"], "javascript:alert(1)"],
    ["data link href", ["links", link, "href"], "data:text/html,x"],
    ["protocol-relative link href", ["links", link, "href"], "//evil.example"],
    ["javascript baseUrl", ["site", "baseUrl"], "javascript:alert(1)"],
    ["absolute site avatar", ["site", "avatar", "url"], "https://evil.example/a.png"],
    ["absolute qr", ["communities", "01920000-0000-7000-8000-000000000005", "qr", "url"], "https://evil.example/q.png"],
  ];
  it.each(cases)("%s", (_name, path, value) => {
    expect(isPublicPage(edited(path, value))).toBe(false);
  });

  /** The fixture plus a valid copy of the Discord community under `key`, parsed from JSON. */
  function withCommunityKey(key: string): unknown {
    const copy = JSON.stringify({ ...sample().communities[DISCORD], id: key });
    return JSON.parse(JSON.stringify(fixture).replace('"communities":{', `"communities":{"${key}":${copy},`));
  }

  it("accepts an extra community under an ordinary key (control)", () => {
    expect(isPublicPage(withCommunityKey("01920000-0000-7000-8000-0000000000ff"))).toBe(true);
  });

  it.each(["__proto__", "constructor", "prototype"])("a %s map key", (key) => {
    const page = withCommunityKey(key) as { communities: object };
    expect(Object.hasOwn(page.communities, key)).toBe(true);
    expect(isPublicPage(page)).toBe(false);
  });

  it("accepts mailto links and absolute rel=me hrefs", () => {
    const page = edited(["links", link, "url"], "mailto:hi@example.com");
    expect(isPublicPage(page)).toBe(true);
  });

  it("non-objects", () => {
    expect(isPublicPage([])).toBe(false);
    expect(isPublicPage(null)).toBe(false);
  });
});

describe("other guards", () => {
  it("validates LiveDTO", () => {
    const p = sample();
    const live: LiveDTO = {
      revision: p.revision,
      generatedAt: p.generatedAt,
      communities: Object.fromEntries(Object.entries(p.communities).map(([id, c]) => [id, c.live])),
    };
    expect(isLiveDTO(live)).toBe(true);
    expect(isLiveDTO({ ...live, communities: { x: { state: "live" } } })).toBe(false);
    expect(isLiveDTO({ revision: 1 })).toBe(false);
    const polluted = JSON.parse(
      `{"revision":"r","generatedAt":"g","communities":{"__proto__":${JSON.stringify(live.communities[DISCORD])}}}`,
    );
    expect(isLiveDTO(polluted)).toBe(false);
  });

  it("checks individual pieces", () => {
    const p = sample();
    expect(isPublicSite(p.site)).toBe(true);
    expect(isPublicSite({ ...p.site, avatar: { url: "/x" } })).toBe(false);
    expect(isBlock({ id: "x", kind: "heading", text: {}, count: "3" })).toBe(false);
    expect(isCommunity(p.communities[DISCORD])).toBe(true);
    expect(isCommunity(p.communities[DISCORD], "different-key")).toBe(false);
    expect(isLiveView({ ...p.communities[DISCORD]?.live, online: Number.NaN })).toBe(false);
  });
});
