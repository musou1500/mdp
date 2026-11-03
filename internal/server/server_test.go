package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/musou1500/mdp/internal/render"
	"github.com/musou1500/mdp/internal/tree"
)

// newTestServer serves a document root holding the given files, as
// "relative/path" => "contents" pairs.
func newTestServer(t *testing.T, files map[string]string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h, err := New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, root
}

func get(t *testing.T, ts *httptest.Server, url string) (*http.Response, string) {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func TestServeStaticFile(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"img/logo.png": "PNGDATA"})
	resp, body := get(t, ts, "/img/logo.png")
	if resp.StatusCode != 200 || body != "PNGDATA" {
		t.Errorf("got %d %q", resp.StatusCode, body)
	}
}

func TestServeMarkdownReturnsShell(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# Hi\n"})
	resp, body := get(t, ts, "/a.md")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type %q", ct)
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Errorf("not the viewer shell: %q", body)
	}
	// The shell is the same for every document; the body is fetched separately.
	if strings.Contains(body, "Hi") {
		t.Errorf("shell should not contain the document: %q", body)
	}
}

func TestServeMarkdownRaw(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# Hi\n"})
	resp, body := get(t, ts, "/a.md?raw")
	if resp.StatusCode != 200 || body != "# Hi\n" {
		t.Errorf("got %d %q", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type %q", ct)
	}
}

func TestServeMissingMarkdown(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# Hi\n"})
	if resp, _ := get(t, ts, "/nope.md"); resp.StatusCode != 404 {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestServeDirectoryIndex(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{
		"README.md":     "# Root\n",
		"empty/pic.png": "x",
	})
	if resp, body := get(t, ts, "/"); resp.StatusCode != 200 || !strings.Contains(body, "<!doctype html>") {
		t.Errorf("root: got %d %q", resp.StatusCode, body)
	}
	// Without a markdown index a directory falls back to the file listing.
	resp, body := get(t, ts, "/empty/")
	if resp.StatusCode != 200 || !strings.Contains(body, "pic.png") {
		t.Errorf("listing: got %d %q", resp.StatusCode, body)
	}
}

func TestServeTraversal(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# Hi\n"})
	for _, url := range []string{
		"/../../etc/passwd",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/_md/doc?path=/../../etc/passwd",
		"/_md/doc?path=../../etc/passwd",
	} {
		resp, body := get(t, ts, url)
		if resp.StatusCode == 200 && strings.Contains(body, "root:") {
			t.Errorf("%s leaked /etc/passwd", url)
		}
	}
}

func TestTreeEndpoint(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{
		"index.md":       "no heading\n",
		"docs/guide.md":  "# Guide\n",
		"img/logo.png":   "x",
		".git/config.md": "x",
	})
	resp, body := get(t, ts, "/_md/tree")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var nodes []tree.Node
	if err := json.Unmarshal([]byte(body), &nodes); err != nil {
		t.Fatalf("bad json %q: %v", body, err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want docs/ and index.md, got %+v", nodes)
	}
	if nodes[0].Path != "/docs/" || !nodes[0].IsDir || nodes[1].Path != "/index.md" {
		t.Errorf("unexpected tree %+v", nodes)
	}
	// The sidebar shows the title next to the file name, and leaves it out for
	// a document without one rather than repeating the file name.
	if got := nodes[0].Children[0].Title; got != "Guide" {
		t.Errorf("guide.md title = %q, want %q", got, "Guide")
	}
	if got := nodes[1].Title; got != "" {
		t.Errorf("index.md title = %q, want empty", got)
	}
	if strings.Contains(body, `"title":""`) {
		t.Errorf("empty title should be omitted from the JSON: %s", body)
	}
}

func TestTreeEndpointEmpty(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.png": "x"})
	_, body := get(t, ts, "/_md/tree")
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("want [], got %q", body)
	}
}

func TestDocEndpoint(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{
		"docs/a.md": "# Title\n\n[b](./b.md)\n",
		"README.md": "# Root readme\n",
	})

	var d doc
	resp, body := get(t, ts, "/_md/doc?path=/docs/a.md")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("bad json %q: %v", body, err)
	}
	if d.Path != "/docs/a.md" || d.Title != "Title" {
		t.Errorf("got %+v", d)
	}
	if !strings.Contains(d.HTML, `href="/docs/b.md"`) {
		t.Errorf("relative link not resolved: %q", d.HTML)
	}

	// A directory resolves to its markdown index.
	_, body = get(t, ts, "/_md/doc?path=/")
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Path != "/README.md" || d.Title != "Root readme" {
		t.Errorf("directory index: got %+v", d)
	}
}

