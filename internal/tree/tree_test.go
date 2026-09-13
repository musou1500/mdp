package tree

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// mkdocroot creates the given files (relative paths), each holding "x", and
// returns them as the file system Build walks, an os.Root like the server's.
func mkdocroot(t *testing.T, files ...string) fs.FS {
	t.Helper()
	contents := make(map[string]string, len(files))
	for _, f := range files {
		contents[f] = "x"
	}
	return mkdocrootWith(t, contents)
}

// mkdocrootWith is mkdocroot with the file contents spelled out.
func mkdocrootWith(t *testing.T, files map[string]string) fs.FS {
	t.Helper()
	dir := t.TempDir()
	for f, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root.FS()
}

// flatten renders the tree as "path" lines, indented by depth, for comparison.
func flatten(nodes []Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Path)
		out = append(out, flatten(n.Children)...)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuild(t *testing.T) {
	root := mkdocroot(t,
		"index.md",
		"CHANGELOG.markdown",
		"logo.png",
		"docs/guide/install.md",
		"docs/guide/usage.md",
		"docs/README.md",
		"assets/only-images/x.png",
		".git/HEAD.md",
		"node_modules/pkg/readme.md",
	)
	nodes, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/docs/",
		"/docs/guide/",
		"/docs/guide/install.md",
		"/docs/guide/usage.md",
		"/docs/README.md",
		"/CHANGELOG.markdown",
		"/index.md",
	}
	if got := flatten(nodes); !equal(got, want) {
		t.Errorf("tree =\n  %q\nwant\n  %q", got, want)
	}
}

func TestBuildEmpty(t *testing.T) {
	nodes, err := Build(mkdocroot(t, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Errorf("want empty tree, got %v", nodes)
	}
}

func TestBuildNames(t *testing.T) {
	nodes, err := Build(mkdocroot(t, "docs/a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Name != "docs" || !nodes[0].IsDir {
		t.Fatalf("unexpected root nodes: %+v", nodes)
	}
	child := nodes[0].Children[0]
	if child.Name != "a.md" || child.IsDir {
		t.Errorf("unexpected child: %+v", child)
	}
}

func TestBuildTitles(t *testing.T) {
	root := mkdocrootWith(t, map[string]string{
		"titled.md":   "# はじめに\n\n本文\n",
		"untitled.md": "本文だけ\n",
		"h2only.md":   "## 小見出し\n",
		"fenced.md":   "```sh\n# not a heading\n```\n\n# 本当の見出し\n",
	})
	nodes, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"fenced.md":   "本当の見出し",
		"h2only.md":   "",
		"titled.md":   "はじめに",
		"untitled.md": "",
	}
	for _, n := range nodes {
		if got := n.Title; got != want[n.Name] {
			t.Errorf("%s title = %q, want %q", n.Name, got, want[n.Name])
		}
	}
	if len(nodes) != len(want) {
		t.Errorf("got %d nodes, want %d", len(nodes), len(want))
	}
}

func TestIsMarkdown(t *testing.T) {
	for _, name := range []string{"a.md", "a.MD", "a.markdown", "dir/a.md"} {
		if !IsMarkdown(name) {
			t.Errorf("IsMarkdown(%q) = false", name)
		}
	}
	for _, name := range []string{"a.png", "a", "a.mdx", "md"} {
		if IsMarkdown(name) {
			t.Errorf("IsMarkdown(%q) = true", name)
		}
	}
}
