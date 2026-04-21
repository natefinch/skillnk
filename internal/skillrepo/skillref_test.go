package skillrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		owner string
		repo  string
		sub   string
	}{
		{"github.com/anthropics/skills/skills/skill-creator", true, "anthropics", "skills", "skills/skill-creator"},
		{"https://github.com/anthropics/skills/skills/skill-creator", true, "anthropics", "skills", "skills/skill-creator"},
		{"https://github.com/anthropics/skills.git/skills/skill-creator", true, "anthropics", "skills", "skills/skill-creator"},
		{"git@github.com:anthropics/skills/skills/skill-creator", true, "anthropics", "skills", "skills/skill-creator"},
		{"ssh://git@github.com/anthropics/skills/skills/skill-creator", true, "anthropics", "skills", "skills/skill-creator"},
		{"github.com:anthropics/skills", true, "anthropics", "skills", ""},
		{"github.com/owner/repo", true, "owner", "repo", ""},
		{"github.com/owner/repo/", true, "owner", "repo", ""},
		{"github.com/owner/repo.git", true, "owner", "repo", ""},
		{"https://gitlab.com/owner/repo", false, "", "", ""},
		{"", false, "", "", ""},
		{"github.com/owner", false, "", "", ""},
		{"github.com//repo", false, "", "", ""},
		// subpath escapes are neutralized
		{"github.com/owner/repo/../foo", true, "owner", "repo", ""},
	}
	for _, c := range cases {
		got, ok := ParseGitHubURL(c.in)
		if ok != c.ok {
			t.Errorf("ParseGitHubURL(%q) ok=%v want %v", c.in, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Owner != c.owner || got.Repo != c.repo || got.Subpath != c.sub {
			t.Errorf("ParseGitHubURL(%q) = %+v want {%s %s %s}", c.in, got, c.owner, c.repo, c.sub)
		}
	}
}

func TestGitHubURLDerivations(t *testing.T) {
	g, ok := ParseGitHubURL("github.com/anthropics/skills/skills/skill-creator")
	if !ok {
		t.Fatal("parse failed")
	}
	if got := g.CloneURL(); got != "https://github.com/anthropics/skills.git" {
		t.Errorf("CloneURL = %q", got)
	}
	if got := g.RepoName(); got != "anthropics/skills" {
		t.Errorf("RepoName = %q", got)
	}
	if got := g.DisplayPath(); got != "anthropics/skills/skills/skill-creator" {
		t.Errorf("DisplayPath = %q", got)
	}
	if got := g.DefaultSkillName(); got != "skill-creator" {
		t.Errorf("DefaultSkillName = %q", got)
	}

	// no subpath: default name = repo name
	g2, _ := ParseGitHubURL("github.com/anthropics/skills")
	if got := g2.DefaultSkillName(); got != "skills" {
		t.Errorf("DefaultSkillName (no sub) = %q", got)
	}
	if got := g2.DisplayPath(); got != "anthropics/skills" {
		t.Errorf("DisplayPath (no sub) = %q", got)
	}
}

func TestReadSkillRefsAllFormats(t *testing.T) {
	cases := []struct {
		fname, body string
	}{
		{"skillnk.yaml", `
skills:
  - url: github.com/anthropics/skills/skills/skill-creator
  - url: https://github.com/me/r.git
    name: custom
    version: v1
`},
		{"skillnk.json", `{"skills":[
  {"url":"github.com/anthropics/skills/skills/skill-creator"},
  {"url":"https://github.com/me/r.git","name":"custom","version":"v1"}
]}`},
		{"skillnk.toml", `
[[skills]]
url = "github.com/anthropics/skills/skills/skill-creator"

[[skills]]
url = "https://github.com/me/r.git"
name = "custom"
version = "v1"
`},
	}
	for _, c := range cases {
		t.Run(c.fname, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, c.fname, c.body)
			got, err := ReadSkillRefs(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("got %+v", got)
			}
			if got[0].Name != "skill-creator" || got[0].URL != "github.com/anthropics/skills/skills/skill-creator" {
				t.Errorf("got[0] = %+v", got[0])
			}
			if got[1].Name != "custom" || got[1].Version != "v1" {
				t.Errorf("got[1] = %+v", got[1])
			}
		})
	}
}

func TestReadSkillRefsRejectsNonGitHub(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skillnk.yaml", "skills:\n  - url: https://gitlab.com/a/b\n")
	_, err := ReadSkillRefs(dir)
	if err == nil || !strings.Contains(err.Error(), "only github.com") {
		t.Errorf("want github-only error, got %v", err)
	}
}

func TestReadSkillRefsMissingURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skillnk.yaml", "skills:\n  - name: x\n")
	_, err := ReadSkillRefs(dir)
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("want url required, got %v", err)
	}
}

