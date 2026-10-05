import { type RenderResult, render } from "@testing-library/react";
import type { ReactElement } from "react";
import { vi } from "vitest";
import { type Env, EnvContext } from "../public/context";
import { createT } from "../shared/i18n/t";
import { pickLocalized } from "../shared/localized";
import type { UAClass } from "../shared/ua";
import { FIXTURE_NOW, samplePage } from "./fixtures";

export const DESKTOP: UAClass = { inWeChat: false, inQQ: false, mobile: false };
export const PHONE: UAClass = { inWeChat: false, inQQ: false, mobile: true };
export const WECHAT: UAClass = { inWeChat: true, inQQ: false, mobile: true };
export const QQ_APP: UAClass = { inWeChat: false, inQQ: true, mobile: true };

/** Env built from the canonical fixture; override any field. */
export function testEnv(patch: Partial<Env> = {}): Env {
  const page = samplePage();
  const locale = patch.locale ?? "zh-CN";
  return {
    locale,
    site: page.site,
    platforms: page.platforms,
    t: createT(locale, page.site.defaultLocale, page.site.copy),
    ua: DESKTOP,
    now: FIXTURE_NOW,
    pick: (text) => pickLocalized(text, locale, page.site.defaultLocale),
    openDialog: vi.fn(),
    ...patch,
  };
}

/** Render inside an EnvContext; returns the env alongside the RTL result. */
export function renderWithEnv(ui: ReactElement, patch: Partial<Env> = {}): RenderResult & { env: Env } {
  const env = testEnv(patch);
  return { env, ...render(<EnvContext value={env}>{ui}</EnvContext>) };
}
