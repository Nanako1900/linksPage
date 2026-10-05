import type { Bootstrap } from "../shared/bootstrap";
import type { Translator } from "../shared/i18n/t";
import { localeName, pickLocalized } from "../shared/localized";
import type { Route } from "./route";

interface AppProps {
  data: Bootstrap;
  route: Route;
  locale: string;
  t: Translator;
}

const SECTION_ID = "lp-section";

function SectionHeading({ label, count }: { label: string; count: number }) {
  return (
    <h2
      id={SECTION_ID}
      className="mt-12 mb-4 flex items-baseline gap-3 border-b border-border pb-2 text-xs font-semibold uppercase tracking-[0.18em] text-muted"
    >
      <span>{label}</span>
      <span aria-hidden="true">—</span>
      <span aria-hidden="true" className="tabular-nums">
        {String(count).padStart(2, "0")}
      </span>
    </h2>
  );
}

function Placeholder({ children }: { children: string }) {
  return (
    <p className="rounded-card border border-dashed border-border bg-surface px-5 py-6 text-sm text-muted">
      {children}
    </p>
  );
}

function Body({ route, t }: { route: Route; t: Translator }) {
  switch (route.kind) {
    case "privacy":
      return (
        <section aria-labelledby={SECTION_ID}>
          <SectionHeading label={t("privacy")} count={1} />
          <Placeholder>{t("privacyBody")}</Placeholder>
        </section>
      );
    case "community":
      return (
        <section aria-labelledby={SECTION_ID}>
          <SectionHeading label={t("communityLead")} count={0} />
          <p className="mb-4 font-mono text-sm text-muted">/c/{route.slug}</p>
          <Placeholder>{t("comingSoon")}</Placeholder>
        </section>
      );
    case "notFound":
      // Unreachable in production (the server answers 404 without the
      // app); in the Vite dev server it keeps typos from looking valid.
      return <Placeholder>{t("notFound")}</Placeholder>;
    case "home":
      return (
        <section aria-labelledby={SECTION_ID}>
          <SectionHeading label={t("communities")} count={0} />
          <Placeholder>{t("comingSoon")}</Placeholder>
        </section>
      );
    default:
      return assertNever(route);
  }
}

function assertNever(value: never): never {
  throw new Error(`unhandled route: ${JSON.stringify(value)}`);
}

/** Home link that keeps a non-default language so server and client agree. */
function homeHref(locale: string, defaultLocale: string): string {
  return locale === defaultLocale ? "/" : `/?lang=${encodeURIComponent(locale)}`;
}

export function App({ data, route, locale, t }: AppProps) {
  const { site } = data;
  const title = pickLocalized(site.title, locale, site.defaultLocale);
  const description = pickLocalized(site.description, locale, site.defaultLocale);
  const locales = site.locales ?? [];
  return (
    <div className="mx-auto flex min-h-dvh max-w-[34rem] flex-col px-5 pt-12 pb-[calc(3rem+env(safe-area-inset-bottom))]">
      <header>
        {route.kind !== "home" && (
          <a
            href={homeHref(locale, site.defaultLocale)}
            className="mb-8 inline-flex min-h-11 items-center text-sm text-accent underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          >
            <span aria-hidden="true">←&nbsp;</span>
            {t("backHome")}
          </a>
        )}
        <h1 className="font-display text-4xl leading-[1.15] tracking-[-0.01em] text-fg [overflow-wrap:anywhere]">
          {title}
        </h1>
        {description && <p className="mt-3 text-muted [overflow-wrap:anywhere]">{description}</p>}
      </header>
      <main className="flex-1">
        <Body route={route} t={t} />
      </main>
      <footer className="mt-16 flex flex-wrap items-center justify-between gap-4 border-t border-border pt-4 text-xs text-muted">
        <nav aria-label={t("language")} className="flex gap-3">
          {locales.map((l) => (
            <a
              key={l}
              href={`?lang=${encodeURIComponent(l)}`}
              hrefLang={l}
              lang={l}
              aria-current={l === locale ? "true" : undefined}
              className="inline-flex min-h-11 items-center underline-offset-4 hover:text-fg hover:underline aria-[current=true]:text-fg aria-[current=true]:font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            >
              {localeName(l)}
            </a>
          ))}
        </nav>
        <span>{t("poweredBy")}</span>
      </footer>
    </div>
  );
}
