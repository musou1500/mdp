// Package render converts markdown to HTML.
package render

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Renderer turns markdown into HTML fragments. It is safe for concurrent use.
type Renderer struct {
	md goldmark.Markdown
}

// Document is the result of rendering one markdown file.
type Document struct {
	// HTML is a fragment, not a page.
	HTML string
	// Title is the text of the first level-1 heading, or "" when there is none.
	Title string
	// Headings are every heading of the document, in document order.
	Headings []Heading
}

// Heading is one heading of a document. ID is the id the heading carries in
// HTML, so that the viewer's table of contents can link to it. The JSON tags
// are the shape /_md/doc hands to the viewer.
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	ID    string `json:"id"`
}

func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				// Emit class names instead of inline styles so the colors can
				// follow the light/dark stylesheet from HighlightCSS.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		// The files come from the user's own disk, so raw HTML is allowed.
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	return &Renderer{md: md}
}

// Render renders src. docPath is the URL path of the document ("/docs/a.md");
// relative links and images are rewritten to absolute URL paths against it so
// the fragment stays correct wherever it is inserted.
func (r *Renderer) Render(src []byte, docPath string) (Document, error) {
	node := r.md.Parser().Parse(text.NewReader(src))
	headings := identifyHeadings(node, src)
	absolutizeLinks(node, path.Dir(docPath))
	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, src, node); err != nil {
		return Document{}, err
	}
	return Document{HTML: buf.String(), Title: firstLevel1(headings), Headings: headings}, nil
}

func firstLevel1(headings []Heading) string {
	for _, h := range headings {
		if h.Level == 1 {
			return h.Text
		}
	}
	return ""
}

// Title returns the text of the first level-1 heading of src, or "" when the
// document has none. It is the same title Render reports, for callers that
// want only the title: the sidebar tree reads every markdown file, so it parses
// without rendering.
func Title(src []byte) string {
	return firstHeadingText(titleParser().Parse(text.NewReader(src)), src)
}

// goldmark parsers are safe for concurrent use, so one is enough.
var titleParser = sync.OnceValue(func() parser.Parser {
	return goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()
})

// identifyHeadings gives every heading an id and returns them in document
// order. The ids are what the table of contents links to, so they are assigned
// here rather than by goldmark: its own auto-generated ids drop every non-ASCII
// rune, which collapses a document of Japanese headings into "heading",
// "heading-1", ... .
func identifyHeadings(doc ast.Node, src []byte) []Heading {
	var headings []Heading
	used := map[string]bool{}
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		text := strings.TrimSpace(nodeText(h, src))
		if text == "" {
			// Nothing to label it with: a heading holding only an image or raw
			// HTML would be a blank line in the table of contents.
			return ast.WalkSkipChildren, nil
		}
		id := headingID(text, used)
		h.SetAttributeString("id", []byte(id))
		headings = append(headings, Heading{Level: h.Level, Text: text, ID: id})
		return ast.WalkSkipChildren, nil
	})
	return headings
}

// headingID turns heading text into an id, keeping letters of any script. used
// holds the ids already taken by this document; a repeated heading gets "-1",
// "-2", ... appended.
func headingID(text string, used map[string]bool) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "section"
	}
	for base, n := id, 1; used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	used[id] = true
	return id
}

func firstHeadingText(doc ast.Node, src []byte) string {
	var title string
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level != 1 {
			return ast.WalkContinue, nil
		}
		title = nodeText(h, src)
		return ast.WalkStop, nil
	})
	return title
}

// nodeText collects the plain text of a node's inline children.
func nodeText(node ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			b.Write(plainText(n.Segment.Value(src), n.IsRaw()))
		case *ast.String:
			b.Write(plainText(n.Value, n.IsRaw()))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// plainText turns the source bytes of a text node into the text a reader sees.
// goldmark keeps the source as written and lets the HTML renderer resolve it,
// so without this a heading written "100\% done" or "AT&amp;T" would keep its
// backslash or its entity in the title and the table of contents while the body
// shows "100% done" and "AT&T". Raw nodes are what the renderer writes as-is.
func plainText(v []byte, raw bool) []byte {
	if raw {
		return v
	}
	return util.UnescapePunctuations(util.ResolveEntityNames(util.ResolveNumericReferences(v)))
}

// absolutizeLinks rewrites relative destinations against dir, the directory of
// the document being rendered.
func absolutizeLinks(doc ast.Node, dir string) {
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			n.Destination = []byte(absolutize(string(n.Destination), dir))
		case *ast.Image:
			n.Destination = []byte(absolutize(string(n.Destination), dir))
		}
		return ast.WalkContinue, nil
	})
}

// absolutize turns "./img/a.png" into "/docs/img/a.png". Absolute paths, full
// URLs and bare fragments are returned unchanged.
func absolutize(dest, dir string) string {
	if dest == "" {
		return dest
	}
	u, err := url.Parse(dest)
	if err != nil {
		// Not something we can reason about; leave it to the browser.
		return dest
	}
	if u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return dest
	}
	if u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return dest
	}
	trailingSlash := strings.HasSuffix(u.Path, "/")
	u.Path = path.Join(dir, u.Path)
	if trailingSlash && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawPath = ""
	return u.String()
}

var highlightCSS = sync.OnceValue(buildHighlightCSS)

// HighlightCSS returns the class-based chroma stylesheet for both color
// schemes. The two themes use the same selectors, so each one is scoped to its
// own media query: otherwise a token that the dark theme leaves unstyled would
// keep the light theme's color instead of inheriting the dark base color.
func HighlightCSS() []byte {
	return highlightCSS()
}

func buildHighlightCSS() []byte {
	f := chromahtml.New(chromahtml.WithClasses(true))
	var buf bytes.Buffer
	// "light" also matches browsers with no color scheme preference.
	writeScheme(&buf, f, "light", styles.Get("github"))
	writeScheme(&buf, f, "dark", styles.Get("github-dark"))
	return buf.Bytes()
}

func writeScheme(w *bytes.Buffer, f *chromahtml.Formatter, scheme string, style *chroma.Style) {
	var rules bytes.Buffer
	if err := f.WriteCSS(&rules, style); err != nil {
		return
	}
	fmt.Fprintf(w, "@media (prefers-color-scheme: %s) {\n", scheme)
	w.Write(rules.Bytes())
	w.WriteString("}\n")
}
