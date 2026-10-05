import { formatCount, relativeAgo } from "../../shared/format";
import { fmt } from "../../shared/i18n/t";
import type { LiveView } from "../../shared/types/public";
import { useEnv } from "../context";

/** "● 13 在线 · 125 成员 · 16分钟前更新"; a skeleton while pending. */
export function StatusLine({ live }: { live: LiveView }) {
  const { t, locale, now } = useEnv();
  if (live.state === "pending") {
    return (
      <p className="mt-1.5 flex h-5 items-center gap-2">
        <span className="lp-skeleton w-36" aria-hidden="true" />
        <span className="sr-only">{t("loading")}</span>
      </p>
    );
  }
  const parts: string[] = [];
  if (live.members !== null) parts.push(fmt(t("members"), { n: formatCount(live.members, locale) }));
  const ago = live.state === "stale" && live.updatedAt ? relativeAgo(live.updatedAt, now, locale) : null;
  if (ago) parts.push(fmt(t("updatedAgo"), { ago }));
  if (live.online === null && parts.length === 0) return null;
  return (
    <p className="mt-1 flex flex-wrap items-center gap-x-2 text-sm text-muted tabular-nums">
      {live.online !== null && (
        <span className="inline-flex items-center gap-1.5 text-fg">
          <span className="lp-dot" aria-hidden="true" />
          {fmt(t("online"), { n: formatCount(live.online, locale) })}
        </span>
      )}
      {parts.map((p, i) => (
        <span key={p}>
          {(i > 0 || live.online !== null) && (
            <span aria-hidden="true" className="mr-2">
              ·
            </span>
          )}
          {p}
        </span>
      ))}
    </p>
  );
}
