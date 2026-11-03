// Package server serves a document root: static files, a markdown viewer and a
// stream of change signals.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"mdp/internal/render"
	"mdp/internal/tree"
	"mdp/internal/watch"

	"os"
)

// dist holds the built viewer. It is served from /_md/assets/, which is also
// the base path Vite builds against.
//
//go:embed all:dist
var dist embed.FS

// indexNames are the markdown files a directory URL falls back to, in order.
var indexNames = []string{"README.md", "readme.md", "index.md"}

const heartbeat = 30 * time.Second

type server struct {
	root     *os.Root
	files    http.Handler // static files from the document root
	renderer *render.Renderer
	hub      *hub
	done     <-chan struct{} // closed when watching stops, ending the event streams
	shell    []byte          // dist/index.html, the viewer's HTML shell
	mux      *http.ServeMux
}

// New serves root. Watching stops when ctx is cancelled.
func New(ctx context.Context, root string) (http.Handler, error) {
	osRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	shell, err := dist.ReadFile("dist/index.html")
	if err != nil {
		return nil, fmt.Errorf("viewer is missing, run `make web`: %w", err)
	}
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	changes, err := watch.Watch(ctx, root)
	if err != nil {
		return nil, err
	}

	s := &server{
		root:     osRoot,
		files:    http.FileServerFS(osRoot.FS()),
		renderer: render.New(),
		hub:      newHub(changes),
		done:     ctx.Done(),
		shell:    shell,
		mux:      http.NewServeMux(),
	}
	s.mux.HandleFunc("/_md/events", s.handleEvents)
	s.mux.HandleFunc("/_md/tree", s.handleTree)
	s.mux.HandleFunc("/_md/doc", s.handleDoc)
	s.mux.HandleFunc("/_md/highlight.css", s.handleHighlightCSS)
	s.mux.Handle("/_md/assets/", immutable(http.StripPrefix("/_md", http.FileServerFS(assets))))
	s.mux.HandleFunc("/", s.handlePath)
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handlePath serves the document root. Markdown files and directories that
// have a markdown index render the viewer; everything else is a static file.
func (s *server) handlePath(w http.ResponseWriter, r *http.Request) {
	name, ok := relName(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if strings.HasSuffix(r.URL.Path, "/") {
		if _, ok := s.dirIndex(name); ok {
			s.serveShell(w)
			return
		}
		s.files.ServeHTTP(w, r)
		return
	}

	if tree.IsMarkdown(name) {
		if fi, err := s.root.Stat(name); err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Has("raw") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			s.files.ServeHTTP(w, r)
			return
		}
		s.serveShell(w)
		return
	}

	s.files.ServeHTTP(w, r)
}

func (s *server) serveShell(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(s.shell)
}

func (s *server) handleTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := tree.Build(s.root.FS())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if nodes == nil {
		nodes = []tree.Node{}
	}
	writeJSON(w, http.StatusOK, nodes)
}

// doc is the rendered document returned by /_md/doc.
type doc struct {
	Path     string           `json:"path"`
	Title    string           `json:"title"`
	HTML     string           `json:"html"`
	Headings []render.Heading `json:"headings"`
}

func (s *server) handleDoc(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Query().Get("path")
	if urlPath == "" {
		urlPath = "/"
	}
	name, ok := relName(urlPath)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if strings.HasSuffix(urlPath, "/") {
		index, ok := s.dirIndex(name)
		if !ok {
			writeError(w, http.StatusNotFound, "no markdown index in "+urlPath)
			return
		}
		name = index
		urlPath = "/" + index
	}
	if !tree.IsMarkdown(name) {
		writeError(w, http.StatusNotFound, "not a markdown file: "+urlPath)
		return
	}

	src, err := fs.ReadFile(s.root.FS(), name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found: "+urlPath)
			return
		}
		writeError(w, http.StatusInternalServerError, "cannot read "+urlPath)
		return
	}
	rendered, err := s.renderer.Render(src, urlPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	title := rendered.Title
	if title == "" {
		title = path.Base(urlPath)
	}
	if rendered.Headings == nil {
		rendered.Headings = []render.Heading{}
	}
	writeJSON(w, http.StatusOK, doc{
		Path:     urlPath,
		Title:    title,
		HTML:     rendered.HTML,
		Headings: rendered.Headings,
	})
}

func (s *server) handleHighlightCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Write(render.HighlightCSS())
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		// Spent by the change it reports, so it is reread every time around.
		changed := s.hub.wait()
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case <-changed:
			// The payload carries no information: the client rereads the
			// document root when it sees one.
			fmt.Fprint(w, "data: change\n\n")
			flusher.Flush()
		case <-ticker.C:
			// A comment line keeps idle connections from being dropped.
			fmt.Fprint(w, ":\n\n")
			flusher.Flush()
		}
	}
}

// dirIndex returns the markdown index of a directory, as a path relative to the
// document root.
func (s *server) dirIndex(dir string) (string, bool) {
	for _, index := range indexNames {
		name := path.Join(dir, index)
		if fi, err := s.root.Stat(name); err == nil && !fi.IsDir() {
			return name, true
		}
	}
	return "", false
}

// relName converts a URL path into a path relative to the document root, the
// form fs.FS and os.Root expect. "/" becomes ".".
func relName(urlPath string) (string, bool) {
	if !strings.HasPrefix(urlPath, "/") {
		return "", false
	}
	name := path.Clean(strings.TrimPrefix(urlPath, "/"))
	if name == "." || name == "" {
		return ".", true
	}
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Built asset names contain a content hash.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
