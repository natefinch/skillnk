package skillrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGit records calls and can simulate a clone by creating .git.
type fakeGit struct {
	calls       [][]string
	cloneCreate bool
	err         error
}

func (f *fakeGit) Run(_ context.Context, dir string, args ...string) error {
	f.calls = append(f.calls, append([]string{dir}, args...))
	if f.err != nil {
		return f.err
	}
	if f.cloneCreate && len(args) >= 3 && args[0] == "clone" {
		target := filepath.Join(dir, args[2])
		if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func TestExistsFalse(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "none"), &fakeGit{})
	if r.Exists() {
		t.Error("should not exist")
	}
}

func TestExistsTrue(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(dir, &fakeGit{})
	if !r.Exists() {
		t.Error("should exist")
	}
}

func TestCloneEmptyURL(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "repo"), &fakeGit{})
	if err := r.Clone(context.Background(), ""); err == nil {
		t.Error("empty url should error")
	}
}

func TestCloneCallsGit(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	g := &fakeGit{cloneCreate: true}
	r := New(dir, g)
	if err := r.Clone(context.Background(), "git@x:y.git"); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("calls: %v", g.calls)
	}
	call := g.calls[0]
	if call[0] != parent {
		t.Errorf("dir = %q want %q", call[0], parent)
	}
	if call[1] != "clone" || call[2] != "git@x:y.git" || call[3] != "repo" {
		t.Errorf("args = %v", call[1:])
	}
	if !r.Exists() {
		t.Error("expected checkout to exist after clone")
	}
}

func TestCloneRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(dir, &fakeGit{})
	err := r.Clone(context.Background(), "git@x:y.git")
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("want non-empty error, got %v", err)
	}
}

func TestPullRequiresCheckout(t *testing.T) {
	r := New(t.TempDir(), &fakeGit{})
	if err := r.Pull(context.Background()); err == nil {
		t.Error("pull without .git should fail")
	}
}

func TestPullCallsGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	g := &fakeGit{}
	r := New(dir, g)
	if err := r.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 || g.calls[0][1] != "pull" {
		t.Errorf("unexpected calls: %v", g.calls)
	}
}

func TestPullPropagatesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(dir, &fakeGit{err: errors.New("nope")})
	if err := r.Pull(context.Background()); err == nil {
		t.Error("want error from git")
	}
}

func setupRepo(t *testing.T, names ...string) Repo {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return New(dir, &fakeGit{})
}

func TestListFiltersAndSorts(t *testing.T) {
	r := setupRepo(t, "zeta", "alpha", ".git", ".github", "beta")
	// also add a file at top level
	if err := os.WriteFile(filepath.Join(r.Dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, s := range got {
		if s.Name != want[i] {
			t.Errorf("got[%d] = %q want %q", i, s.Name, want[i])
		}
		if s.Path != filepath.Join(r.Dir, s.Name) {
			t.Errorf("path = %q", s.Path)
		}
	}
}

func TestListMissingDir(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "nope"), &fakeGit{})
	if _, err := r.List(); err == nil {
		t.Error("missing dir should error")
	}
}

func TestFind(t *testing.T) {
	r := setupRepo(t, "alpha", "beta")
	s, ok, err := r.Find("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || s.Name != "alpha" {
		t.Errorf("find alpha: %+v ok=%v", s, ok)
	}
	_, ok, err = r.Find("missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("missing should be false")
	}
}
