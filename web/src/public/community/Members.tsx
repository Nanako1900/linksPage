import { fmt } from "../../shared/i18n/t";
import type { ChannelView, MemberDisplay, UserStatus, UserView } from "../../shared/types/public";
import { Glyph } from "../components/icons/glyphs";
import { useEnv } from "../context";

export const STACK_MAX = 5;
const CHANNELS_MAX = 6;

const STATUS_KEY = { online: "statusOnline", idle: "statusIdle", dnd: "statusDnd" } as const;
const STATUS_CLASS: Readonly<Record<UserStatus, string>> = { online: "", idle: "lp-status-idle", dnd: "lp-status-dnd" };

function Avatar({ user, size }: { user: UserView; size: number }) {
  if (user.avatarUrl) {
    return (
      <img
        src={user.avatarUrl}
        width={size}
        height={size}
        alt=""
        loading="lazy"
        decoding="async"
        className="lp-avatar"
      />
    );
  }
  return <span className="lp-avatar inline-block" style={{ width: size, height: size }} aria-hidden="true" />;
}

/** Collapsed avatar stack (max 5 + "+N") that expands to the member list. */
export function Members({ users, display }: { users: UserView[]; display: MemberDisplay }) {
  const { t } = useEnv();
  if (display === "hidden" || users.length === 0) return null;
  const extra = users.length - STACK_MAX;
  return (
    <details className="lp-members mt-3">
      <summary className="text-sm text-muted">
        <span className="lp-stack flex">
          {users.slice(0, STACK_MAX).map((u, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: users have no stable id; the list is replaced wholesale on each poll
            <Avatar key={i} user={u} size={24} />
          ))}
        </span>
        {extra > 0 && <span className="tabular-nums">{fmt(t("moreCount"), { n: extra })}</span>}
        <span>{t("onlineMembers")}</span>
        <Glyph name="chevron" size={18} className="lp-chevron ml-auto" />
      </summary>
      <ul
        className={display === "avatars" ? "mt-2 flex flex-wrap gap-2" : "mt-2 grid grid-cols-2 gap-x-4 gap-y-2"}
        aria-label={t("onlineMembers")}
      >
        {users.map((u, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: see above
          <li key={i} className="flex min-w-0 items-center gap-2 text-sm">
            <span className="relative flex-none">
              <Avatar user={u} size={32} />
              <span className={`lp-pip absolute -right-0.5 -bottom-0.5 ${STATUS_CLASS[u.status]}`} />
            </span>
            {u.name !== null && <span className="truncate">{u.name}</span>}
            <span className="sr-only">{t(STATUS_KEY[u.status])}</span>
          </li>
        ))}
      </ul>
    </details>
  );
}

/** Channel chips; the rest collapses into "+N". */
export function Channels({ channels }: { channels: ChannelView[] }) {
  const { t } = useEnv();
  if (channels.length === 0) return null;
  const extra = channels.length - CHANNELS_MAX;
  return (
    <ul className="mt-3 flex flex-wrap gap-1.5" aria-label={t("channels")}>
      {channels.slice(0, CHANNELS_MAX).map((c) => (
        <li key={c.id} className="lp-tag normal-case tracking-normal">
          <span aria-hidden="true">#</span>
          {c.name}
        </li>
      ))}
      {extra > 0 && <li className="lp-tag tabular-nums">{fmt(t("moreCount"), { n: extra })}</li>}
    </ul>
  );
}
