// Copy actions (doc 8.5): QQ group number on desktop, "open on desktop"
// on phones. http://localhost is a secure context, so the Clipboard API
// is used and its result can be read back.
import { BASE_URL, UA } from "./support/env";
import { card, expect, test, waitForApp } from "./support/fixtures";

test.use({ permissions: ["clipboard-read", "clipboard-write"] });

test("QQ group: copy the group number", async ({ page }) => {
  await page.goto("/");
  await waitForApp(page);
  const qq = card(page, /^QQ 群/);
  await expect(qq.getByRole("link", { name: "加群" })).toHaveAttribute("href", "/go/qq");
  await qq.getByRole("button", { name: "复制群号" }).click();
  // The label switches to "已复制" (its accessible name changes too).
  await expect(qq.getByRole("button", { name: "已复制" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("123456789");
});

test.describe("phone browser", () => {
  test.use({ userAgent: UA.androidChrome, viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });

  test("Discord: “在电脑上打开” shows the share address with a copy button", async ({ page }) => {
    await page.goto("/");
    await waitForApp(page);
    const discord = card(page, /^Discord 主服务器/);
    // Not in an in-app browser: join navigates normally.
    await expect(discord.getByRole("link", { name: "加入" })).toHaveAttribute("href", "/go/discord");
    await discord.getByRole("button", { name: "在电脑上打开" }).click();

    const dialog = page.getByRole("dialog", { name: "在电脑上打开" });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("在电脑浏览器中打开下面的地址：");
    await expect(dialog).toContainText(`${BASE_URL}/c/discord`);
    await dialog.getByRole("button", { name: "复制链接" }).click();
    await expect(dialog.getByRole("button", { name: "已复制" })).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${BASE_URL}/c/discord`);

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });

  test("only Discord cards with a join target offer “在电脑上打开”", async ({ page }) => {
    await page.goto("/");
    await waitForApp(page);
    const desktop = { name: "在电脑上打开" };
    await expect(card(page, /^Discord 主服务器/).getByRole("button", desktop)).toHaveCount(1);
    await expect(card(page, /^未开启小部件的 Discord/).getByRole("button", desktop)).toHaveCount(1);
    await expect(card(page, /^已解散的服务器/).getByRole("button", desktop)).toHaveCount(0);
    await expect(page.getByRole("button", desktop)).toHaveCount(2);
  });
});

test.describe("desktop browser", () => {
  test("no “在电脑上打开” action", async ({ page }) => {
    await page.goto("/");
    await waitForApp(page);
    await expect(page.getByRole("button", { name: "在电脑上打开" })).toHaveCount(0);
  });
});
