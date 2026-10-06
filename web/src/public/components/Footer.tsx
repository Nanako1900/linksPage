import { useState } from "react";
import { localeName } from "../../shared/localized";
import { APPEARANCE_STORAGE_KEY, resolveAppearance } from "../../shared/theme";
import { useEnv } from "../context";
import { Markdown } from "./Markdown";

const MODES = [
  ["light", "appearanceLight"],
  ["dark", "appearanceDark"],
  ["auto", "appearanceAuto"],
] as const;

type Mode = (typeof MODES)[number][0];

function readStoredMode(): Mode {
  try {
    const v = window.localStorage.getItem(APPEARANCE_STORAGE_KEY);
    return v === "light" || v === "dark" ? v : "auto";
  } catch {
    return "auto";
  }
}

/** Apply a visitor-choice appearance the same way the boot script does. */
export function applyAppearance(mode: Mode): void {
  try {
    if (mode === "auto") window.localStorage.removeItem(APPEARANCE_STORAGE_KEY);
    else window.localStorage.setItem(APPEARANCE_STORAGE_KEY, mode);
  } catch {
    // Storage disabled: the choice lasts for this page view only.
  }
  const prefersDark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  const stored = mode === "auto" ? null : mode;
  document.documentElement.setAttribute("data-appearance", resolveAppearance("visitor-choice", prefersDark, stored));
}

function AppearanceToggle() {
  const { t } = useEnv();
  const [mode, setMode] = useState<Mode>(readStoredMode);
  return (
    <fieldset className="flex items-center gap-1">
      <legend className="sr-only">{t("appearance")}</legend>
      {MODES.map(([m, key]) => (
        <button
          key={m}
          type="button"
          aria-pressed={mode === m}
          onClick={() => {
            setMode(m);
            applyAppearance(m);
          }}
          className="lp-btn lp-btn-quiet min-h-11 px-2 text-xs aria-pressed:text-fg aria-pressed:underline"
        >
          {t(key)}
        </button>
      ))}
    </fieldset>
  );
}

/** Home link that keeps a non-default language so server and client agree. */
export function homeHref(locale: string, defaultLocale: string): string {
  return locale === defaultLocale ? "/" : `/?lang=${encodeURIComponent(locale)}`;
}

export function Footer({ className }: { className?: string }) {
  const { site, locale, t, pick } = useEnv();
  const footer = pick(site.footer);
  const privacyHref = locale === site.defaultLocale ? "/privacy" : `/privacy?lang=${encodeURIComponent(locale)}`;
  const linkCls =
    "lp-focus inline-flex min-h-11 items-center underline-offset-4 hover:text-fg hover:underline aria-[current=true]:font-semibold aria-[current=true]:text-fg";
  return (
    <footer className={`mt-16 border-t border-border pt-4 text-xs text-muted ${className ?? ""}`}>
      {footer && <Markdown source={footer} className="mb-3 text-sm" />}
      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-1">
        <nav aria-label={t("language")} className="flex flex-wrap gap-x-3">
          {site.locales.map((l) => (
            <a
              key={l}
              href={`?lang=${encodeURIComponent(l)}`}
              hrefLang={l}
              lang={l}
              aria-current={l === locale ? "true" : undefined}
              className={linkCls}
            >
              {localeName(l)}
            </a>
          ))}
        </nav>
        {site.appearance === "visitor-choice" && <AppearanceToggle />}
        <div className="flex flex-wrap items-center gap-x-4">
          <a href={privacyHref} className={linkCls}>
            {t("privacy")}
          </a>
          {site.showPoweredBy && <span>{t("poweredBy")}</span>}
        </div>
      </div>
    </footer>
  );
}
