// Shared Playwright fixtures. Every test gets a guarded page:
// - CSP violations (enforced and report-only) are collected through a
//   binding registered before any page script runs, on every navigation;
// - console errors and uncaught page errors are collected too;
// - the test fails at teardown if any were seen.
// /media/p/* (the Discord image proxy) is answered locally by default: the
// stack is offline, and the proxy itself is covered by Go tests. Request
// interception disables Chromium's HTTP cache, so tests that need real
// revalidation (304) set `mockImages: false` and tolerate the proxy's
// failed image loads instead.
import { test as base, expect, type Page } from "@playwright/test";
import { PNG_1X1 } from "./images";

export interface CspViolation {
  directive: string;
  blockedURI: string;
  disposition: string;
  sourceFile: string;
  sample: string;
  documentURI: string;
}

export interface ConsoleError {
  text: string;
  /** URL of the resource the message is about ("" when none). */
  url: string;
}

export interface PageGuard {
  csp: CspViolation[];
  consoleErrors: ConsoleError[];
  pageErrors: string[];
}

/** Image proxy URLs (unreachable upstream in the offline stack). */
const MEDIA_PROXY = /\/media\/p\//;

const CSP_BINDING = "__lpReportCsp";

/** Runs in every document before its own scripts (not subject to CSP). */
const CSP_LISTENER = `
document.addEventListener("securitypolicyviolation", (e) => {
  window.${CSP_BINDING}({
    directive: e.effectiveDirective,
    blockedURI: e.blockedURI,
    disposition: e.disposition,
    sourceFile: e.sourceFile,
    sample: e.sample,
    documentURI: e.documentURI,
  });
}, true);
`;

export async function installGuard(page: Page, mockImages = true): Promise<PageGuard> {
  const guard: PageGuard = { csp: [], consoleErrors: [], pageErrors: [] };
  await page.exposeBinding(CSP_BINDING, (_source, v: CspViolation) => {
    guard.csp.push(v);
  });
  await page.addInitScript(CSP_LISTENER);
  page.on("console", (msg) => {
    if (msg.type() === "error") guard.consoleErrors.push({ text: msg.text(), url: msg.location().url });
  });
  page.on("pageerror", (err) => guard.pageErrors.push(err.message));
  if (mockImages) {
    await page.route(MEDIA_PROXY, (route) => route.fulfill({ status: 200, contentType: "image/png", body: PNG_1X1 }));
  }
  return guard;
}

/**
 * Console errors not covered by `allowed` (e.g. the 404 document itself).
 * Without image mocking, failed /media/p/ loads are expected.
 */
export function unexpectedConsoleErrors(guard: PageGuard, allowed: readonly RegExp[], mockImages: boolean): string[] {
  return guard.consoleErrors
    .filter((e) => mockImages || !MEDIA_PROXY.test(e.url))
    .map((e) => e.text)
    .filter((text) => !allowed.some((re) => re.test(text)));
}

interface Fixtures {
  guard: PageGuard;
  /** Console error patterns a test expects (checked at teardown). */
  allowConsole: RegExp[];
  /** Answer /media/p/ locally (disables the HTTP cache). */
  mockImages: boolean;
}

export const test = base.extend<Fixtures>({
  allowConsole: [[], { option: true }],
  mockImages: [true, { option: true }],
  guard: [
    async ({ page, allowConsole, mockImages }, use) => {
      const guard = await installGuard(page, mockImages);
      await use(guard);
      expect.soft(guard.csp, "CSP violations").toEqual([]);
      expect.soft(guard.pageErrors, "uncaught page errors").toEqual([]);
      expect.soft(unexpectedConsoleErrors(guard, allowConsole, mockImages), "console errors").toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };

/** Wait until React has replaced the server fallback markup (.lp-fb). */
export async function waitForApp(page: Page): Promise<void> {
  await expect(page.locator("#root .lp-fb")).toHaveCount(0);
  await expect(page.locator("#root main")).toBeVisible();
}

/**
 * Wait until finite animations and transitions (card entrance,
 * @starting-style) have finished; infinite ones (the online dot) are ignored.
 */
export async function waitForSettled(page: Page): Promise<void> {
  await page.waitForFunction(() =>
    document
      .getAnimations()
      .every((a) => a.effect?.getTiming().iterations === Number.POSITIVE_INFINITY || a.playState !== "running"),
  );
}

/** The server-rendered 404 page (no SPA): code, heading and seeded text. */
export async function expectNotFoundPage(page: Page): Promise<void> {
  await expect(page.locator("#root .lp-code")).toHaveText("404");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
}

/** The community card whose name heading matches `name`. */
export function card(page: Page, name: string | RegExp) {
  return page.locator("article.lp-card").filter({ has: page.getByRole("heading", { name }) });
}
