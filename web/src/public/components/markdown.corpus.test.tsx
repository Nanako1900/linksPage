import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import corpus from "../../test/fixtures/markdown-cases.json";
import { Markdown } from "./Markdown";

afterEach(cleanup);

/** Mirrors normalizeHTML in internal/content/markdown_corpus_test.go. */
function normalizeHTML(h: string): string {
  return h
    .replaceAll("\n", "")
    .replace(/ (rel|target|class)="[^"]*"/g, "")
    .replaceAll("&#34;", '"')
    .replaceAll("&quot;", '"')
    .replaceAll("&#39;", "'")
    .replaceAll("&#x27;", "'");
}

describe("Markdown matches the server renderer (shared corpus)", () => {
  it("has the full corpus", () => {
    expect(corpus.cases.length).toBeGreaterThanOrEqual(30);
  });

  it.each(corpus.cases.map((c) => [c.name, c] as const))("%s", (_name, c) => {
    const { container } = render(<Markdown source={c.markdown} />);
    const root = container.firstElementChild;
    expect(normalizeHTML(root?.innerHTML ?? "")).toBe(c.html);
  });
});
