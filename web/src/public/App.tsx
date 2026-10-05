import { useEffect, useMemo, useState } from "react";
import { createT } from "../shared/i18n/t";
import { pickLocalized } from "../shared/localized";
import type { CommunityView, LinkView, PublicPage } from "../shared/types/public";
import type { UAClass } from "../shared/ua";
import { Footer, homeHref } from "./components/Footer";
import { Identity } from "./components/Identity";
import { HomeView, NotFoundView, PinnedView, PrivacyView } from "./components/Views";
import { type DialogSpec, type Env, EnvContext } from "./context";
import { DialogHost } from "./dialogs/DialogHost";
import { type LivePageDeps, useLivePage } from "./hooks/useLivePage";
import { useNow } from "./hooks/useNow";
import type { Route } from "./route";

interface AppProps {
  page: PublicPage;
  route: Route;
  locale: string;
  ua: UAClass;
  /** Injectable for tests; defaults to window fetch/document. */
  liveDeps?: LivePageDeps;
}

export function findCommunity(page: PublicPage, slug: string): CommunityView | undefined {
  return Object.values(page.communities).find((c) => c.slug === slug);
}

/** Social rows live in the identity column; the content column gets the rest. */
export function splitSocial(page: PublicPage): { content: PublicPage; social: LinkView[] } {
  const social = page.blocks.flatMap((b) => (b.kind === "social_row" ? b.linkIds : []));
  return {
    content: { ...page, blocks: page.blocks.filter((b) => b.kind !== "social_row") },
    social: [...new Set(social)].flatMap((id) => page.links[id] ?? []),
  };
}

/** document.title in the visitor's locale (the server renders the default one). */
export function pageTitle(page: PublicPage, route: Route, locale: string): string {
  const pick = (t: Record<string, string>) => pickLocalized(t, locale, page.site.defaultLocale);
  const title = pick(page.site.title);
  const c = route.kind === "community" ? findCommunity(page, route.slug) : undefined;
  return c ? `${pick(c.name)} · ${title}` : title;
}

function Body({ page, route }: { page: PublicPage; route: Route }) {
  switch (route.kind) {
    case "home":
      return <HomeView page={page} />;
    case "privacy":
      return <PrivacyView />;
    case "community": {
      const c = findCommunity(page, route.slug);
      return c ? <PinnedView page={page} community={c} /> : <NotFoundView />;
    }
    case "notFound":
      return <NotFoundView />;
  }
}

function BackHome({ env }: { env: Env }) {
  return (
    <a
      href={homeHref(env.locale, env.site.defaultLocale)}
      className="lp-focus mb-6 inline-flex min-h-11 items-center text-sm text-accent underline-offset-4 hover:underline"
    >
      <span aria-hidden="true">←&nbsp;</span>
      {env.t("backHome")}
    </a>
  );
}

export function App({ page: initial, route, locale, ua, liveDeps }: AppProps) {
  const page = useLivePage(initial, liveDeps);
  const now = useNow(Date.now());
  const [dialog, setDialog] = useState<DialogSpec | null>(null);
  const { site } = page;
  const env = useMemo<Env>(
    () => ({
      locale,
      site,
      platforms: page.platforms,
      t: createT(locale, site.defaultLocale, site.copy),
      ua,
      now,
      pick: (text) => pickLocalized(text, locale, site.defaultLocale),
      openDialog: setDialog,
    }),
    [locale, site, page.platforms, ua, now],
  );
  useEffect(() => {
    const title = pageTitle(page, route, locale);
    if (title) document.title = title;
  }, [page, route, locale]);
  const { content, social } = useMemo(() => splitSocial(page), [page]);
  return (
    <EnvContext value={env}>
      <div className="lp-page mx-auto max-w-[34rem] pt-[calc(2.5rem+env(safe-area-inset-top))] pr-[max(1.25rem,env(safe-area-inset-right))] pb-[calc(3rem+env(safe-area-inset-bottom))] pl-[max(1.25rem,env(safe-area-inset-left))] lg:grid lg:max-w-[70rem] lg:grid-cols-[minmax(0,20rem)_minmax(0,38rem)] lg:content-start lg:justify-between lg:gap-x-16 lg:pt-20">
        <Identity social={social} className="lg:sticky lg:top-20 lg:self-start" />
        <main className="mt-10 lg:mt-0">
          {route.kind !== "home" && <BackHome env={env} />}
          <Body page={content} route={route} />
        </main>
        <Footer className="lg:col-start-2" />
      </div>
      <DialogHost spec={dialog} onClose={() => setDialog(null)} />
    </EnvContext>
  );
}