func TestDocEndpointHeadings(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# A\n\n## B\n", "plain.md": "no heading\n"})
	var d doc
	_, body := get(t, ts, "/_md/doc?path=/a.md")
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	want := []render.Heading{{Level: 1, Text: "A", ID: "a"}, {Level: 2, Text: "B", ID: "b"}}
	if !reflect.DeepEqual(d.Headings, want) {
		t.Errorf("headings = %+v, want %+v", d.Headings, want)
	}

	// A document without headings still reports a list, not null.
	_, body = get(t, ts, "/_md/doc?path=/plain.md")
	if !strings.Contains(body, `"headings":[]`) {
		t.Errorf("want an empty headings array, got %s", body)
	}
}

func TestDocEndpointTitleFallsBackToFileName(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "no heading\n"})
	var d doc
	_, body := get(t, ts, "/_md/doc?path=/a.md")
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Title != "a.md" {
		t.Errorf("title = %q, want %q", d.Title, "a.md")
	}
}

func TestDocEndpointErrors(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# A\n", "img/logo.png": "x"})
	for _, path := range []string{"/nope.md", "/img/logo.png", "/img/"} {
		resp, body := get(t, ts, "/_md/doc?path="+path)
		if resp.StatusCode != 404 {
			t.Errorf("%s: status %d, want 404", path, resp.StatusCode)
		}
		if !strings.Contains(body, `"error"`) {
			t.Errorf("%s: want a json error, got %q", path, body)
		}
	}
}

func TestHighlightCSSEndpoint(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, body := get(t, ts, "/_md/highlight.css")
	if resp.StatusCode != 200 || !strings.Contains(body, ".chroma") {
		t.Errorf("got %d %q", resp.StatusCode, body[:min(len(body), 80)])
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content type %q", ct)
	}
}

func TestEventsEndpoint(t *testing.T) {
	ts, root := newTestServer(t, map[string]string{"a.md": "# A\n"})

	resp, err := ts.Client().Get(ts.URL + "/_md/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	// A stream only sees changes it is already waiting for, so let the handler
	// reach its select before touching the file.
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	type line struct{ text string }
	lines := make(chan line, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				lines <- line{scanner.Text()}
				return
			}
		}
	}()

	select {
	case l := <-lines:
		if l.text != "data: change" {
			t.Errorf("got %q, want \"data: change\"", l.text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event received")
	}
}

// Watching stops when the context passed to New is cancelled, and the open
// event streams have to end with it: they are what a graceful shutdown waits on.
func TestEventsEndpointEndsWhenWatchingStops(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	h, err := New(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/_md/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	ended := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(resp.Body)
		ended <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-ended:
		if err != nil {
			t.Errorf("reading the stream: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the stream did not end")
	}
}

func TestImmutable(t *testing.T) {
	h := immutable(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("asset"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/_md/assets/app-abc123.js", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("cache-control %q", cc)
	}
}

func TestShellIsNotCached(t *testing.T) {
	ts, _ := newTestServer(t, map[string]string{"a.md": "# A\n"})
	resp, _ := get(t, ts, "/a.md")
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control %q, want no-store", cc)
	}
}

func TestRelName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/", ".", true},
		{"/a.md", "a.md", true},
		{"/docs/", "docs", true},
		{"/docs/a.md", "docs/a.md", true},
		{"/../a.md", "", false},
		{"a.md", "", false},
	}
	for _, tt := range tests {
		got, ok := relName(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("relName(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
