/**
 * In-app browser detection (docs/m1/contract.md section 7). Mirrors
 * internal/uaclass.Classify (same markers, including "OpenHarmony"); both
 * are table-tested against the single corpus src/test/fixtures/user-agents.json.
 */
export interface UAClass {
  /** Inside WeChat: the UA contains "MicroMessenger". */
  inWeChat: boolean;
  /** Inside the QQ app WebView: "QQ/". "MQQBrowser" alone is QQ Browser. */
  inQQ: boolean;
  /** Phone or tablet. */
  mobile: boolean;
}

const MOBILE_MARKERS = ["Mobi", "Android", "iPhone", "iPad", "iPod", "OpenHarmony"] as const;

/** Classify a User-Agent string with the fixed rules shared with Go. */
export function classifyUA(ua: string): UAClass {
  return {
    inWeChat: ua.includes("MicroMessenger"),
    inQQ: ua.includes("QQ/"),
    mobile: MOBILE_MARKERS.some((m) => ua.includes(m)),
  };
}

/** True inside WeChat or QQ, where some platforms need an external browser. */
export function isInApp(c: UAClass): boolean {
  return c.inWeChat || c.inQQ;
}

/**
 * Classify the running browser. Beyond the shared string rules, iPadOS in
 * desktop mode reports a Mac UA; touch support tells it apart (the server
 * cannot see this, which only affects the optional "open on desktop" hint).
 */
export function classifyNavigator(nav: Pick<Navigator, "userAgent" | "maxTouchPoints">): UAClass {
  const c = classifyUA(nav.userAgent);
  const desktopIPad = nav.userAgent.includes("Macintosh") && nav.maxTouchPoints > 1;
  return desktopIPad ? { ...c, mobile: true } : c;
}
