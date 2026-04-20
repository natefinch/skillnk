package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestByName(t *testing.T) {
	c, ok := ByName("claude")
	if !ok || c.Name != "claude" {
		t.Fatalf("ByName(claude) = %+v ok=%v", c, ok)
	}
	if _, ok := ByName("bogus"); ok {
		t.Error("bogus should not be found")
	}
}

func TestNames(t *testing.T) {
	got := Names()
	if len(got) != len(Registry) {
		t.Fatalf("len = %d want %d", len(got), len(Registry))
	}
	for i, c := range Registry {
		if got[i] != c.Name {
			t.Errorf("names[%d] = %q want %q", i, got[i], c.Name)
		}
	}
}

func TestTargetFor(t *testing.T) {
	c, _ := ByName("copilot")
	got := c.TargetFor("/proj", "explain")
	want := filepath.Join("/proj", ".github", "skills", "explain")
	if got != want {
		t.Errorf("TargetFor = %q want %q", got, want)
	}
}

func mkDirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectNone(t *testing.T) {
	got, err := Detect(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want 0, got %+v", got)
	}
}

func TestDetectOne(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, ".claude")
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "claude" {
		t.Errorf("got %+v", got)
	}
}

func TestDetectMany(t *testing.T) {
	root := t.TempDir()
	mkDirs(t, root, ".cursor", ".github", ".claude")
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	want := []string{"claude", "copilot", "cursor"}
	for i, c := range got {
		if c.Name != want[i] {
			t.Errorf("got[%d] = %q want %q", i, c.Name, want[i])
		}
	}
}

func TestDetectIgnoresFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".claude"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("file marker should be ignored, got %+v", got)
	}
}
