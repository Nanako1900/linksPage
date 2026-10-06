import { describe, expect, it, vi } from "vitest";
import { samplePage } from "../test/fixtures";
import { BOOTSTRAP_PATH, BootstrapError, fetchPage, loadPage, parseEnvelope, readEmbeddedPage } from "./bootstrap";

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

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

describe("parseEnvelope", () => {
  it("unwraps data", () => {
    expect(parseEnvelope({ data: samplePage() })?.version).toBe(4);
  });
  it.each([
    ["string", "x"],
    ["null", null],
    ["empty data", { data: {} }],
    ["bare page", samplePage()],
    ["bad locales", { data: { ...samplePage(), site: { ...samplePage().site, locales: "en" } } }],
  ])("rejects %s", (_name, value) => {
    expect(parseEnvelope(value)).toBeNull();
  });
});

describe("readEmbeddedPage", () => {
  it("reads #lp-data", () => {
    expect(readEmbeddedPage(docWith(JSON.stringify({ data: samplePage() })))?.page.slug).toBe("default");
  });
  it("returns null when absent, empty or malformed", () => {
    expect(readEmbeddedPage(docWith(null))).toBeNull();
    expect(readEmbeddedPage(docWith(""))).toBeNull();
    expect(readEmbeddedPage(docWith("{not json"))).toBeNull();
  });
});

describe("fetchPage", () => {
  it("requests the bootstrap endpoint as JSON", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse({ data: samplePage() }));
    await expect(fetchPage(fetcher)).resolves.toMatchObject({ revision: "r-5f2c9a1e" });
    expect(fetcher).toHaveBeenCalledWith(BOOTSTRAP_PATH, expect.objectContaining({ credentials: "same-origin" }));
  });

  it("uses global fetch by default", async () => {
    const f = vi.fn().mockResolvedValue(jsonResponse({ data: samplePage() }));
    vi.stubGlobal("fetch", f);
    await fetchPage();
    expect(f).toHaveBeenCalledOnce();
  });

  it.each([
    ["non-200", () => jsonResponse({}, 503), { name: "BootstrapError", status: 503 }],
    ["non-JSON", () => new Response("<html>", { status: 200 }), { message: "bootstrap response is not JSON" }],
    ["bad shape", () => jsonResponse({ nope: true }), { message: "bootstrap response has an unexpected shape" }],
  ])("throws on %s", async (_name, make, want) => {
    await expect(fetchPage(vi.fn().mockResolvedValue(make()))).rejects.toMatchObject(want);
  });
});

describe("loadPage", () => {
  it("prefers embedded data without fetching", async () => {
    const fetcher = vi.fn();
    const data = await loadPage(docWith(JSON.stringify({ data: samplePage() })), fetcher);
    expect(data.version).toBe(4);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("falls back to the API when the embedded data is invalid or absent", async () => {
    const fetcher = vi.fn().mockImplementation(async () => jsonResponse({ data: samplePage() }));
    await expect(loadPage(docWith('{"data":{}}'), fetcher)).resolves.toMatchObject({ version: 4 });
    await expect(loadPage(docWith(null), fetcher)).resolves.toMatchObject({ version: 4 });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("propagates fetch failures as BootstrapError", async () => {
    const fetcher = vi.fn().mockResolvedValue(jsonResponse({}, 404));
    await expect(loadPage(docWith(null), fetcher)).rejects.toBeInstanceOf(BootstrapError);
  });
});
