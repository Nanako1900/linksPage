import { createRoot } from "react-dom/client";
import { loadPage } from "../shared/bootstrap";
import { createT } from "../shared/i18n/t";
import { LANG_STORAGE_KEY, resolveLocale } from "../shared/localized";
import type { PublicPage } from "../shared/types/public";
import { classifyNavigator } from "../shared/ua";
import { App } from "./App";
import { ErrorBoundary } from "./ErrorBoundary";
import { ErrorNotice } from "./ErrorNotice";
import { reportFailure } from "./report";
import { matchRoute } from "./route";

function safeStorageGet(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeStorageSet(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Storage may be disabled (private mode, policies); the choice then
    // only lasts for this page view.
  }
}

function enabledLocales(data: PublicPage): readonly string[] {
  return data.site.locales.length > 0 ? data.site.locales : [data.site.defaultLocale];
}

/** Resolve the UI locale: ?lang= → stored choice → browser languages → default. */
export function localeFor(data: PublicPage, loc: Pick<Location, "search">, nav: Pick<Navigator, "languages">): string {
  return resolveLocale({
    enabled: enabledLocales(data),
    defaultLocale: data.site.defaultLocale,
    query: new URLSearchParams(loc.search).get("lang"),
    stored: safeStorageGet(LANG_STORAGE_KEY),
    browser: nav.languages,
  });
}

/** Persist an explicit ?lang= choice so it survives navigation. */
export function rememberLocaleChoice(data: PublicPage, loc: Pick<Location, "search">): void {
  const query = new URLSearchParams(loc.search).get("lang");
  if (query && enabledLocales(data).includes(query)) safeStorageSet(LANG_STORAGE_KEY, query);
}

/** Show a load error while keeping any server-rendered fallback markup. */
function showLoadError(root: HTMLElement): void {
  const lang = document.documentElement.lang || "en";
  const host = document.createElement("div");
  root.prepend(host);
  createRoot(host).render(<ErrorNotice message={createT(lang, "en")("loadError")} />);
}

/** Mount the public app, replacing the server-rendered fallback markup. */
export async function start(root: HTMLElement): Promise<void> {
  let data: PublicPage;
  try {
    data = await loadPage(document);
  } catch (err) {
    reportFailure(err);
    showLoadError(root);
    return;
  }
  const locale = localeFor(data, window.location, navigator);
  rememberLocaleChoice(data, window.location);
  document.documentElement.lang = locale;
  const t = createT(locale, data.site.defaultLocale, data.site.copy);
  createRoot(root).render(
    <ErrorBoundary fallback={<ErrorNotice message={t("loadError")} />}>
      <App page={data} route={matchRoute(window.location.pathname)} locale={locale} ua={classifyNavigator(navigator)} />
    </ErrorBoundary>,
  );
}
