/** Where the app under test listens (see compose.yaml). */
export const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:8080";
/** The stub upstream (request log at /__stub/requests). */
export const STUB_URL = process.env.E2E_STUB_URL ?? "http://localhost:8090";

/** User agents from web/src/test/fixtures/user-agents.json (shared Go/TS rules). */
export const UA = {
  wechatIOS:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.50(0x1800322d) NetType/WIFI Language/zh_CN",
  wechatAndroid:
    "Mozilla/5.0 (Linux; Android 14; Pixel 7 Build/UQ1A.240205.004; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/130.0.6723.103 Mobile Safari/537.36 XWEB/1300059 MMWEBSDK/20240404 MMWEBID/1234 MicroMessenger/8.0.49.2600(0x28003133) WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64",
  qqAndroid:
    "Mozilla/5.0 (Linux; Android 14; Pixel 7 Build/UQ1A.240205.004; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/130.0.6723.103 Mobile Safari/537.36 V1_AND_SQ_9.0.65_6618_YYB_D QQ/9.0.65.17060 NetType/WIFI WebP/0.3.0 AppTheme/1000 StatusBarHeight/110 StatusBarDensity/2.75 QQTheme/1000 InMagicWin/0 StudyMode/0 CurrentMode/0 CurrentFontScale/1.0 GlobalDensityScale/0.9 AllowLandscape/false InKidsMode/0",
  androidChrome:
    "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
} as const;
