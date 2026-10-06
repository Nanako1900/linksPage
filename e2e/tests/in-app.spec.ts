// WeChat / QQ in-app browsers (contract section 7, doc 5.7): Discord needs
// an external browser, so the page shows the open-in-browser overlay and
// /go/{slug} answers with the guide page instead of a 302.
import { BASE_URL, UA } from "./support/env";
import { card, expect, test, waitForApp } from "./support/fixtures";

const DISCORD = /^Discord 主服务器/;
const INVITE = "https://discord.gg/KwdRuAkT";

for (const [name, userAgent] of [
  ["WeChat iOS", UA.wechatIOS],
  ["WeChat Android", UA.wechatAndroid],
  ["QQ Android", UA.qqAndroid],
] as const) {
  test.describe(name, () => {
    test.use({ userAgent, viewport: { width: 375, height: 812 }, hasTouch: true, isMobile: true });

    test("Discord join opens the open-in-browser overlay", async ({ page }) => {
      const navigations: string[] = [];
      page.on("request", (req) => {
        if (req.isNavigationRequest()) navigations.push(req.url());
      });
      await page.goto("/");
      await waitForApp(page);
      await card(page, DISCORD).getByRole("link", { name: "加入" }).click();

      const overlay = page.getByRole("dialog", { name: "请在浏览器中打开" });
      await expect(overlay).toBeVisible();
      // site.copy overrides the built-in text (seed.yaml).
      await expect(overlay).toContainText("点右上角 ··· 选择「在浏览器打开」（E2E）");
      await expect(overlay).toContainText(`${BASE_URL}/go/discord`);
      expect(navigations).toEqual([`${BASE_URL}/`]);

      await overlay.getByRole("button", { name: "关闭" }).click();
      await expect(overlay).toBeHidden();
    });

    test("/go/discord returns the guide page, not a redirect", async ({ request }) => {
      const res = await request.get("/go/discord", { headers: { "User-Agent": userAgent }, maxRedirects: 0 });
      expect(res.status()).toBe(200);
      expect(res.headers()["cache-control"]).toBe("no-store");
      expect(res.headers()["x-robots-tag"]).toMatch(/noindex/);
      const html = await res.text();
      expect(html).toContain("lp-page--open-in-browser");
      expect(html).toContain(`${BASE_URL}/go/discord`);
      expect(html).not.toContain(INVITE);
    });
  });
}

test.describe("WeChat: QQ group", () => {
  test.use({ userAgent: UA.wechatIOS, viewport: { width: 375, height: 812 }, isMobile: true });

  test("shows the QR inline and never links to qm.qq.com", async ({ page }) => {
    await page.goto("/");
    await waitForApp(page);
    const qq = card(page, /^QQ 群/);
    await expect(qq.getByRole("img", { name: /二维码/ })).toBeVisible();
    await expect(qq.getByRole("link", { name: "加群" })).toHaveCount(0);
    await expect(qq.getByRole("button", { name: "复制群号" })).toBeVisible();
  });

  test("/go/qq answers 200 with the group number", async ({ request }) => {
    const res = await request.get("/go/qq", { headers: { "User-Agent": UA.wechatIOS }, maxRedirects: 0 });
    expect(res.status()).toBe(200);
    expect(await res.text()).toContain("123456789");
  });
});

test.describe("regular browser", () => {
  test("/go redirects to the target", async ({ request }) => {
    const cases = [
      { path: "/go/discord", location: INVITE },
      { path: "/go/discord-nowidget", location: "https://discord.gg/E2eFallback" },
      { path: "/go/qq", location: "https://qm.qq.com/q/LinksPageE2E" },
      { path: "/go/blog", location: "https://blog.example.com" },
    ];
    for (const c of cases) {
      const res = await request.get(c.path, { maxRedirects: 0 });
      expect(res.status(), c.path).toBe(302);
      expect(res.headers().location, c.path).toBe(c.location);
    }
  });

  test("/go never redirects to a dead community and 404s unknown slugs", async ({ request }) => {
    const gone = await request.get("/go/discord-gone", { maxRedirects: 0 });
    expect(gone.status()).toBe(200);
    expect(await gone.text()).toContain("lp-page--unavailable");
    const missing = await request.get("/go/no-such-slug", { maxRedirects: 0 });
    expect(missing.status()).toBe(404);
  });
});
