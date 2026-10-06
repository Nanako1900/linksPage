import type { LinkView } from "../../shared/types/public";
import { useEnv } from "../context";
import { SocialRow } from "./Blocks";
import { Markdown } from "./Markdown";

const AVATAR_SIZE = 96;

/** Identity column: avatar, display name (display serif), bio and social links. */
export function Identity({ social, className }: { social: LinkView[]; className?: string }) {
  const { site, pick } = useEnv();
  const name = pick(site.displayName) || pick(site.title);
  const bio = pick(site.bio);
  const description = pick(site.description);
  return (
    <header className={className}>
      {site.avatar && (
        <img
          src={site.avatar.url}
          width={AVATAR_SIZE}
          height={AVATAR_SIZE}
          alt=""
          fetchPriority="high"
          decoding="async"
          className="mb-6 size-24 rounded-full bg-border object-cover"
        />
      )}
      <h1 className="font-display text-[2.5rem] leading-[1.05] tracking-[-0.015em] text-fg [overflow-wrap:anywhere] lg:text-[3.25rem]">
        {name}
      </h1>
      {bio ? (
        <Markdown source={bio} className="mt-4 text-muted leading-relaxed [overflow-wrap:anywhere]" />
      ) : (
        description && <p className="mt-4 text-muted leading-relaxed [overflow-wrap:anywhere]">{description}</p>
      )}
      {social.length > 0 && (
        <div className="mt-5">
          <SocialRow links={social} />
        </div>
      )}
    </header>
  );
}
