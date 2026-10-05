import { describe, expect, it, vi } from "vitest";
import { sampleBootstrap } from "../test/fixtures";
import { BootstrapError, isBootstrap, loadBootstrap, parseEnvelope, readEmbeddedBootstrap } from "./bootstrap";

function docWith(json: string | null): Document {
  const doc = document.implementation.createHTMLDocument("t");
  if (json !== null) {
    const el = doc.createElement("script");
    el.id = "lp-data";
    el.type = "application/json";
    el.textContent = json;
    doc.body.append(el);
  }
  return doc;
}

const okResponse = (data: unknown) => ({ status: 200 as const, data: data as never, headers: new Headers() });

describe("isBootstrap", () => {
  it("accepts the server shape", () => {
    expect(isBootstrap(sampleBootstrap())).toBe(true);
  });

  it.each([
    ["null", null],
    ["array", []],
    ["non-numeric version", { ...sampleBootstrap(), version: "1" }],
    ["missing version", { ...sampleBootstrap(), version: undefined }],
    ["locales as a string", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: "en" } }],
    ["locales with numbers", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: [1, 2] } }],
    ["locales with empty tags", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: [""] } }],
    ["locales as an object", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: { 0: "en" } } }],
    ["empty default locale", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, defaultLocale: "" } }],
    [
      "palette missing a color",
      {
        ...sampleBootstrap(),
        site: {
          ...sampleBootstrap().site,
          theme: { ...sampleBootstrap().site.theme, dark: { ...sampleBootstrap().site.theme.dark, bg: 1 } },
        },
      },
    ],
    [
      "theme radius not a string",
      {
        ...sampleBootstrap(),
        site: { ...sampleBootstrap().site, theme: { ...sampleBootstrap().site.theme, radius: 3 } },
      },
    ],
    ["bad page", { ...sampleBootstrap(), page: { id: "1", slug: "x" } }],
    ["bad appearance", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, appearance: "neon" } }],
    ["non-string title", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, title: { en: 1 } } }],
    ["missing theme", { ...sampleBootstrap(), site: { ...sampleBootstrap().site, theme: null } }],
    ["missing site", { ...sampleBootstrap(), site: undefined }],
  ])("rejects %s", (_name, value) => {
    expect(isBootstrap(value)).toBe(false);
  });
});

describe("parseEnvelope", () => {
  it("unwraps data", () => {
    expect(parseEnvelope({ data: sampleBootstrap() })?.version).toBe(3);
  });
  it("rejects non-objects and bad data", () => {
    expect(parseEnvelope("x")).toBeNull();
    expect(parseEnvelope({ data: {} })).toBeNull();
  });
});

describe("readEmbeddedBootstrap", () => {
  it("reads #lp-data", () => {
    expect(readEmbeddedBootstrap(docWith(JSON.stringify({ data: sampleBootstrap() })))?.page.slug).toBe("default");
  });
  it("returns null when absent, empty or malformed", () => {
    expect(readEmbeddedBootstrap(docWith(null))).toBeNull();
    expect(readEmbeddedBootstrap(docWith(""))).toBeNull();
    expect(readEmbeddedBootstrap(docWith("{not json"))).toBeNull();
  });
});

describe("loadBootstrap", () => {
  it("prefers embedded data without fetching", async () => {
    const fetcher = vi.fn();
    const data = await loadBootstrap(docWith(JSON.stringify({ data: sampleBootstrap() })), fetcher);
    expect(data.version).toBe(3);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("falls back to the API when the embedded data is invalid", async () => {
    const invalid = { data: { ...sampleBootstrap(), site: { ...sampleBootstrap().site, locales: "en" } } };
    const fetcher = vi.fn().mockResolvedValue(okResponse({ data: sampleBootstrap() }));
    await expect(loadBootstrap(docWith(JSON.stringify(invalid)), fetcher)).resolves.toMatchObject({ version: 3 });
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it("falls back to the API", async () => {
    const fetcher = vi.fn().mockResolvedValue(okResponse({ data: sampleBootstrap() }));
    await expect(loadBootstrap(docWith(null), fetcher)).resolves.toMatchObject({ version: 3 });
    expect(fetcher).toHaveBeenCalledOnce();
  });

  it("throws on non-200", async () => {
    const fetcher = vi.fn().mockResolvedValue({ status: 503, data: {}, headers: new Headers() });
    await expect(loadBootstrap(docWith(null), fetcher)).rejects.toMatchObject({ name: "BootstrapError", status: 503 });
  });

  it("throws on an unexpected body", async () => {
    const fetcher = vi.fn().mockResolvedValue(okResponse({ nope: true }));
    await expect(loadBootstrap(docWith(null), fetcher)).rejects.toBeInstanceOf(BootstrapError);
  });
});
