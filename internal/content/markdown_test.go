package content

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestMarkdownRender(t *testing.T) {
	m := NewMarkdown()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"paragraph", "hello", "<p>hello</p>"},
		{"emphasis", "*e* **s** ~~d~~ `c`", "<p><em>e</em> <strong>s</strong> <del>d</del> <code>c</code></p>"},
		{"hard break", "a  \nb", "<p>a<br>\nb</p>"},
		{"newline is a hard break", "a\nb", "<p>a<br>\nb</p>"},
		{"reference link not parsed", "[r]: https://a.example\n\n[r]", "<p>[r]: https://a.example</p>\n<p>[r]</p>"},
		{"https link", "[x](https://Example.com/a)", `<p><a href="https://Example.com/a" rel="nofollow noreferrer noopener" target="_blank">x</a></p>`},
		{"http link", "[x](http://a.example)", `<p><a href="http://a.example" rel="nofollow noreferrer noopener" target="_blank">x</a></p>`},
		{"mailto link", "[m](mailto:hi@example.com)", `<p><a href="mailto:hi@example.com" rel="nofollow noreferrer">m</a></p>`},
		{"javascript link", "[x](javascript:alert(1))", "<p>x</p>"},
		{"data link", "[x](data:text/html,hi)", "<p>x</p>"},
		{"relative link", "[x](/admin)", "<p>x</p>"},
		{"hostless link", "[x](http:)", "<p>x</p>"},
		{"credential link", "[x](https://u:p@a.example)", "<p>x</p>"},
		{"heading not parsed", "# Title", "<p># Title</p>"},
		{"thematic break not parsed", "---", "<p>---</p>"},
		{"image as alt text", "![alt text](https://x.example/a.png)", "<p>alt text</p>"},
		{"raw html escaped", "<b>x</b>", "<p>&lt;b&gt;x&lt;/b&gt;</p>"},
		{"html block escaped", "<div onclick=\"x\">y</div>", "<p>&lt;div onclick=&#34;x&#34;&gt;y&lt;/div&gt;</p>"},
		{"script escaped", "<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>"},
		{"bare url not linked", "https://bare.example", "<p>https://bare.example</p>"},
		{"autolink not parsed", "<https://x.example>", "<p>&lt;https://x.example&gt;</p>"},
		{"table not parsed", "| a |\n|---|\n| 1 |", "<p>| a |<br>\n|---|<br>\n| 1 |</p>"},
		{"unordered list", "- a\n- b", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>"},
		{"ordered start", "3. a\n4. b", "<ol start=\"3\">\n<li>a</li>\n<li>b</li>\n</ol>"},
		{"blockquote", "> q", "<blockquote>\n<p>q</p>\n</blockquote>"},
		{"indented code as text", "    <x> code", "<p>&lt;x&gt; code</p>"},
		{"fence as inline code", "```\nf\n```", "<p><code>f</code></p>"},
		{"invalid utf8", "a\xffb", "<p>a�b</p>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Render(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Render(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMarkdownTooLong(t *testing.T) {
	m := NewMarkdown()
	if _, err := m.Render(strings.Repeat("a", MaxMarkdownBytes)); err != nil {
		t.Fatalf("limit-sized input rejected: %v", err)
	}
	if _, err := m.Render(strings.Repeat("a", MaxMarkdownBytes+1)); !errors.Is(err, ErrMarkdownTooLong) {
		t.Fatalf("err = %v, want ErrMarkdownTooLong", err)
	}
}

var (
	tagRe  = regexp.MustCompile(`<\s*/?\s*([a-zA-Z0-9]+)[^>]*>`)
	hrefRe = regexp.MustCompile(`href="([^"]*)"`)
	attrRe = regexp.MustCompile(`\s([a-zA-Z-]+)=`)
)

var allowedTags = map[string]bool{
	"p": true, "br": true, "em": true, "strong": true, "del": true, "code": true,
	"ul": true, "ol": true, "li": true, "blockquote": true, "a": true,
}

var allowedAttrs = map[string]bool{"href": true, "rel": true, "target": true, "start": true}

// checkSanitized asserts the structural guarantees of Render's output.
func checkSanitized(t *testing.T, in, out string) {
	t.Helper()
	// Text never contains a raw "<" (it is escaped), so every "<" starts a tag.
	if strings.Count(out, "<") != len(tagRe.FindAllString(out, -1)) {
		t.Fatalf("Render(%q) produced a malformed tag: %q", in, out)
	}
	for _, m := range tagRe.FindAllStringSubmatch(out, -1) {
		if !allowedTags[strings.ToLower(m[1])] {
			t.Fatalf("Render(%q) produced tag %q: %q", in, m[1], out)
		}
		for _, a := range attrRe.FindAllStringSubmatch(m[0], -1) {
			if !allowedAttrs[strings.ToLower(a[1])] {
				t.Fatalf("Render(%q) produced attribute %q: %q", in, a[1], out)
			}
		}
	}
	for _, m := range hrefRe.FindAllStringSubmatch(out, -1) {
		h := strings.ToLower(m[1])
		if !strings.HasPrefix(h, "https://") && !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "mailto:") {
			t.Fatalf("Render(%q) produced href %q", in, m[1])
		}
	}
}

func FuzzMarkdownRender(f *testing.F) {
	seeds := []string{
		"", "**x**", "[a](javascript:alert(1))", "<script>x</script>", "![i](https://x/y.png)",
		"[a](https://x.example \"t\")", "<a href=\"javascript:x\">y</a>", "[a]: javascript:x\n\n[b][a]",
		"- [x](mailto:a@b.c)\n  > q", "`<img src=x onerror=alert(1)>`", "[x](HTTPS://A.B)",
		"[x](jav&#x09;ascript:alert(1))", "<<script>>", "&lt;b&gt;",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	m := NewMarkdown()
	f.Fuzz(func(t *testing.T, in string) {
		out, err := m.Render(in)
		if len(in) > MaxMarkdownBytes {
			if !errors.Is(err, ErrMarkdownTooLong) {
				t.Fatalf("oversized input: err = %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Render(%q): %v", in, err)
		}
		checkSanitized(t, in, out)
	})
}
