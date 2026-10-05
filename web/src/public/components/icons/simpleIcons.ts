/**
 * simple-icons paths available on the public page, loaded as a separate
 * chunk so brand paths do not count against the initial bundle. Slugs
 * outside this list render a monogram; extend it when platforms.yaml or
 * common social links need more.
 */
import {
  siBilibili,
  siBluesky,
  siDiscord,
  siGithub,
  siMastodon,
  siMatrix,
  siQq,
  siReddit,
  siSinaweibo,
  siSteam,
  siTeamspeak,
  siTelegram,
  siTwitch,
  siWechat,
  siX,
  siXiaohongshu,
  siYoutube,
  siZhihu,
} from "simple-icons";

const ICONS = [
  siBilibili,
  siBluesky,
  siDiscord,
  siGithub,
  siMastodon,
  siMatrix,
  siQq,
  siReddit,
  siSinaweibo,
  siSteam,
  siTeamspeak,
  siTelegram,
  siTwitch,
  siWechat,
  siX,
  siXiaohongshu,
  siYoutube,
  siZhihu,
];

/** slug → SVG path (24×24 viewBox). */
export const SIMPLE_ICONS: Readonly<Record<string, string>> = Object.fromEntries(ICONS.map((i) => [i.slug, i.path]));
