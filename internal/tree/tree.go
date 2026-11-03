// Package tree lists the markdown files under a document root.
package tree

import (
	"io/fs"
	"path"
	"strings"

	"mdp/internal/render"
)

// Node is one entry of the sidebar tree. Path is the URL path of the entry,
// unescaped: "/docs/a.md" for a file, "/docs/" for a directory. Title is the
// document's own title, empty for directories and for files without one.
type Node struct {
	Name     string `json:"name"`
	Title    string `json:"title,omitempty"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Children []Node `json:"children,omitempty"`
}

// Build walks fsys, the document root, and returns its markdown files as a
// tree. Directories that contain no markdown at any depth are left out.
// Directories come before files, each group sorted by name.
func Build(fsys fs.FS) ([]Node, error) {
	return buildDir(fsys, ".", "/")
}

func buildDir(fsys fs.FS, dir, urlDir string) ([]Node, error) {
	// fs.ReadDir sorts by filename, which gives us the ordering we want within
	// each of the two groups below.
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var dirs, files []Node
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir():
			if SkipDir(name) {
				continue
			}
			childURL := urlDir + name + "/"
			children, err := buildDir(fsys, path.Join(dir, name), childURL)
			if err != nil {
				return nil, err
			}
			if len(children) == 0 {
				continue
			}
			dirs = append(dirs, Node{Name: name, Path: childURL, IsDir: true, Children: children})
		case IsMarkdown(name):
			title := title(fsys, path.Join(dir, name))
			files = append(files, Node{Name: name, Title: title, Path: urlDir + name})
		}
	}
	return append(dirs, files...), nil
}

// title reads a markdown file and returns its title. A file that cannot be read
// gets no title rather than failing the whole tree: the sidebar can still list
// it, and opening it reports the error.
func title(fsys fs.FS, name string) string {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	return render.Title(src)
}

// SkipDir reports whether a directory is ignored when listing and watching.
func SkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

// IsMarkdown reports whether a file name is a markdown document.
func IsMarkdown(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}
