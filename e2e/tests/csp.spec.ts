// Hash-based CSP (doc 12.4, M0 spike): no securitypolicyviolation on the
// first load nor on a reload that the server answers with 304. The guard
// fixture fails any test that sees a violation; these tests also assert it
// inline so a failure points at the right step.
import type { Page } from "@playwright/test";
import { UA } from "./support/env";
import { expect, expectNotFoundPage, type PageGuard, test, waitForApp } from "./support/fixtures";

interface DocumentTiming {
  transferSize: number;
  encodedBodySize: number;
}

/**
 * Navigation timing of the current document. Chromium reports a
 * revalidated (304) document as 200 to Playwright, but its transferSize
 * then covers only the response headers, not the cached body.
 */
function documentTiming(page: Page): Promise<DocumentTiming> {
  return page.evaluate(() => {
    const [nav] = performance.getEntriesByType("navigation") as PerformanceNavigationTiming[];
    return { transferSize: nav?.transferSize ?? -1, encodedBodySize: nav?.encodedBodySize ?? -1 };
  });
}

async function loadAndReload(page: Page, guard: PageGuard, path: string, ready: (p: Page) => Promise<void>) {
  const first = await page.goto(path);
  await ready(page);
  expect(guard.csp, "violations on first load").toEqual([]);
  const firstTiming = await documentTiming(page);
  await page.reload();
  await ready(page);
  expect(guard.csp, "violations after reload").toEqual([]);
  return { status: first?.status(), firstTiming, reloadTiming: await documentTiming(page) };
}

// Route interception would disable the HTTP cache and with it the 304.
test.use({ mockImages: false });

for (const path of ["/", "/c/discord", "/privacy", "/?lang=en"]) {
  test(`no CSP violation on load and 304 reload: ${path}`, async ({ page, guard }) => {
    const r = await loadAndReload(page, guard, path, waitForApp);
    expect(r.status).toBe(200);
    // First load transfers the body; the no-cache reload is a 304 (headers only).
    expect(r.firstTiming.transferSize).toBeGreaterThan(r.firstTiming.encodedBodySize);
    expect(r.reloadTiming.encodedBodySize).toBe(r.firstTiming.encodedBodySize);
    expect(r.reloadTiming.transferSize).toBeGreaterThan(0);
    expect(r.reloadTiming.transferSize).toBeLessThan(r.reloadTiming.encodedBodySize);
  });
}

test.describe("404 page", () => {
  // Chromium logs the 404 document itself as a failed resource.
  test.use({ allowConsole: [/status of 404/] });

  test("no CSP violation on load and reload", async ({ page, guard }) => {
    const r = await loadAndReload(page, guard, "/no-such-page", expectNotFoundPage);
    expect(r.status).toBe(404);
  });
});

test("304 carries the same CSP and Vary as the 200", async ({ request }) => {
  const headers = { "Accept-Encoding": "gzip" };
  const first = await request.get("/", { headers });
  expect(first.status()).toBe(200);
  const etag = first.headers().etag ?? "";
  expect(etag).toMatch(/^W\/"/);
  const again = await request.get("/", { headers: { ...headers, "If-None-Match": etag } });
  expect(again.status()).toBe(304);
  expect(again.headers()["content-security-policy"]).toBe(first.headers()["content-security-policy"]);
  expect(again.headers().vary).toMatch(/Accept-Encoding/i);
});

test.describe("/go guide page inside WeChat", () => {
  test.use({ userAgent: UA.wechatIOS });

  test("no CSP violation on load and reload", async ({ page, guard }) => {
    const res = await page.goto("/go/discord");
    expect(res?.status()).toBe(200);
    await expect(page.locator("body.lp-page--open-in-browser")).toBeVisible();
    await page.reload();
    await expect(page.locator("body.lp-page--open-in-browser")).toBeVisible();
    expect(guard.csp).toEqual([]);
  });
});

test("control: the guard does catch a violation", async ({ page, guard }) => {
  await page.goto("/");
  await waitForApp(page);
  await page.evaluate(() => {
    const style = document.createElement("style");
    style.textContent = "body{outline:1px solid red}";
    document.head.append(style);
  });
  await expect.poll(() => guard.csp.map((v) => v.directive)).toContain("style-src-elem");
  // Chromium also logs the refused inline style; both are expected here.
  guard.csp.splice(0);
  guard.consoleErrors.splice(0);
  guard.pageErrors.splice(0);
});
