// The stack is offline: provider data must have come from the stub, which
// replays internal/provider/*/testdata.
import { STUB_URL } from "./support/env";
import { expect, test } from "./support/fixtures";

interface StubRequest {
  path: string;
  query: string;
  fixture: string | null;
}

test("the app fetched every provider fixture from the stub", async ({ request }) => {
  const res = await request.get(`${STUB_URL}/__stub/requests`);
  expect(res.ok()).toBe(true);
  const seen = new Set(((await res.json()) as StubRequest[]).map((r) => r.fixture));
  for (const fixture of [
    "widget_ok",
    "invite_ok",
    "widget_disabled",
    "widget_unknown_guild",
    "badge_public_style0",
    "badge_public_style2",
  ]) {
    expect(seen, fixture).toContain(fixture);
  }
  expect(seen).not.toContain(null);
});
