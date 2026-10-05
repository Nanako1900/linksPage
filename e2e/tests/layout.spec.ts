// Doc 8.6 / 13.3: at 320 px with a 130 % default font size nothing may
// scroll sideways.
import type { Page } from "@playwright/test";
import { UA } from "./support/env";
import { expect, test, waitForApp } from "./support/fixtures";

/** 130 % of Chromium's 16 px default, set like a user font preference. */
const FONT_PX = 21;

async function setDefaultFontSize(page: Page): Promise<void> {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Page.enable");
  await cdp.send("Page.setFontSizes", { fontSizes: { standard: FONT_PX, fixed: FONT_PX } });
}

async function horizontalOverflow(page: Page) {
  return page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    const wide = [...document.querySelectorAll<HTMLElement>("body *")]
      .filter((el) => el.getBoundingClientRect().right > vw + 0.5 && el.getClientRects().length > 0)
      .filter((el) => !el.closest("dialog:not([open])"))
      .slice(0, 5)
      .map((el) => `${el.tagName.toLowerCase()}.${el.className}`);
    return { scrollWidth: document.documentElement.scrollWidth, clientWidth: vw, wide };
  });
}

for (const [label, userAgent] of [
  ["phone", UA.androidChrome],
  ["WeChat", UA.wechatAndroid],
] as const) {
  test.describe(`320px, 130% font, ${label}`, () => {
    test.use({ viewport: { width: 320, height: 640 }, userAgent, isMobile: true, hasTouch: true });

    for (const path of ["/", "/c/discord", "/?lang=en"]) {
      test(`no horizontal overflow: ${path}`, async ({ page }) => {
        await setDefaultFontSize(page);
        await page.goto(path);
        await waitForApp(page);
        const rootPx = await page.evaluate(() =>
          Number.parseFloat(getComputedStyle(document.documentElement).fontSize),
        );
        expect(rootPx).toBeGreaterThanOrEqual(FONT_PX);
        const o = await horizontalOverflow(page);
        expect(o.wide, "elements past the right edge").toEqual([]);
        expect(o.scrollWidth).toBeLessThanOrEqual(o.clientWidth);
      });
    }
  });
}
