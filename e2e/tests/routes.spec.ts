// Page routes: /c/{slug} pins the shared community, unknown paths get the
// 404 page (doc 4.2, contract section 3).
import { card, expect, expectNotFoundPage, test, waitForApp } from "./support/fixtures";

test("/c/{slug} pins the community above the rest of the page", async ({ page }) => {
  const res = await page.goto("/c/kook");
  expect(res?.status()).toBe(200);
  await waitForApp(page);

  const pinned = page.getByRole("region", { name: "分享给你的社区" });
  await expect(pinned).toBeVisible();
  await expect(pinned.locator("article.lp-card")).toHaveCount(1);
  await expect(pinned.getByRole("heading", { name: /^KOOK 语音/ })).toBeVisible();

  // The pinned card is first in document order and not repeated below.
  const names = await page.locator("article.lp-card h3").allInnerTexts();
  expect(names[0]).toMatch(/^KOOK 语音/);
  expect(names.filter((n) => n.startsWith("KOOK 语音"))).toHaveLength(1);
  expect(names).toHaveLength(6);
  await expect(page).toHaveTitle(/KOOK 语音/);
});

test("home lists every seeded community with its card state", async ({ page }) => {
  await page.goto("/");
  await waitForApp(page);
  const states = {
    "Discord 主服务器": "live",
    "KOOK 语音": "live",
    "未开启小部件的 Discord": "static",
    已解散的服务器: "unavailable",
    "QQ 群": "static",
    微信群: "qr-only",
  };
  for (const [name, state] of Object.entries(states)) {
    await expect(card(page, new RegExp(`^${name}`)), name).toHaveAttribute("data-state", state);
  }
  // Live numbers come from the stub fixtures (widget_ok / invite_ok / badge style 2).
  await expect(card(page, /^Discord 主服务器/)).toContainText("16 在线");
  await expect(card(page, /^Discord 主服务器/)).toContainText("125 成员");
  await expect(card(page, /^KOOK 语音/)).toContainText("在线");
  await expect(card(page, /^已解散的服务器/)).toContainText("这个服务器已经关闭。");
  await expect(card(page, /^已解散的服务器/).getByRole("link")).toHaveCount(0);
  await expect(card(page, /^微信群/).getByRole("img", { name: /二维码/ })).toBeVisible();
});

test("?lang=en renders English UI and content", async ({ page }) => {
  await page.goto("/?lang=en");
  await waitForApp(page);
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(card(page, /^Main Discord/).getByRole("link", { name: "Join" })).toBeVisible();
});

test.describe("404", () => {
  test.use({ allowConsole: [/status of 404/] });

  for (const path of ["/no-such-page", "/c/no-such-community", "/c/Not_A_Slug"]) {
    test(`unknown path ${path}`, async ({ page }) => {
      const res = await page.goto(path);
      expect(res?.status()).toBe(404);
      await expectNotFoundPage(page);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("页面不存在");
      await expect(page.getByText("E2E：这个地址不存在。")).toBeVisible();
      await expect(page.getByRole("link", { name: /返回首页/ })).toHaveAttribute("href", "/");
    });
  }
});
