package skillrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Library is the full set of skill sources available to skillnk: the user's
// primary skills checkout plus any imports and individual skill references
// declared in its config file.
type Library struct {
	Primary Repo
	Imports []Source
	Skills  []SkillEntry
}

// Source is one imported repo in a Library with the name it should be
// referred to by. Name is "" for the primary repo, and the import name
// otherwise. Version is the pinned git ref, "" for unpinned.
type Source struct {
	Name    string
	Version string
	URL     string
	Repo    Repo
}

// SkillEntry is one individual skill pinned by URL. The Repo field points
// to the shared checkout of the source github repo; Subpath (possibly "")
// identifies the skill directory within that checkout.
type SkillEntry struct {
	Name     string
	Version  string
	CloneURL string // canonical https clone URL
	Subpath  string
	Display  string // "owner/repo[/subpath]" for messages
	Repo     Repo
}

// SkillPath is the absolute path to the skill directory on disk (once the
// underlying repo has been cloned).
func (s SkillEntry) SkillPath() string {
	if s.Subpath == "" {
		return s.Repo.Dir
	}
	return filepath.Join(s.Repo.Dir, s.Subpath)
}

// NewLibrary constructs a Library for the given skillnk home dir. The
// primary checkout lives at <home>/repo; imports live at <home>/<name>;
// skill-referenced repos live at <home>/<owner>/<repo>. Imports and skill
// references are read from the primary repo's skillnk config file. Configs
// inside imported repos are ignored.
func NewLibrary(home string, git GitRunner) (Library, error) {
	primaryDir := filepath.Join(home, "repo")
	primary := New(primaryDir, git)
	lib := Library{Primary: primary}
	if !primary.Exists() {
		return lib, nil
	}

	imports, err := ReadImports(primaryDir)
	if err != nil {
		return lib, err
	}
	skillRefs, err := ReadSkillRefs(primaryDir)
	if err != nil {
		return lib, err
	}

	// Track version pins per clone dir so that multiple references to the
	// same underlying repo can't disagree on which ref to check out.
	dirVersion := map[string]string{}
	dirSource := map[string]string{}
	recordVersion := func(dir, version, source string) error {
		if prev, ok := dirVersion[dir]; ok {
			if prev != version {
				return fmt.Errorf("skillrepo: repo %s pinned to conflicting versions: %q (%s) vs %q (%s)",
					dir, prev, dirSource[dir], version, source)
			}
			return nil
		}
		dirVersion[dir] = version
		dirSource[dir] = source
		return nil
	}

	for _, imp := range imports {
		dir := filepath.Join(home, imp.Name)
		if err := recordVersion(dir, imp.Version, "import "+imp.Name); err != nil {
			return lib, err
		}
		lib.Imports = append(lib.Imports, Source{
			Name:    imp.Name,
			Version: imp.Version,
			URL:     imp.URL,
			Repo:    New(dir, git),
		})
	}

	for _, sr := range skillRefs {
		g, _ := ParseGitHubURL(sr.URL) // already validated in ReadSkillRefs
		cloneDir := filepath.Join(home, g.Owner, g.Repo)
		if err := recordVersion(cloneDir, sr.Version, "skill "+sr.Name); err != nil {
			return lib, err
		}
		lib.Skills = append(lib.Skills, SkillEntry{
			Name:     sr.Name,
			Version:  sr.Version,
			CloneURL: g.CloneURL(),
			Subpath:  g.Subpath,
			Display:  g.DisplayPath(),
			Repo:     New(cloneDir, git),
		})
	}
	return lib, nil
}

// managedRepo captures everything needed to clone or update one underlying
// git checkout.
type managedRepo struct {
	Dir     string
	URL     string
	Version string
	Repo    Repo
	Label   string // human-readable for messages
}

