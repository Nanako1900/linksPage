import { describe, expect, it } from "vitest";
import { parseMarkdown } from "./block";
import { type Inline, parseInline, safeHref } from "./inline";

const text = (v: string): Inline => ({ t: "text", v });

describe("safeHref", () => {
  it.each([
    ["https://example.com/a?b=c", "https://example.com/a?b=c"],
    ["  http://example.com ", "http://example.com"],
    ["mailto:hi@example.com", "mailto:hi@example.com"],
    ["HTTPS://EXAMPLE.COM", "HTTPS://EXAMPLE.COM"],
    ["javascript:alert(1)", null],
    ["JaVaScRiPt:alert(1)", null],
    ["data:text/html,x", null],
    ["//evil.example", null],
    ["/relative", null],
    ["https://exa mple.com", null],
    ["https://example.com/\u0000", null],
    ["", null],
    [`https://${"a".repeat(2050)}`, null],
  ])("%s", (raw, want) => {
    expect(safeHref(raw)).toBe(want);
  });
});

describe("parseInline", () => {
  it.each<[string, string, Inline[]]>([
    ["plain", "hello", [text("hello")]],
    ["strong", "a **b** c", [text("a "), { t: "strong", c: [text("b")] }, text(" c")]],
    ["strong underscore", "__b__", [{ t: "strong", c: [text("b")] }]],
    ["em", "*b*", [{ t: "em", c: [text("b")] }]],
    ["em underscore", "_b_", [{ t: "em", c: [text("b")] }]],
    ["intraword underscore stays literal", "snake_case_name", [text("snake_case_name")]],
    ["intraword star emphasis", "a*b*c", [text("a"), { t: "em", c: [text("b")] }, text("c")]],
    ["del", "~~gone~~", [{ t: "del", c: [text("gone")] }]],
    ["single tilde strikethrough (GFM, like the server)", "~a~", [{ t: "del", c: [text("a")] }]],
    ["unclosed", "**open", [text("**open")]],
    ["space after opener", "* not em*", [text("* not em*")]],
    ["space before closer", "*not em *", [text("*not em *")]],
    [
      "nested strong inside em",
      "*a **b** c*",
      [{ t: "em", c: [text("a "), { t: "strong", c: [text("b")] }, text(" c")] }],
    ],
    ["code", "use `x*y*`", [text("use "), { t: "code", v: "x*y*" }]],
    ["double backtick code", "``a ` b``", [{ t: "code", v: "a ` b" }]],
    ["code strips one space", "`` `x` ``", [{ t: "code", v: "`x`" }]],
    ["unclosed code", "`a", [text("`a")]],
    ["escape", "\\*not\\*", [text("*not*")]],
    ["backslash before letter", "a\\b", [text("a\\b")]],
    ["hard break", "a\nb", [text("a"), { t: "br" }, text("b")]],
    ["trailing spaces trimmed", "a  \nb", [text("a"), { t: "br" }, text("b")]],
    ["backslash break", "a\\\nb", [text("a"), { t: "br" }, text("b")]],
    ["raw html is text", "<b>x</b>", [text("<b>x</b>")]],
  ])("%s", (_name, src, want) => {
    expect(parseInline(src)).toEqual(want);
  });

  it("parses links with an allowed scheme", () => {
    expect(parseInline('see [the **blog**](https://blog.example.com "Blog") now')).toEqual([
      text("see "),
      { t: "a", href: "https://blog.example.com", c: [text("the "), { t: "strong", c: [text("blog")] }] },
      text(" now"),
    ]);
    expect(parseInline("[mail](<mailto:hi@example.com>)")).toEqual([
      { t: "a", href: "mailto:hi@example.com", c: [text("mail")] },
    ]);
  });

  it("drops unsafe links but keeps their label", () => {
    expect(parseInline("[click](javascript:alert)")).toEqual([text("click")]);
    expect(parseInline("[click](javascript:alert(1))")).toEqual([text("click")]);
    expect(parseInline("[x](/relative)")).toEqual([text("x")]);
  });

  it("keeps unmatched brackets literal", () => {
    expect(parseInline("[a] (b)")).toEqual([text("[a] (b)")]);
    expect(parseInline("[a")).toEqual([text("[a")]);
    expect(parseInline("[a [b] c](https://x.example)")).toEqual([
      { t: "a", href: "https://x.example", c: [text("a [b] c")] },
    ]);
  });

  it("renders images as their alt text only", () => {
    expect(parseInline("![logo](https://x.example/a.png)")).toEqual([text("logo")]);
    expect(parseInline("!not image")).toEqual([text("!not image")]);
  });

  it("supports neither autolinks nor bare URLs (like the server)", () => {
    expect(parseInline("<https://x.example>")).toEqual([text("<https://x.example>")]);
    expect(parseInline("https://x.example")).toEqual([text("https://x.example")]);
    expect(parseInline("<ftp://x.example>")).toEqual([text("<ftp://x.example>")]);
  });

  it.each<[string, string, string]>([
    ["balanced parentheses", "[x](https://en.wikipedia.org/wiki/Foo_(bar))", "https://en.wikipedia.org/wiki/Foo_(bar)"],
    ["nested parentheses", "[x](https://a.example/a_(b_(c)))", "https://a.example/a_(b_(c))"],
    ["single-quoted title", "[x](https://a.example 'title')", "https://a.example"],
    ["parenthesized title", "[x](https://a.example (title))", "https://a.example"],
    ["escaped parenthesis", "[x](https://a.example/\\))", "https://a.example/)"],
    ["spaces around destination", "[x](  https://a.example  )", "https://a.example"],
  ])("link destination: %s", (_name, src, href) => {
    expect(parseInline(src)).toEqual([{ t: "a", href, c: [text("x")] }]);
  });

  it.each([
    ["unbalanced parenthesis", "[x](https://a.example/(a)"],
    ["unterminated title", '[x](https://a.example "t)'],
    ["angle destination with newline", "[x](<https://a\n.example>)"],
    ["unterminated angle destination", "[x](<https://a.example)"],
    ["too deeply nested", `[x](https://a.example/${"(".repeat(40)}${")".repeat(40)})`],
  ])("link destination stays literal: %s", (_name, src) => {
    expect(parseInline(src).some((n) => n.t === "a")).toBe(false);
  });
});

