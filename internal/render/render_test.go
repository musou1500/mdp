package render

import (
	"reflect"
	"strings"
	"testing"
)

func render(t *testing.T, src, docPath string) (string, string) {
	t.Helper()
	doc := renderDoc(t, src, docPath)
	return doc.HTML, doc.Title
}

func renderDoc(t *testing.T, src, docPath string) Document {
	t.Helper()
	doc, err := New().Render([]byte(src), docPath)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return doc
}

func TestRenderGFM(t *testing.T) {
	src := `# Title

| a | b |
|---|---|
| 1 | 2 |

- [x] done
- [ ] todo

~~gone~~
`
	html, title := render(t, src, "/a.md")
	if title != "Title" {
		t.Errorf("title = %q, want %q", title, "Title")
	}
	for _, want := range []string{"<table>", "<th>a</th>", `type="checkbox"`, "<del>gone</del>"} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q:\n%s", want, html)
		}
	}
}

func TestRenderTitle(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"# Hello `code`\n", "Hello code"},
		{"## Only h2\n\n# Later h1\n", "Later h1"},
		{"no heading at all\n", ""},
		{"## h2 only\n", ""},
		// The body shows what the escapes and references mean, and so must the
		// title: goldmark leaves both to the renderer.
		{"# 100\\% done\n", "100% done"},
		{"# AT&amp;T &#65;\n", "AT&T A"},
		// Code spans are not escaped in the body either.
		{"# a `x\\|y`\n", "a x\\|y"},
	}
	for _, tt := range tests {
		if _, got := render(t, tt.src, "/a.md"); got != tt.want {
			t.Errorf("title of %q = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestRenderHeadings(t *testing.T) {
	src := `# Title

## Getting started

### 見出しのアンカー

## Getting started

## !!!

## 50\% off
`
	doc := renderDoc(t, src, "/a.md")
	want := []Heading{
		{Level: 1, Text: "Title", ID: "title"},
		{Level: 2, Text: "Getting started", ID: "getting-started"},
		{Level: 3, Text: "見出しのアンカー", ID: "見出しのアンカー"},
		// A repeated heading keeps a distinct id.
		{Level: 2, Text: "Getting started", ID: "getting-started-1"},
		// Nothing usable is left of the text.
		{Level: 2, Text: "!!!", ID: "section"},
		// The text a reader sees, not the source: the body shows "50% off".
		{Level: 2, Text: "50% off", ID: "50-off"},
	}
	if !reflect.DeepEqual(doc.Headings, want) {
		t.Errorf("headings = %+v, want %+v", doc.Headings, want)
	}
	// The ids the table of contents links to must exist in the HTML.
	for _, h := range want {
		if !strings.Contains(doc.HTML, `id="`+h.ID+`"`) {
			t.Errorf("html missing id %q:\n%s", h.ID, doc.HTML)
		}
	}
}

func TestRenderNoHeadings(t *testing.T) {
	// A heading whose text is empty (only raw HTML here) would be a blank line
	// in the table of contents, so it is left out too.
	for _, src := range []string{"just text\n", "# <span></span>\n"} {
		if doc := renderDoc(t, src, "/a.md"); len(doc.Headings) != 0 {
			t.Errorf("headings of %q = %+v, want none", src, doc.Headings)
		}
	}
}

func TestRenderHighlight(t *testing.T) {
	html, _ := render(t, "```go\nfunc main() {}\n```\n", "/a.md")
	if !strings.Contains(html, `class="chroma"`) {
		t.Errorf("code block not highlighted:\n%s", html)
	}
	if strings.Contains(html, "style=\"color:") {
		t.Errorf("expected class-based highlighting, got inline styles:\n%s", html)
	}
}

func TestAbsolutize(t *testing.T) {
	tests := []struct {
		dest string
		dir  string
		want string
	}{
		{"./img/a.png", "/docs", "/docs/img/a.png"},
		{"b.md", "/docs", "/docs/b.md"},
		{"../top.md", "/docs/guide", "/docs/top.md"},
		{"sub/", "/docs", "/docs/sub/"},
		{"b.md#frag", "/docs", "/docs/b.md#frag"},
		{"日本語.md", "/docs", "/docs/%E6%97%A5%E6%9C%AC%E8%AA%9E.md"},
		{"a b.md", "/docs", "/docs/a%20b.md"},
		// left alone
		{"/abs.md", "/docs", "/abs.md"},
		{"#frag", "/docs", "#frag"},
		{"https://example.com/x.png", "/docs", "https://example.com/x.png"},
		{"mailto:a@example.com", "/docs", "mailto:a@example.com"},
		{"", "/docs", ""},
	}
	for _, tt := range tests {
		if got := absolutize(tt.dest, tt.dir); got != tt.want {
			t.Errorf("absolutize(%q, %q) = %q, want %q", tt.dest, tt.dir, got, tt.want)
		}
	}
}

func TestRenderRewritesRelativeLinks(t *testing.T) {
	src := "[b](./b.md) ![logo](img/logo.png) [ext](https://example.com) [top](/top.md)\n"
	html, _ := render(t, src, "/docs/guide/a.md")
	for _, want := range []string{
		`href="/docs/guide/b.md"`,
		`src="/docs/guide/img/logo.png"`,
		`href="https://example.com"`,
		`href="/top.md"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %q:\n%s", want, html)
		}
	}
}

func TestRenderRawHTML(t *testing.T) {
	html, _ := render(t, "<div class=\"x\">raw</div>\n", "/a.md")
	if !strings.Contains(html, `<div class="x">raw</div>`) {
		t.Errorf("raw html was escaped:\n%s", html)
	}
}

func TestHighlightCSS(t *testing.T) {
	css := string(HighlightCSS())
	light := strings.Index(css, "@media (prefers-color-scheme: light)")
	dark := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if light < 0 || dark < 0 || light > dark {
		t.Fatalf("want a light query followed by a dark one, got %d and %d", light, dark)
	}
	if strings.Count(css, "@media") != 2 {
		t.Errorf("expected exactly two media queries, got %d", strings.Count(css, "@media"))
	}
	// Every rule must sit inside one of the two queries, so that a token only
	// one theme styles cannot leak its color into the other scheme.
	if strings.Contains(css[:light], ".chroma") {
		t.Errorf("unscoped rules before the light query:\n%s", css[:light])
	}
	if !strings.Contains(css[dark:], "color: #e6edf3") {
		t.Errorf("dark query does not set a base text color:\n%s", css[dark:])
	}
}