// managed returns every underlying repo that skillnk manages (imports +
// skill-reference clones), deduplicated by clone directory. The primary
// repo is not included.
func (l Library) managed() []managedRepo {
	seen := map[string]bool{}
	var out []managedRepo
	add := func(m managedRepo) {
		if seen[m.Dir] {
			return
		}
		seen[m.Dir] = true
		out = append(out, m)
	}
	for _, imp := range l.Imports {
		add(managedRepo{
			Dir: imp.Repo.Dir, URL: imp.URL, Version: imp.Version,
			Repo: imp.Repo, Label: "import " + imp.Name,
		})
	}
	for _, s := range l.Skills {
		add(managedRepo{
			Dir: s.Repo.Dir, URL: s.CloneURL, Version: s.Version,
			Repo: s.Repo, Label: "skill " + s.Display,
		})
	}
	return out
}

// EnsureCloned clones any managed repos (imports and skill-ref repos) that
// are not yet present on disk. For repos with a pinned version, the ref is
// checked out after cloning.
func (l Library) EnsureCloned(ctx context.Context) error {
	for _, m := range l.managed() {
		if m.Repo.Exists() {
			continue
		}
		if err := m.Repo.Clone(ctx, m.URL); err != nil {
			return fmt.Errorf("clone %s: %w", m.Label, err)
		}
		if m.Version != "" {
			if err := m.Repo.Checkout(ctx, m.Version); err != nil {
				return fmt.Errorf("checkout %s @ %s: %w", m.Label, m.Version, err)
			}
		}
	}
	return nil
}

// PullAll updates the primary repo and every managed repo. The primary is
// always pulled with `git pull --ff-only`. For pinned repos skillnk runs
// `git fetch --tags` followed by `git checkout <version>`; unpinned repos
// are pulled with `git pull --ff-only`.
//
// Errors from individual repos are collected and returned after all updates
// are attempted so one broken repo doesn't block the rest.
func (l Library) PullAll(ctx context.Context) error {
	var errs []error
	if err := l.Primary.Pull(ctx); err != nil {
		errs = append(errs, fmt.Errorf("primary: %w", err))
	}
	for _, m := range l.managed() {
		if !m.Repo.Exists() {
			continue
		}
		var err error
		if m.Version != "" {
			if err = m.Repo.Fetch(ctx); err == nil {
				err = m.Repo.Checkout(ctx, m.Version)
			}
		} else {
			err = m.Repo.Pull(ctx)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.Label, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	var msg string
	for i, e := range errs {
		if i > 0 {
			msg += "; "
		}
		msg += e.Error()
	}
	return fmt.Errorf("pull errors: %s", msg)
}

// ListAll returns every skill available in the library. Skills are emitted
// in order: primary first, then imports (in config order, by skill name
// within each), then individual skill references (in config order). When
// the same skill name appears in more than one source, the first
// occurrence wins.
func (l Library) ListAll() ([]Skill, error) {
	seen := map[string]struct{}{}
	var out []Skill

	appendFrom := func(sourceName string, r Repo) error {
		if !r.Exists() {
			return nil
		}
		skills, err := r.List()
		if err != nil {
			return err
		}
		sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
		for _, s := range skills {
			if _, dup := seen[s.Name]; dup {
				continue
			}
			seen[s.Name] = struct{}{}
			s.Source = sourceName
			out = append(out, s)
		}
		return nil
	}

	if err := appendFrom("", l.Primary); err != nil {
		return nil, err
	}
	for _, imp := range l.Imports {
		if err := appendFrom(imp.Name, imp.Repo); err != nil {
			return nil, err
		}
	}
	for _, s := range l.Skills {
		if _, dup := seen[s.Name]; dup {
			continue
		}
		p := s.SkillPath()
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				// repo not yet cloned, or subpath missing in this version
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("skillrepo: skill %q path is not a directory: %s", s.Name, p)
		}
		seen[s.Name] = struct{}{}
		out = append(out, Skill{
			Name:   s.Name,
			Path:   p,
			Source: s.Display,
		})
	}
	return out, nil
}

// Find returns the first skill matching name across the library.
func (l Library) Find(name string) (Skill, bool, error) {
	skills, err := l.ListAll()
	if err != nil {
		return Skill{}, false, err
	}
	for _, s := range skills {
		if s.Name == name {
			return s, true, nil
		}
	}
	return Skill{}, false, nil
}
