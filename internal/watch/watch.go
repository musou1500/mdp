// Package watch signals that files under a document root changed.
package watch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"mdp/internal/tree"
)

// quiet is how long changes are waited out before signalling. Editors emit
// several filesystem operations for a single save.
const quiet = 100 * time.Millisecond

// Watch signals on the returned channel once per quiet period in which files
// under root changed, and closes it when ctx is cancelled. What changed is not
// reported: consumers reread the document root, which is the only thing that
// can answer what its current state is.
//
// Hidden files and the directories skipped by tree.SkipDir are ignored, so what
// is watched matches what tree.Build lists.
func Watch(ctx context.Context, root string) (<-chan struct{}, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addRecursive(w, root); err != nil {
		w.Close()
		return nil, err
	}
	out := make(chan struct{}, 1)
	go run(ctx, w, root, out)
	return out, nil
}

func run(ctx context.Context, w *fsnotify.Watcher, root string, out chan<- struct{}) {
	defer close(out)
	defer w.Close()

	// Go 1.23 timers never deliver a stale value after Stop or Reset, so the
	// timer needs no draining. Stop is how a stopped timer is made: the quiet
	// period only starts once something changes.
	timer := time.NewTimer(quiet)
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case e, ok := <-w.Events:
			if !ok {
				return
			}
			if ignored(e.Name, root) {
				continue
			}
			// A new directory needs its own watch, and may already contain
			// files by the time we get here.
			if e.Has(fsnotify.Create) && isDir(e.Name) {
				addRecursive(w, e.Name)
			}
			timer.Reset(quiet)

		case <-timer.C:
			select {
			case out <- struct{}{}:
			default:
				// A signal is already pending and means the same thing.
			}

		case _, ok := <-w.Errors:
			// Watch errors are not fatal: the affected path simply stops
			// reporting changes.
			if !ok {
				return
			}
		}
	}
}

// ignored reports whether a path is outside root or one we do not watch.
func ignored(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return true
	}
	for part := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		// Skipped directories, editor swap files and other dotfiles.
		if strings.HasPrefix(part, ".") || tree.SkipDir(part) {
			return true
		}
	}
	return false
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func addRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// The tree can change under us while we walk it.
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != dir && tree.SkipDir(d.Name()) {
			return filepath.SkipDir
		}
		w.Add(path)
		return nil
	})
}
