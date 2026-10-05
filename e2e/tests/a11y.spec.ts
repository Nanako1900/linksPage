// axe-core: zero serious or critical violations at 375 and 1440 px, in
// light and dark appearance (the seed uses appearance: auto). Findings
// listed in KNOWN_ISSUES are reported separately (see support/axe.ts).
import { blockingFindings, KNOWN_ISSUES } from "./support/axe";
import { expect, expectNotFoundPage, test, waitForApp, waitForSettled } from "./support/fixtures";

const WIDTHS = [375, 1440] as const;
const SCHEMES = ["light", "dark"] as const;
const PAGES = ["/", "/c/qq", "/?lang=en"] as const;

for (const width of WIDTHS) {
  for (const colorScheme of SCHEMES) {
    test.describe(`${width}px ${colorScheme}`, () => {
      test.use({ viewport: { width, height: 900 }, colorScheme });

      for (const path of PAGES) {
        test(`axe: ${path}`, async ({ page }) => {
          await page.goto(path);
          await waitForApp(page);
          await expect(page.locator("html")).toHaveAttribute("data-appearance", colorScheme);
          await waitForSettled(page);
          expect((await blockingFindings(page)).filter((f) => !f.known)).toEqual([]);
        });
      }

      test("axe: QR code dialog", async ({ page }) => {
        await page.goto("/");
        await waitForApp(page);
        await page.getByRole("button", { name: "二维码" }).first().click();
        await expect(page.getByRole("dialog")).toBeVisible();
        await waitForSettled(page);
        expect((await blockingFindings(page)).filter((f) => !f.known)).toEqual([]);
      });
    });
  }
}

test.describe("404 page", () => {
  test.use({ allowConsole: [/status of 404/] });

  for (const colorScheme of SCHEMES) {
    test(`axe: 404 ${colorScheme}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme });
      const res = await page.goto("/c/no-such-community");
      expect(res?.status()).toBe(404);
      await expectNotFoundPage(page);
      expect((await blockingFindings(page)).filter((f) => !f.known)).toEqual([]);
    });
  }
});

test("known a11y issues still reproduce (remove fixed entries from KNOWN_ISSUES)", async ({ page }) => {
  test.skip(KNOWN_ISSUES.length === 0, "no known a11y issues");
  await page.goto("/");
  await waitForApp(page);
  await waitForSettled(page);
  const known = (await blockingFindings(page)).filter((f) => f.known);
  expect(known.length, "a KNOWN_ISSUES entry no longer reproduces").toBeGreaterThan(0);
});
