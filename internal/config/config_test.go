package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(filepath.Join(dir, "nope.yaml"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")
	in := Config{SkillsRepo: "git@example.com:me/skills.git", CheckoutDir: "/tmp/co"}
	if err := Save(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestSaveValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := Save(path, Config{}); err == nil {
		t.Error("empty config should fail to save")
	}
	if err := Save(path, Config{SkillsRepo: "x"}); err == nil {
		t.Error("missing checkout should fail")
	}
}

func TestLoadMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("::: not yaml :::\n\t- ["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatal("parse error should not be ErrNotFound")
	}
}

func TestSaveAtomicOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := Save(path, Config{SkillsRepo: "a", CheckoutDir: "/a"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Config{SkillsRepo: "b", CheckoutDir: "/b"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SkillsRepo != "b" || got.CheckoutDir != "/b" {
		t.Errorf("overwrite failed: %+v", got)
	}
}
