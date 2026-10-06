import type { CommunityView, PublicPage } from "../../shared/types/public";
import { CommunityCard } from "../community/CommunityCard";
import { useEnv } from "../context";
import { BlockList, SectionHeading } from "./Blocks";

export const SECTION_ID = "lp-section";

export function Placeholder({ children }: { children: string }) {
  return (
    <p className="rounded-card border border-dashed border-border bg-surface px-5 py-6 text-sm text-muted">
      {children}
    </p>
  );
}

export function NotFoundView() {
  const { site, t, pick } = useEnv();
  return (
    <section aria-labelledby={SECTION_ID} className="pt-2">
      <p className="font-display text-7xl leading-none text-accent tabular-nums" aria-hidden="true">
        404
      </p>
      <h2 id={SECTION_ID} className="mt-4 text-lg text-fg">
        {pick(site.notFound) || t("notFound")}
      </h2>
    </section>
  );
}

export function PrivacyView() {
  const { t } = useEnv();
  return (
    <section aria-labelledby={SECTION_ID}>
      <SectionHeading id={SECTION_ID} label={t("privacy")} />
      <div className="mt-4">
        <Placeholder>{t("privacyBody")}</Placeholder>
      </div>
    </section>
  );
}

/** /c/{slug}: the shared community first, then the rest of the page. */
export function PinnedView({ page, community }: { page: PublicPage; community: CommunityView }) {
  const { t } = useEnv();
  const rest = page.blocks.filter((b) => !(b.kind === "community" && b.communityId === community.id));
  return (
    <>
      <section aria-labelledby={SECTION_ID} className="lp-enter mb-10">
        <SectionHeading id={SECTION_ID} label={t("communityLead")} />
        <div className="mt-4">
          <CommunityCard c={community} pinned />
        </div>
      </section>
      <BlockList page={page} blocks={rest} offset={1} />
    </>
  );
}

export function HomeView({ page }: { page: PublicPage }) {
  const { t } = useEnv();
  if (page.blocks.length === 0) return <Placeholder>{t("comingSoon")}</Placeholder>;
  return <BlockList page={page} blocks={page.blocks} />;
}
