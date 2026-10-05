package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// markdownCorpus is shared with the client parser (web/src/shared/markdown):
// both must render every case to the same normalized HTML.
var markdownCorpus = filepath.Join("..", "..", "web", "src", "test", "fixtures", "markdown-cases.json")

var presentationalAttrRe = regexp.MustCompile(` (rel|target|class)="[^"]*"`)

var quoteEntities = strings.NewReplacer("&#34;", `"`, "&quot;", `"`, "&#39;", "'", "&#x27;", "'")

// normalizeHTML mirrors normalizeHTML in web/src/public/components/markdown.corpus.test.tsx.
func normalizeHTML(h string) string {
	h = strings.ReplaceAll(h, "\n", "")
	h = presentationalAttrRe.ReplaceAllString(h, "")
	return quoteEntities.Replace(h)
}

func TestMarkdownSharedCorpus(t *testing.T) {
	data, err := os.ReadFile(markdownCorpus)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var doc struct {
		Cases []struct {
			Name     string `json:"name"`
			Markdown string `json:"markdown"`
			HTML     string `json:"html"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(doc.Cases) < 30 {
		t.Fatalf("corpus has %d cases, want at least 30", len(doc.Cases))
	}
	m := NewMarkdown()
	for _, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := m.Render(c.Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if n := normalizeHTML(got); n != c.HTML {
				t.Errorf("Render(%q)\n got %q\nwant %q", c.Markdown, n, c.HTML)
			}
		})
	}
}
