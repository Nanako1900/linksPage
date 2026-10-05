import { useId } from "react";
import type { CommunityView, QRView } from "../../shared/types/public";
import { Icon } from "../components/icons/Icon";
import { useEnv } from "../context";
import { Actions } from "./Actions";
import { DiscordEmbed } from "./DiscordEmbed";
import { Channels, Members } from "./Members";
import { type CardModel, cardModel } from "./model";
import { StatusLine } from "./StatusLine";

const ICON_SIZE = 48;

function CardIcon({ c, label }: { c: CommunityView; label: string }) {
  const { platforms } = useEnv();
  if (c.icon) {
    return (
      <img
        src={c.icon.url}
        width={ICON_SIZE}
        height={ICON_SIZE}
        alt=""
        decoding="async"
        className="size-12 flex-none rounded-[22%] bg-border object-cover"
      />
    );
  }
  return (
    <span className="flex size-12 flex-none items-center justify-center rounded-[22%] bg-accent-soft text-accent">
      <Icon icon={platforms[c.platform]?.icon ?? null} label={label} size={26} />
    </span>
  );
}

function InlineQr({ qr, name }: { qr: QRView; name: string }) {
  const { t, pick } = useEnv();
  const note = pick(qr.note);
  return (
    <figure className="mt-4">
      <img src={qr.url} width={qr.width} height={qr.height} alt={`${t("qrCode")} · ${name}`} className="lp-qr-img" />
      <figcaption className="mt-2 text-sm text-muted">
        {t("qrLongPress")}
        {note && <span className="block text-fg">{note}</span>}
      </figcaption>
    </figure>
  );
}

/** Group number / contact facts shown as text so they can also be read or selected. */
function Facts({ c }: { c: CommunityView }) {
  const { t, pick } = useEnv();
  if (!c.qq && !c.contact) return null;
  return (
    <dl className="mt-3 grid gap-1 text-sm">
      {c.qq && (
        <div className="flex gap-2">
          <dt className="text-muted">{t("groupNumber")}</dt>
          <dd className="select-all font-mono tabular-nums">{c.qq.groupNumber}</dd>
        </div>
      )}
      {c.contact && (
        <div className="flex flex-wrap gap-x-2">
          <dt className="text-muted">{pick(c.contact.label)}</dt>
          <dd className="select-all font-mono">{c.contact.value}</dd>
        </div>
      )}
    </dl>
  );
}

function useModel(c: CommunityView): CardModel {
  const { t, ua, site, platforms, pick } = useEnv();
  return cardModel(c, {
    ua,
    baseUrl: site.baseUrl,
    platform: platforms[c.platform],
    texts: {
      inviteUnavailable: t("inviteUnavailable"),
      communityUnavailable: t("communityUnavailable"),
      unavailableText: pick(c.unavailableText),
    },
  });
}

/** Horizontal community row card (doc 8.5). */
export function CommunityCard({ c, pinned }: { c: CommunityView; pinned?: boolean }) {
  const { pick, platforms } = useEnv();
  const nameId = useId();
  const m = useModel(c);
  const name = pick(c.name);
  const platformName = pick(platforms[c.platform]?.name);
  const description = pick(c.description);
  return (
    <article className="lp-card" aria-labelledby={nameId} data-state={c.live.state} data-dimmed={m.dimmed}>
      <div className="lp-card-head flex gap-4">
        <CardIcon c={c} label={platformName || name} />
        <div className="min-w-0 flex-1">
          <h3 id={nameId} className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span
              className={`font-semibold leading-snug [overflow-wrap:anywhere] ${pinned ? "text-xl" : "text-[1.0625rem]"}`}
            >
              {name}
            </span>
            {platformName && <span className="lp-tag">{platformName}</span>}
          </h3>
          {!m.dimmed && <StatusLine live={c.live} />}
          {description && <p className="mt-1.5 text-sm text-muted [overflow-wrap:anywhere]">{description}</p>}
        </div>
      </div>
      {m.notice && <p className="mt-3 text-sm text-muted">{m.notice}</p>}
      {!m.dimmed && <Channels channels={c.live.channels} />}
      {!m.dimmed && <Members users={c.live.users} display={c.memberDisplay} />}
      {m.inlineQr && <InlineQr qr={m.inlineQr} name={name} />}
      <Facts c={c} />
      <Actions primary={m.primary} secondary={m.secondary} name={name} />
      {m.embed && c.embed && (
        <DiscordEmbed embed={c.embed} name={name} sharePath={c.sharePath} intercept={m.embedIntercept} />
      )}
    </article>
  );
}
