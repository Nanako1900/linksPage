import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

const TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"];
const BLOCKING = new Set(["serious", "critical"]);

export interface AxeFinding {
  id: string;
  impact: string;
  target: string;
  /** The finding matches an entry of KNOWN_ISSUES. */
  known: boolean;
}

/**
 * Known violations, each tracked in docs/m1/notes.md (已知问题). A finding
 * is "known" when its rule matches and its node sits inside `within`.
 * a11y.spec.ts fails once a known issue stops reproducing, so entries
 * cannot outlive the fix.
 */
export const KNOWN_ISSUES: ReadonlyArray<{ id: string; within: string }> = [];

async function isKnown(page: Page, id: string, target: string): Promise<boolean> {
  const candidates = KNOWN_ISSUES.filter((k) => k.id === id);
  if (candidates.length === 0) return false;
  return page.evaluate(
    ({ selector, scopes }) => {
      const el = document.querySelector(selector);
      return el !== null && scopes.some((s) => el.closest(s) !== null);
    },
    { selector: target, scopes: candidates.map((k) => k.within) },
  );
}

/** Serious and critical axe findings on the current page. */
export async function blockingFindings(page: Page): Promise<AxeFinding[]> {
  const results = await new AxeBuilder({ page }).withTags(TAGS).analyze();
  const findings: AxeFinding[] = [];
  for (const v of results.violations) {
    if (!BLOCKING.has(v.impact ?? "")) continue;
    for (const node of v.nodes) {
      const target = node.target.map(String).join(" ");
      findings.push({ id: v.id, impact: v.impact ?? "", target, known: await isKnown(page, v.id, target) });
    }
  }
  return findings;
}
