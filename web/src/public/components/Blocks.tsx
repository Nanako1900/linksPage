import type { CSSProperties, ReactNode } from "react";
import { displayHost, padCount } from "../../shared/format";
import type { Block, HeadingBlock, LinkView, PublicPage } from "../../shared/types/public";
import { CommunityCard } from "../community/CommunityCard";
import { type Env, useEnv } from "../context";
import { Icon } from "./icons/Icon";
import { Markdown } from "./Markdown";

/** "社区 — 03": label, em dash and a two-digit tabular count. */
export function SectionHeading({ id, label, count }: { id?: string; label: string; count?: number }) {
  return (
    <h2
      id={id}
      className="flex items-baseline gap-3 border-b border-border pb-2 text-xs font-semibold uppercase tracking-[0.18em] text-muted"
    >
      <span>{label}</span>
      {count !== undefined && (
        <>
          <span aria-hidden="true">—</span>
          <span aria-hidden="true" className="tabular-nums text-accent">
            {padCount(count)}
          </span>
          <span className="sr-only">({count})</span>
        </>
      )}
    </h2>
  );
}

function linkRel(l: LinkView): string | undefined {
  return l.relMe ? "me noopener" : undefined;
}

export function LinkRow({ link }: { link: LinkView }) {
  const { pick } = useEnv();
  const label = pick(link.label) || displayHost(link.url);
  return (
    <a href={link.href} rel={linkRel(link)} className="lp-row">
      <span className="flex size-9 flex-none items-center justify-center rounded-[22%] bg-accent-soft text-accent">
        <Icon icon={link.icon} label={label} size={18} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">{label}</span>
        <span className="block truncate text-xs text-muted">{displayHost(link.url)}</span>
      </span>
      <span className="lp-arrow text-muted" aria-hidden="true">
        →
      </span>
    </a>
  );
}

/** Icon-only social links (44px targets, labelled). */
export function SocialRow({ links }: { links: LinkView[] }) {
  const { pick } = useEnv();
  if (links.length === 0) return null;
  return (
    <ul className="-ml-3 flex flex-wrap gap-1">
      {links.map((l) => {
        const label = pick(l.label) || displayHost(l.url);
        return (
          <li key={l.id}>
            <a
              href={l.href}
              rel={linkRel(l)}
              aria-label={label}
              title={label}
              className="lp-btn lp-btn-quiet w-11 px-0"
            >
              <Icon icon={l.icon} label={label} size={20} />
            </a>
          </li>
        );
      })}
    </ul>
  );
}

function Heading({ block }: { block: HeadingBlock }) {
  const { pick } = useEnv();
  return <SectionHeading label={pick(block.text)} count={block.count} />;
}

function renderBlock(block: Block, page: PublicPage, pick: Env["pick"]): ReactNode {
  switch (block.kind) {
    case "heading":
      return <Heading block={block} />;
    case "text":
      return <Markdown source={pick(block.markdown)} className="text-[0.9375rem] leading-relaxed" />;
    case "community": {
      const c = page.communities[block.communityId];
      return c ? <CommunityCard c={c} /> : null;
    }
    case "link": {
      const l = page.links[block.linkId];
      return l ? <LinkRow link={l} /> : null;
    }
    case "social_row":
      return <SocialRow links={block.linkIds.flatMap((id) => page.links[id] ?? [])} />;
  }
}

interface BlockListProps {
  page: PublicPage;
  blocks: Block[];
  /** Entrance stagger offset (items already shown above). */
  offset?: number;
}

/** Page blocks in order, each entering 40ms after the previous one. */
export function BlockList({ page, blocks, offset = 0 }: BlockListProps) {
  const { pick } = useEnv();
  return (
    <div className="flex flex-col gap-3">
      {blocks.map((b, i) => {
        const node = renderBlock(b, page, pick);
        if (node === null) return null;
        const style = { "--i": i + offset } as CSSProperties;
        const spacing = b.kind === "heading" && i > 0 ? "pt-7" : "";
        return (
          <div key={b.id} className={`lp-enter ${spacing}`} style={style}>
            {node}
          </div>
        );
      })}
    </div>
  );
}
