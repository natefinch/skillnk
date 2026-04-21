# skillnk

A tiny CLI that links skills from your personal skills git repo into the AI
client (Claude, Copilot, Cursor, Codex) a project uses. Skills live in one
place, are version-controlled, and are shared across projects via symlinks.

## Install

```
go install github.com/natefinch/skillnk@latest
```

Requires `git` on your `PATH`.

## First run

On first run, skillnk asks for the URL of the git repo that holds your
skills and clones it into `~/.skillnk/repo`. Config is saved to
`~/.skillnk/config.yaml`.

```
$ skillnk init
```

A "skill" is any top-level directory in that repo (dotfiles and `.github`
are ignored).

## The `skillnk` config file

Your skills repo may include a `skillnk` config file at its root, which
declares additional skills sources via a single `imports:` list. Each
import is one git repo; by default every top-level directory is treated
as a skill, but you can narrow it down to a single directory or wildcard
subtree with `dir:`.

### Location and format

The file lives at the root of your skills repo and may be written in any of
four formats. If multiple exist, this precedence applies (first match wins):

1. `skillnk.yaml`
2. `skillnk.yml`
3. `skillnk.json`
4. `skillnk.toml`

### Schema

One top-level key: `imports`, a list of objects.

| field     | required | notes                                                                 |
|-----------|----------|-----------------------------------------------------------------------|
| `url`     | yes      | Any git URL `git clone` understands.                                  |
| `dir`     | no       | Which directory(ies) of the repo to treat as skills. Defaults to `*`. |
| `version` | no       | Pin to a specific git ref — tag, branch, or commit SHA.               |

`dir` accepts:

- omitted or `*` — every top-level directory of the repo is a skill.
- `some/path` (with optional trailing `/`) — a single skill directory at
  that path.
- `some/path/*` — every immediate subdirectory of `some/path` is a skill.

`..`, absolute paths, and wildcards anywhere other than the final segment
are rejected.

### URL handling

skillnk accepts the usual git URL forms — `https://`, `http://`, `ssh://`,
`git://`, scp-style `user@host:path`, and bare `host/path`. The URL is
passed through to `git clone` unchanged, so your existing SSH keys or
credential helpers keep working.

Both the on-disk clone and the install path under your project mirror the
URL structure. For example, with
`url: git@example.com:my-org/my-repo` and `dir: skills/do-the-thing`:

- Cloned to: `~/.skillnk/example.com/my-org/my-repo/`
- Installed to: `.github/skills/example.com/my-org/my-repo/skills/do-the-thing/`
  (and the equivalent under `.claude/skills/`, `.codex/skills/`, etc.)

This namespacing keeps skills from different sources from colliding on
disk, and makes it obvious where each one came from.

### Behavior

- If multiple imports point at the same repo, the clone is shared. An
  error is raised if they disagree on `version`.
- `skillnk update` runs `git pull --ff-only` on the primary checkout and
  on every unpinned source. Pinned sources get `git fetch --tags`
  followed by `git checkout <version>`.
- Imports are **not transitive**: a `skillnk` config inside an imported
  repo is ignored.
- Primary-repo skills (in your own personal skills repo) are still
  installed with flat names (`.github/skills/<skill>/`). The URL
  namespace only applies to imports.

### Examples

```yaml
# skillnk.yaml
imports:
  # Pull a single skill at a known path, pinned.
  - url: github.com/anthropics/skills
    dir: skills/skill-creator
    version: v0.3.0

  # Pull every skill under a subdirectory.
  - url: github.com/anthropics/skills
    dir: skills/*

  # Pull every top-level directory of a private repo.
  - url: git@example.com:my-org/my-repo
    version: v1.4.0

  # Whole repo, default branch.
  - url: https://github.com/charmbracelet/skills.git
```

```json
{
  "imports": [
    { "url": "github.com/anthropics/skills", "dir": "skills/skill-creator", "version": "v0.3.0" },
    { "url": "github.com/anthropics/skills", "dir": "skills/*" },
    { "url": "git@example.com:my-org/my-repo", "version": "v1.4.0" },
    { "url": "https://github.com/charmbracelet/skills.git" }
  ]
}
```

```toml
# skillnk.toml
[[imports]]
url     = "github.com/anthropics/skills"
dir     = "skills/skill-creator"
version = "v0.3.0"

[[imports]]
url = "github.com/anthropics/skills"
dir = "skills/*"

[[imports]]
url     = "git@example.com:my-org/my-repo"
version = "v1.4.0"

[[imports]]
url = "https://github.com/charmbracelet/skills.git"
```

## Commands

| command     | what it does                                                      |
|-------------|-------------------------------------------------------------------|
| `init`      | Prompt for the skills repo and clone it.                          |
| `install`   | Pick skills (multi-select) and symlink them into the project.     |
| `uninstall` | Remove previously-installed skill symlinks (sources untouched).   |
| `list`      | List available skills; mark which are installed in this project. |
| `status`    | Show installed skills and where they link to.                     |
| `update`    | `git pull --ff-only` in the primary checkout and every import.    |

Non-interactive use:

```
skillnk install --client=claude --skill=foo --skill=bar
skillnk uninstall --client=claude --skill=foo
```

## Client detection

skillnk looks for these marker directories in the project root and installs
into the matching skills dir:

| client  | marker     | install target          |
|---------|------------|-------------------------|
| claude  | `.claude`  | `.claude/skills/<name>` |
| copilot | `.github`  | `.github/skills/<name>` |
| cursor  | `.cursor`  | `.cursor/skills/<name>` |
| codex   | `.codex`   | `.codex/skills/<name>`  |

With zero matches, skillnk prompts. With multiple matches, it prompts with
the subset. `--client` overrides detection.

## Development

```
go test ./...
go build ./...
go vet ./...
```

Layout:

```
internal/
  paths/      resolve home, ~/.skillnk, project root (pure)
  config/     load/save ~/.skillnk/config.yaml (pure)
  client/     client registry + auto-detect (pure)
  skillrepo/  clone/pull/list skills via injected GitRunner (pure)
  installer/  symlink create/remove/status (pure)
  tui/        Bubble Tea models (no core logic)
  cli/        Cobra wiring; only layer that imports tui + core
```

Core packages have no UI/CLI knowledge and are fully unit-tested with
`t.TempDir()` and fakes — no network, no real git.
