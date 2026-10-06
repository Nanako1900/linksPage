import type { CommunityView, LiveView, PublicPage } from "../shared/types/public";
import fixture from "./fixtures/public-page.json";

/** Fresh deep copy of the canonical contract fixture (public-page.json). */
export function samplePage(): PublicPage {
  return structuredClone(fixture) as unknown as PublicPage;
}

/** Community ids in the canonical fixture, by role. */
export const IDS = {
  discord: "01920000-0000-7000-8000-000000000001",
  discordStale: "01920000-0000-7000-8000-000000000002",
  discordDegraded: "01920000-0000-7000-8000-000000000003",
  kook: "01920000-0000-7000-8000-000000000004",
  qqWithLink: "01920000-0000-7000-8000-000000000005",
  qqNoLink: "01920000-0000-7000-8000-000000000006",
  wechat: "01920000-0000-7000-8000-000000000007",
  kookUnavailable: "01920000-0000-7000-8000-000000000008",
} as const;

/** A community from the fixture with optional overrides (live merged shallowly). */
export function sampleCommunity(
  id: string,
  patch: Partial<Omit<CommunityView, "live">> & { live?: Partial<LiveView> } = {},
): CommunityView {
  const base = samplePage().communities[id];
  if (!base) throw new Error(`fixture has no community ${id}`);
  const { live, ...rest } = patch;
  return { ...base, ...rest, live: { ...base.live, ...live } };
}

/** The fixture's "now" (its generatedAt), in ms. */
export const FIXTURE_NOW = Date.parse(fixture.generatedAt);
