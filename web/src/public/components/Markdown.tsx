import { createElement, type ReactNode, useMemo } from "react";
import { type MdBlock, parseMarkdown } from "../../shared/markdown/block";
import type { Inline } from "../../shared/markdown/inline";

// Children are passed as variadic createElement arguments (not arrays), so
// the static parse tree needs no keys.

function inline(n: Inline): ReactNode {
  switch (n.t) {
    case "text":
      return n.v;
    case "br":
      return createElement("br");
    case "code":
      return createElement("code", { className: "lp-md-code" }, n.v);
    case "a":
      return createElement(
        "a",
        { href: n.href, target: "_blank", rel: "nofollow noopener noreferrer", className: "lp-md-link" },
        ...n.c.map(inline),
      );
    default:
      return createElement(n.t, null, ...n.c.map(inline));
  }
}

/** A list item; loose lists wrap the text in a paragraph (as the server does). */
function item(it: Inline[], loose: boolean): ReactNode {
  const content = it.map(inline);
  return createElement("li", null, ...(loose ? [createElement("p", null, ...content)] : content));
}

function block(b: MdBlock): ReactNode {
  switch (b.t) {
    case "p":
      return createElement("p", null, ...b.c.map(inline));
    case "quote":
      return createElement("blockquote", null, ...b.c.map(block));
    case "ul":
      return createElement("ul", null, ...b.items.map((it) => item(it, b.loose)));
    case "ol":
      return createElement(
        "ol",
        { start: b.start === 1 ? undefined : b.start },
        ...b.items.map((it) => item(it, b.loose)),
      );
  }
}

interface MarkdownProps {
  source: string;
  className?: string;
}

/** Render the public Markdown subset as React elements (never innerHTML). */
export function Markdown({ source, className }: MarkdownProps) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  if (blocks.length === 0) return null;
  return createElement("div", { className: `lp-md ${className ?? ""}`.trim() }, ...blocks.map(block));
}
