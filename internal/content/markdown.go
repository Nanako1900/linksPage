package content

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// MaxMarkdownBytes bounds Markdown input (bio, footer, text blocks).
const MaxMarkdownBytes = 4096

// ErrMarkdownTooLong is returned for input above MaxMarkdownBytes.
var ErrMarkdownTooLong = errors.New("content: markdown input too long")

// Markdown renders the public Markdown subset: paragraphs, line breaks
// (every newline inside a paragraph is a hard break), emphasis, strong,
// strikethrough, inline code, inline links, ordered/unordered lists and
// blockquotes. Raw HTML, images, headings, tables, autolinks, reference
// links and bare URLs are not rendered. Links pass SafeURL and get
// rel="nofollow noopener noreferrer" target="_blank". The frontend parser
// (web/src/shared/markdown) renders the same subset after hydration; both
// are pinned by the shared corpus web/src/test/fixtures/markdown-cases.json.
type Markdown struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

// NewMarkdown builds the renderer (safe for concurrent use).
func NewMarkdown() *Markdown {
	return &Markdown{md: newGoldmark(), policy: newPolicy()}
}

// newGoldmark configures goldmark with only the subset's parsers. Headings,
// thematic breaks, fenced code and HTML blocks are not parsed at all, so
// "# x" stays a paragraph; raw HTML and autolinks are not parsed inline,
// and link reference definitions are not recognized (the client parser
// has no reference links either).
// Indented code blocks are parsed only so their text is not lost (they
// render as plain paragraphs). goldmark's default (non-unsafe) renderer
// additionally omits raw HTML.
func newGoldmark() goldmark.Markdown {
	p := parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewEmphasisParser(), 500),
		),
	)
	return goldmark.New(
		goldmark.WithParser(p),
		goldmark.WithExtensions(extension.Strikethrough),
		goldmark.WithRendererOptions(
			gmhtml.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(subsetRenderer{}, 100)),
		),
	)
}

// subsetRenderer overrides nodes outside the subset: images render as
// their alt text, indented code blocks as an escaped paragraph.
type subsetRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (subsetRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, func(util.BufWriter, []byte, ast.Node, bool) (ast.WalkStatus, error) {
		return ast.WalkContinue, nil
	})
	reg.Register(ast.KindCodeBlock, renderCodeBlockAsParagraph)
}

func renderCodeBlockAsParagraph(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	var text bytes.Buffer
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		text.Write(seg.Value(source))
	}
	out := "<p>" + html.EscapeString(strings.TrimSpace(text.String())) + "</p>\n"
	if _, err := w.WriteString(out); err != nil {
		return ast.WalkStop, fmt.Errorf("content: write code block: %w", err)
	}
	return ast.WalkSkipChildren, nil
}

var olStartRe = regexp.MustCompile(`^[0-9]{1,9}$`)

// newPolicy is a strict allow-list matching the subset (stricter than
// bluemonday.UGCPolicy, which also allows images, tables and headings).
func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "em", "strong", "del", "code", "ul", "ol", "li", "blockquote")
	p.AllowAttrs("start").Matching(olStartRe).OnElements("ol")
	p.AllowAttrs("href").OnElements("a")
	for _, scheme := range AllowedSchemes {
		p.AllowURLSchemeWithCustomPolicy(scheme, func(u *url.URL) bool {
			_, err := SafeURL(u.String())
			return err == nil
		})
	}
	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(false)
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}

// Render returns sanitized HTML for src (to be wrapped in template.HTML by
// the caller).
func (m *Markdown) Render(src string) (string, error) {
	if len(src) > MaxMarkdownBytes {
		return "", ErrMarkdownTooLong
	}
	src = strings.ToValidUTF8(src, "�")
	var buf bytes.Buffer
	if err := m.md.Convert([]byte(src), &buf); err != nil {
		return "", fmt.Errorf("content: render markdown: %w", err)
	}
	return strings.TrimSpace(m.policy.Sanitize(buf.String())), nil
}
