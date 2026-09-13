package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// signal waits for one change signal.
func signal(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case _, ok := <-changes:
		if !ok {
			t.Fatal("channel closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a change")
	}
}

// idle asserts no signal arrives for a while.
func idle(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
		t.Fatal("unexpected change")
	case <-time.After(300 * time.Millisecond):
	}
}

func start(t *testing.T, root string) <-chan struct{} {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	changes, err := Watch(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func TestWatchCreateWriteRemove(t *testing.T) {
	root := t.TempDir()
	changes := start(t, root)

	p := write(t, root, "a.md", "hello")
	signal(t, changes)

	write(t, root, "a.md", "hello again")
	signal(t, changes)

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	signal(t, changes)
	idle(t, changes)
}

func TestWatchNewSubdirectory(t *testing.T) {
	root := t.TempDir()
	changes := start(t, root)

	if err := os.MkdirAll(filepath.Join(root, "docs", "guide"), 0o755); err != nil {
		t.Fatal(err)
	}
	signal(t, changes)
	idle(t, changes)

	// The nested directory got a watch of its own on the way.
	write(t, root, "docs/guide/a.md", "hi")
	signal(t, changes)
}

func TestWatchCoalescesOneSave(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.md", "one")
	changes := start(t, root)

	// A save is typically a write plus a chmod or two.
	p := filepath.Join(root, "a.md")
	for range 3 {
		if err := os.WriteFile(p, []byte("two"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	signal(t, changes)
	idle(t, changes)
}

// Several files changing at once is one signal, not one per file: consumers
// reread the whole document root anyway.
func TestWatchCoalescesAcrossPaths(t *testing.T) {
	root := t.TempDir()
	changes := start(t, root)

	write(t, root, "a.md", "one")
	write(t, root, "docs/b.md", "two")
	write(t, root, "docs/c.md", "three")

	signal(t, changes)
	idle(t, changes)
}

func TestWatchIgnoresHiddenAndSkippedDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	changes := start(t, root)

	write(t, root, ".git/HEAD", "ref")
	write(t, root, ".a.md.swp", "swap")
	write(t, root, "node_modules/pkg/readme.md", "x")
	idle(t, changes)

	write(t, root, "real.md", "x")
	signal(t, changes)
}

func TestWatchClosesOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	changes, err := Watch(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-changes:
		if ok {
			t.Error("expected the channel to be closed")
		}
	case <-time.After(3 * time.Second):
		t.Error("channel was not closed")
	}
}
