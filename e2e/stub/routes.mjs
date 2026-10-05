// Maps upstream API requests to recorded fixtures. The IDs match
// e2e/seed/seed.yaml; anything unknown answers like the real upstream
// does for a missing guild/invite.

/** Discord guild ID → widget fixture. */
export const WIDGETS = new Map([
  ["1114391825336250432", "widget_ok"],
  ["662267976984297473", "widget_disabled"],
  ["100000000000000000", "widget_unknown_guild"],
]);

/** Discord invite code → invite fixture. */
export const INVITES = new Map([["KwdRuAkT", "invite_ok"]]);

/** The only public KOOK guild in the fixtures. */
export const KOOK_PUBLIC_GUILD = "5417470909511807";

const WIDGET_PATH = /^\/api\/guilds\/(\d{1,20})\/widget\.json$/;
const INVITE_PATH = /^\/api\/v10\/invites\/([A-Za-z0-9-]{1,64})$/;
const BADGE_PATH = "/api/v3/badge/guild";

function kookBadge(query) {
  const style = query.get("style") ?? "0";
  const guild = query.get("guild_id") ?? "";
  if (guild !== KOOK_PUBLIC_GUILD) {
    return { provider: "kook", name: style === "0" ? "badge_not_public_style0" : "badge_not_public_style2" };
  }
  // The real endpoint treats unknown styles like style=0.
  const known = ["0", "1", "2"].includes(style) ? style : "0";
  return { provider: "kook", name: `badge_public_style${known}` };
}

/**
 * Resolve a request to a fixture reference, or null for "no such route".
 * @param {string} method
 * @param {URL} url
 * @returns {{ provider: string, name: string } | null}
 */
export function resolveRoute(method, url) {
  if (method !== "GET") return null;
  const widget = WIDGET_PATH.exec(url.pathname);
  if (widget) return { provider: "discord", name: WIDGETS.get(widget[1]) ?? "widget_unknown_guild" };
  const invite = INVITE_PATH.exec(url.pathname);
  if (invite) return { provider: "discord", name: INVITES.get(invite[1]) ?? "invite_unknown" };
  if (url.pathname === BADGE_PATH) return kookBadge(url.searchParams);
  return null;
}
