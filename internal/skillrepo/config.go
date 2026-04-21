package skillrepo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Import is one external skills repo declared in a skillnk config file.
type Import struct {
	Name string `yaml:"name" json:"name" toml:"name"`
	URL  string `yaml:"url"  json:"url"  toml:"url"`
	// Version is an optional git ref (tag, branch, or commit SHA) to pin
	// the import to. If empty, the import tracks the remote default branch
	// and is updated via git pull --ff-only. When set, skillnk checks out
	// this ref after cloning and re-checks it out (after git fetch) on
	// every update.
	Version string `yaml:"version" json:"version" toml:"version"`
}

// SkillRef references one individual skill inside a git repo, typically at
// a subdirectory of that repo. Only github.com URLs are currently
// supported. Path segments after "<owner>/<repo>" are treated as the
// subpath of the skill inside the repo.
type SkillRef struct {
	URL     string `yaml:"url"     json:"url"     toml:"url"`
	Name    string `yaml:"name"    json:"name"    toml:"name"`
	Version string `yaml:"version" json:"version" toml:"version"`
}

type repoConfigFile struct {
	Imports []Import   `yaml:"imports" json:"imports" toml:"imports"`
	Skills  []SkillRef `yaml:"skills"  json:"skills"  toml:"skills"`
}

// configFileNames is the ordered list of accepted skillnk config filenames in
// the root of a skills repo. If more than one exists, the first match wins.
var configFileNames = []string{
	"skillnk.yaml",
	"skillnk.yml",
	"skillnk.json",
	"skillnk.toml",
}

// reservedImportNames are names an import may not take because they would
// collide with skillnk's own files in ~/.skillnk.
var reservedImportNames = map[string]struct{}{
	"repo":        {},
	"config.yaml": {},
}

// readConfigRaw loads the first matching config file and decodes it into
// repoConfigFile. Returns ("", nil) if no file is present.
func readConfigRaw(repoDir string) (repoConfigFile, string, error) {
	var cfg repoConfigFile
	for _, name := range configFileNames {
		p := filepath.Join(repoDir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cfg, "", fmt.Errorf("skillrepo: read %s: %w", p, err)
		}
		switch filepath.Ext(name) {
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return cfg, p, fmt.Errorf("skillrepo: parse %s: %w", p, err)
			}
		case ".json":
			if err := json.Unmarshal(b, &cfg); err != nil {
				return cfg, p, fmt.Errorf("skillrepo: parse %s: %w", p, err)
			}
		case ".toml":
			if err := toml.Unmarshal(b, &cfg); err != nil {
				return cfg, p, fmt.Errorf("skillrepo: parse %s: %w", p, err)
			}
		}
		return cfg, p, nil
	}
	return cfg, "", nil
}

// ReadImports reads the skillnk config from the root of repoDir and returns
// the normalized import list. If no config file is present, it returns (nil,
// nil). Imports with a missing Name are defaulted from their URL.
func ReadImports(repoDir string) ([]Import, error) {
	cfg, found, err := readConfigRaw(repoDir)
	if err != nil {
		return nil, err
	}
	if found == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]Import, 0, len(cfg.Imports))
	for i, imp := range cfg.Imports {
		if strings.TrimSpace(imp.URL) == "" {
			return nil, fmt.Errorf("skillrepo: %s: imports[%d]: url is required", found, i)
		}
		if imp.Name == "" {
			imp.Name = DefaultImportName(imp.URL)
		}
		if err := validateImportName(imp.Name); err != nil {
			return nil, fmt.Errorf("skillrepo: %s: imports[%d]: %w", found, i, err)
		}
		if _, dup := seen[imp.Name]; dup {
			return nil, fmt.Errorf("skillrepo: %s: imports[%d]: duplicate name %q", found, i, imp.Name)
		}
		seen[imp.Name] = struct{}{}
		out = append(out, imp)
	}
	return out, nil
}

// ReadSkillRefs reads the skillnk config and returns the normalized list of
// individual skill references. Each entry's URL is required and must be a
// supported github.com URL. Returns (nil, nil) if no config file exists.
func ReadSkillRefs(repoDir string) ([]SkillRef, error) {
	cfg, found, err := readConfigRaw(repoDir)
	if err != nil {
		return nil, err
	}
	if found == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]SkillRef, 0, len(cfg.Skills))
	for i, sr := range cfg.Skills {
		if strings.TrimSpace(sr.URL) == "" {
			return nil, fmt.Errorf("skillrepo: %s: skills[%d]: url is required", found, i)
		}
		g, ok := ParseGitHubURL(sr.URL)
		if !ok {
			return nil, fmt.Errorf("skillrepo: %s: skills[%d]: unsupported URL %q (only github.com URLs are supported)", found, i, sr.URL)
		}
		if sr.Name == "" {
			sr.Name = g.DefaultSkillName()
		}
		if err := validateImportName(sr.Name); err != nil {
			return nil, fmt.Errorf("skillrepo: %s: skills[%d]: %w", found, i, err)
		}
		if _, dup := seen[sr.Name]; dup {
			return nil, fmt.Errorf("skillrepo: %s: skills[%d]: duplicate name %q", found, i, sr.Name)
		}
		seen[sr.Name] = struct{}{}
		out = append(out, sr)
	}
	return out, nil
}

