/** localStorage key holding the visitor's explicit language choice. */
export const LANG_STORAGE_KEY = "lp_lang";

/** Localized content field: BCP-47 locale → text. */
export type LocalizedText = Readonly<Record<string, string>>;

/** Resolve text with the fallback chain locale → defaultLocale → en. */
export function pickLocalized(text: LocalizedText | null | undefined, locale: string, defaultLocale: string): string {
  if (!text) return "";
  for (const l of [locale, defaultLocale, "en"]) {
    const v = text[l];
    if (v) return v;
  }
  return "";
}

/**
 * Choose the UI locale: ?lang= → stored preference → browser languages →
 * default. Only enabled locales are accepted.
 */
export function resolveLocale(opts: {
  enabled: readonly string[];
  defaultLocale: string;
  query?: string | null;
  stored?: string | null;
  browser?: readonly string[];
}): string {
  const { enabled, defaultLocale } = opts;
  const exact = (c: string | null | undefined) => (c && enabled.includes(c) ? c : null);
  const byLanguage = (c: string) => {
    const lang = c.toLowerCase().split("-")[0];
    return enabled.find((e) => e.toLowerCase().split("-")[0] === lang) ?? null;
  };
  const fromBrowser = (opts.browser ?? []).map((c) => exact(c) ?? byLanguage(c)).find((c) => c !== null);
  return exact(opts.query) ?? exact(opts.stored) ?? fromBrowser ?? defaultLocale;
}

/** Human-readable name of a locale in that locale (endonym), e.g. "English", "中文（中国）". */
export function localeName(locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "language" }).of(locale) ?? locale;
  } catch {
    return locale;
  }
}
