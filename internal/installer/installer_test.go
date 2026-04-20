package installer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mkSource(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestInstallCreates(t *testing.T) {
	src := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "a", "b", "skill")
	if err := Install(src, tgt); err != nil {
		t.Fatal(err)
	}
	dest, err := os.Readlink(tgt)
	if err != nil {
		t.Fatal(err)
	}
	if dest != src {
		t.Errorf("dest = %q want %q", dest, src)
	}
}

func TestInstallIdempotent(t *testing.T) {
	src := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := Install(src, tgt); err != nil {
		t.Fatal(err)
	}
	if err := Install(src, tgt); err != nil {
		t.Errorf("second install should be no-op, got %v", err)
	}
}

func TestInstallReplacesOtherSymlink(t *testing.T) {
	src := mkSource(t)
	other := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := os.Symlink(other, tgt); err != nil {
		t.Fatal(err)
	}
	if err := Install(src, tgt); err != nil {
		t.Fatal(err)
	}
	dest, err := os.Readlink(tgt)
	if err != nil {
		t.Fatal(err)
	}
	if dest != src {
		t.Errorf("dest = %q want %q", dest, src)
	}
}

func TestInstallRefusesRealDir(t *testing.T) {
	src := mkSource(t)
	tgtDir := t.TempDir()
	tgt := filepath.Join(tgtDir, "skill")
	if err := os.MkdirAll(tgt, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Install(src, tgt)
	if !errors.Is(err, ErrExists) {
		t.Errorf("want ErrExists, got %v", err)
	}
}

func TestInstallRefusesRealFile(t *testing.T) {
	src := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := os.WriteFile(tgt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Install(src, tgt)
	if !errors.Is(err, ErrExists) {
		t.Errorf("want ErrExists, got %v", err)
	}
}

func TestInstallMissingSource(t *testing.T) {
	err := Install(filepath.Join(t.TempDir(), "gone"), filepath.Join(t.TempDir(), "t"))
	if err == nil {
		t.Error("missing source should error")
	}
}

func TestRemoveSymlink(t *testing.T) {
	src := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := Install(src, tgt); err != nil {
		t.Fatal(err)
	}
	if err := Remove(tgt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(tgt); !os.IsNotExist(err) {
		t.Error("symlink should be gone")
	}
	// source untouched
	if _, err := os.Stat(src); err != nil {
		t.Error("source must be preserved")
	}
}

func TestRemoveMissingIsNoOp(t *testing.T) {
	if err := Remove(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("missing target should not error, got %v", err)
	}
}

func TestRemoveRefusesRealDir(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "real")
	if err := os.MkdirAll(tgt, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Remove(tgt)
	if !errors.Is(err, ErrNotSymlink) {
		t.Errorf("want ErrNotSymlink, got %v", err)
	}
	if _, err := os.Stat(tgt); err != nil {
		t.Error("dir should not be removed")
	}
}

func TestStatOne(t *testing.T) {
	src := mkSource(t)
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := Install(src, tgt); err != nil {
		t.Fatal(err)
	}
	s, err := StatOne(tgt)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsSymlink || s.IsDangling || s.LinkDest != src {
		t.Errorf("stat = %+v", s)
	}
}

func TestStatOneDangling(t *testing.T) {
	tgt := filepath.Join(t.TempDir(), "skill")
	if err := os.Symlink("/does/not/exist/anywhere", tgt); err != nil {
		t.Fatal(err)
	}
	s, err := StatOne(tgt)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsSymlink || !s.IsDangling {
		t.Errorf("want dangling symlink, got %+v", s)
	}
}

func TestStatOneMissing(t *testing.T) {
	s, err := StatOne(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if s.IsSymlink {
		t.Errorf("missing should not be symlink, got %+v", s)
	}
}

func TestListInstalled(t *testing.T) {
	src1 := mkSource(t)
	src2 := mkSource(t)
	dir := t.TempDir()
	if err := Install(src1, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if err := Install(src2, filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	// a real file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a real dir should be ignored
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ListInstalled(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("order wrong: %+v", got)
	}
}

func TestListInstalledMissingDir(t *testing.T) {
	got, err := ListInstalled(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}