// GitHubURL is a parsed GitHub-style URL, split into its repo coordinates
// and any subpath after <owner>/<repo>.
type GitHubURL struct {
	Owner   string
	Repo    string // without trailing ".git"
	Subpath string // "" if the URL refers to the repo root
}

// ParseGitHubURL parses a github-style URL into its components. It accepts
// the common forms:
//
//	github.com/OWNER/REPO[/SUB/PATH]
//	github.com:OWNER/REPO[/SUB/PATH]
//	https://github.com/OWNER/REPO[/SUB/PATH]
//	http://github.com/OWNER/REPO[/SUB/PATH]
//	ssh://git@github.com/OWNER/REPO[/SUB/PATH]
//	git@github.com:OWNER/REPO[/SUB/PATH]
//
// A trailing ".git" on REPO is tolerated and stripped. Returns (_, false)
// if the input is not a recognizable github.com URL.
func ParseGitHubURL(raw string) (GitHubURL, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	prefixes := []string{
		"https://github.com/",
		"http://github.com/",
		"ssh://git@github.com/",
		"git@github.com:",
		"github.com/",
		"github.com:",
	}
	matched := false
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			matched = true
			break
		}
	}
	if !matched {
		return GitHubURL{}, false
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return GitHubURL{}, false
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return GitHubURL{}, false
	}
	var sub string
	if len(parts) > 2 {
		rest := strings.Join(parts[2:], "/")
		sub = path.Clean(rest)
		// Reject anything that would escape the repo root.
		if sub == "." || sub == "/" || strings.HasPrefix(sub, "../") || sub == ".." {
			sub = ""
		}
	}
	return GitHubURL{Owner: owner, Repo: repo, Subpath: sub}, true
}

// CloneURL returns a canonical https clone URL (always ending in .git) for
// the repo.
func (g GitHubURL) CloneURL() string {
	return "https://github.com/" + g.Owner + "/" + g.Repo + ".git"
}

// RepoName returns "owner/repo" — the canonical two-segment repo path.
func (g GitHubURL) RepoName() string {
	return g.Owner + "/" + g.Repo
}

// DisplayPath returns the URL's canonical short form, including any
// subpath: e.g. "anthropics/skills/skills/skill-creator".
func (g GitHubURL) DisplayPath() string {
	if g.Subpath == "" {
		return g.RepoName()
	}
	return g.RepoName() + "/" + g.Subpath
}

// DefaultSkillName returns the default skill name for this URL: the last
// segment of the subpath, or the repo name if no subpath is present.
func (g GitHubURL) DefaultSkillName() string {
	if g.Subpath != "" {
		return path.Base(g.Subpath)
	}
	return g.Repo
}

// DefaultImportName derives an import name from a git URL. It strips common
// github.com prefixes and any trailing ".git". If no github.com prefix is
// present, it falls back to the URL's final path segment.
func DefaultImportName(url string) string {
	s := strings.TrimSpace(url)
	stripped := false
	for _, p := range []string{
		"https://github.com/",
		"http://github.com/",
		"ssh://git@github.com/",
		"git@github.com:",
		"github.com/",
		"github.com:",
	} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			stripped = true
			break
		}
	}
	s = strings.TrimSuffix(s, ".git")
	s = strings.Trim(s, "/")
	if !stripped {
		s = path.Base(s)
		s = strings.TrimSuffix(s, ".git")
	}
	if s == "" || s == "." || s == "/" {
		return "import"
	}
	return s
}

func validateImportName(name string) error {
	if name == "" {
		return errors.New("name is empty")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("name %q must not start with '.'", name)
	}
	if strings.ContainsAny(name, `\`+"\x00") || strings.Contains(name, "..") {
		return fmt.Errorf("name %q contains invalid characters", name)
	}
	// slashes are allowed (e.g. "owner/repo") but reject absolute / parent refs
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("name %q must not be absolute", name)
	}
	if _, bad := reservedImportNames[name]; bad {
		return fmt.Errorf("name %q is reserved", name)
	}
	return nil
}
