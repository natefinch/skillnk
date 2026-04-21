// Package installer creates, removes, and inspects skill-install symlinks
// in a project. It knows nothing about clients or skill repos; callers pass
// in absolute source and target paths.
package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Status describes the state of an installed skill.
type Status struct {
	Name       string // skill name (basename of Target)
	RelPath    string // path of Target relative to the skills root that was scanned
	Target     string // absolute path of the symlink in the project
	LinkDest   string // what the symlink points to (readlink), or "" if not a symlink
	IsSymlink  bool   // true if Target is a symlink
	IsDangling bool   // true if Target is a symlink whose destination does not exist
}

// ErrNotSymlink is returned by Remove when the target exists but is not a
// symlink; we never delete real user files or directories.
var ErrNotSymlink = errors.New("installer: target is not a symlink")

// ErrExists is returned by Install when the target exists and is not a
// symlink we'd overwrite.
var ErrExists = errors.New("installer: target already exists and is not a managed symlink")

// Install creates a symlink at target pointing to source. It creates the
// target's parent directory as needed.
//
// If target already exists:
//   - If it is a symlink pointing to source, Install is a no-op (idempotent).
//   - If it is a symlink pointing elsewhere, it is replaced.
//   - If it is a real file or directory, Install returns ErrExists.
//
// source must exist; otherwise Install returns an error.
func Install(source, target string) error {
	if source == "" || target == "" {
		return fmt.Errorf("installer: empty source or target")
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("installer: source: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("installer: mkdir parent: %w", err)
	}

	info, err := os.Lstat(target)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		dest, rerr := os.Readlink(target)
		if rerr == nil && dest == source {
			return nil // idempotent
		}
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("installer: remove old link: %w", err)
		}
	case err == nil:
		return fmt.Errorf("%w: %s", ErrExists, target)
	case !os.IsNotExist(err):
		return fmt.Errorf("installer: lstat: %w", err)
	}

	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("installer: symlink: %w", err)
	}
	return nil
}

// Remove deletes the symlink at target. If target does not exist, Remove is
// a no-op. If target exists but is not a symlink, Remove returns
// ErrNotSymlink (we never delete real files).
func Remove(target string) error {
	if target == "" {
		return fmt.Errorf("installer: empty target")
	}
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("installer: lstat: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%w: %s", ErrNotSymlink, target)
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("installer: remove: %w", err)
	}
	return nil
}

// StatOne returns the Status of a single target path.
func StatOne(target string) (Status, error) {
	s := Status{
		Name:   filepath.Base(target),
		Target: target,
	}
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("installer: lstat: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return s, nil
	}
	s.IsSymlink = true
	dest, err := os.Readlink(target)
	if err != nil {
		return s, fmt.Errorf("installer: readlink: %w", err)
	}
	s.LinkDest = dest
	resolved := dest
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			s.IsDangling = true
		} else {
			return s, fmt.Errorf("installer: stat dest: %w", err)
		}
	}
	return s, nil
}

// ListInstalled returns the Status of every symlink anywhere under dir,
// walking recursively. Non-symlink entries are skipped, but directories
// under them are still descended. If dir does not exist, the result is
// empty with no error.
func ListInstalled(dir string) ([]Status, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("installer: lstat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	var out []Status
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if p == dir {
			return nil
		}
		i, err := d.Info()
		if err != nil {
			return err
		}
		if i.Mode()&os.ModeSymlink != 0 {
			s, err := StatOne(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			if err == nil {
				s.RelPath = rel
			}
			out = append(out, s)
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("installer: walk %s: %w", dir, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}