func TestReadSkillRefsDuplicateName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skillnk.yaml", `
skills:
  - url: github.com/a/b/foo
  - url: github.com/c/d/foo
`)
	_, err := ReadSkillRefs(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("want duplicate error, got %v", err)
	}
}

func TestLibraryResolvesSkillRefs(t *testing.T) {
	home := t.TempDir()
	seedPrimary(t, home, []string{"p"}, `
skills:
  - url: github.com/anthropics/skills/skills/skill-creator
`, "skillnk.yaml")
	cloneDir := filepath.Join(home, "anthropics", "skills")
	g := &libFakeGit{
		seedAfter: map[string][]string{
			cloneDir: nil,
		},
	}
	// Need to pre-seed the subpath inside the clone after fake clone.
	// libFakeGit creates target/.git + any names in seedAfter as top-level
	// dirs. We'll add the nested subpath manually AFTER EnsureCloned.
	lib, err := NewLibrary(home, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Skills) != 1 {
		t.Fatalf("skills = %+v", lib.Skills)
	}
	s := lib.Skills[0]
	if s.Name != "skill-creator" {
		t.Errorf("name = %q", s.Name)
	}
	if s.CloneURL != "https://github.com/anthropics/skills.git" {
		t.Errorf("cloneURL = %q", s.CloneURL)
	}
	if s.Subpath != "skills/skill-creator" {
		t.Errorf("subpath = %q", s.Subpath)
	}
	if s.Repo.Dir != cloneDir {
		t.Errorf("clone dir = %q want %q", s.Repo.Dir, cloneDir)
	}

	if err := lib.EnsureCloned(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		t.Errorf("repo should have been cloned: %v", err)
	}

	// ListAll should skip the skill until its subpath exists on disk.
	skills, err := lib.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		if sk.Name == "skill-creator" {
			t.Error("skill-creator should not appear yet; subpath missing")
		}
	}
	// Create the subpath and list again.
	subDir := filepath.Join(cloneDir, "skills", "skill-creator")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skills, err = lib.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	var found *Skill
	for i := range skills {
		if skills[i].Name == "skill-creator" {
			found = &skills[i]
		}
	}
	if found == nil {
		t.Fatal("skill-creator not in list")
	}
	if found.Path != subDir {
		t.Errorf("path = %q want %q", found.Path, subDir)
	}
	if found.Source != "anthropics/skills/skills/skill-creator" {
		t.Errorf("source = %q", found.Source)
	}
}

func TestLibrarySharedCloneBetweenImportAndSkill(t *testing.T) {
	home := t.TempDir()
	seedPrimary(t, home, []string{"p"}, `
imports:
  - name: anthropics/skills
    url: https://github.com/anthropics/skills.git
skills:
  - url: github.com/anthropics/skills/skills/skill-creator
`, "skillnk.yaml")

	cloneDir := filepath.Join(home, "anthropics", "skills")
	g := &libFakeGit{seedAfter: map[string][]string{cloneDir: {"foo"}}}
	lib, err := NewLibrary(home, g)
	if err != nil {
		t.Fatalf("new library: %v", err)
	}
	if err := lib.EnsureCloned(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Only one clone should have happened.
	clones := 0
	for _, c := range g.allCalls {
		if len(c) >= 2 && c[1] == "clone" {
			clones++
		}
	}
	if clones != 1 {
		t.Errorf("expected 1 clone, got %d (calls: %v)", clones, g.allCalls)
	}
}

func TestLibraryVersionConflictBetweenImportAndSkill(t *testing.T) {
	home := t.TempDir()
	seedPrimary(t, home, []string{"p"}, `
imports:
  - name: anthropics/skills
    url: https://github.com/anthropics/skills.git
    version: v1
skills:
  - url: github.com/anthropics/skills/skills/skill-creator
    version: v2
`, "skillnk.yaml")
	_, err := NewLibrary(home, &libFakeGit{})
	if err == nil || !strings.Contains(err.Error(), "conflicting versions") {
		t.Errorf("want version conflict, got %v", err)
	}
}

func TestLibrarySkillRefPinnedCheckout(t *testing.T) {
	home := t.TempDir()
	seedPrimary(t, home, []string{"p"}, `
skills:
  - url: github.com/anthropics/skills/skills/skill-creator
    version: v1.2.3
`, "skillnk.yaml")
	g := &libFakeGit{}
	lib, err := NewLibrary(home, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.EnsureCloned(context.Background()); err != nil {
		t.Fatal(err)
	}
	sawCheckout := false
	for _, c := range g.allCalls {
		if len(c) >= 3 && c[1] == "checkout" && c[2] == "v1.2.3" {
			sawCheckout = true
		}
	}
	if !sawCheckout {
		t.Errorf("expected checkout v1.2.3, calls: %v", g.allCalls)
	}
}