describe("parseMarkdown", () => {
  it("splits paragraphs on blank lines", () => {
    expect(parseMarkdown("one\ntwo\n\nthree")).toEqual([
      { t: "p", c: [text("one"), { t: "br" }, text("two")] },
      { t: "p", c: [text("three")] },
    ]);
  });

  it("normalizes CRLF", () => {
    expect(parseMarkdown("a\r\n\r\nb")).toHaveLength(2);
  });

  it("parses unordered and ordered lists", () => {
    expect(parseMarkdown("- a\n- **b**\n  more\n\n- c")).toEqual([
      {
        t: "ul",
        items: [[text("a")], [{ t: "strong", c: [text("b")] }, { t: "br" }, text("more")], [text("c")]],
        loose: true,
      },
    ]);
    expect(parseMarkdown("3. x\n4) y")).toEqual([
      { t: "ol", start: 3, items: [[text("x")], [text("y")]], loose: false },
    ]);
  });

  it("ends a list at a blank line followed by a paragraph", () => {
    expect(parseMarkdown("* a\n\nafter")).toEqual([
      { t: "ul", items: [[text("a")]], loose: false },
      { t: "p", c: [text("after")] },
    ]);
    expect(parseMarkdown("- a\n> q")[1]).toMatchObject({ t: "quote" });
  });

  it("lets a list interrupt a paragraph", () => {
    expect(parseMarkdown("intro\n- a")).toEqual([
      { t: "p", c: [text("intro")] },
      { t: "ul", items: [[text("a")]], loose: false },
    ]);
  });

  it("parses blockquotes recursively", () => {
    expect(parseMarkdown("> quoted\n> - item")).toEqual([
      {
        t: "quote",
        c: [
          { t: "p", c: [text("quoted")] },
          { t: "ul", items: [[text("item")]], loose: false },
        ],
      },
    ]);
  });

  it("continues a quoted paragraph with a lazy line", () => {
    expect(parseMarkdown("> a\nlazy\n\nafter")).toEqual([
      { t: "quote", c: [{ t: "p", c: [text("a"), { t: "br" }, text("lazy")] }] },
      { t: "p", c: [text("after")] },
    ]);
    expect(parseMarkdown("> a\n- item")[1]).toMatchObject({ t: "ul" });
  });

  it("limits blockquote nesting", () => {
    const deep = parseMarkdown(`${">".repeat(20)} x`);
    let node = deep[0];
    let depth = 0;
    while (node?.t === "quote") {
      node = node.c[0];
      depth++;
    }
    expect(depth).toBe(8);
    expect(node?.t).toBe("p");
  });

  it("keeps unsupported syntax literal", () => {
    expect(parseMarkdown("# Title\n---")).toEqual([{ t: "p", c: [text("# Title"), { t: "br" }, text("---")] }]);
  });

  it("returns nothing for blank input", () => {
    expect(parseMarkdown("  \n\n")).toEqual([]);
  });
});
