package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
)

type Location struct {
	Path           string `json:"path"`
	RepositoryRoot string `json:"repository_root"`
}

type Loaded struct {
	Location
	Config Config
}

type Loader struct {
	Executor process.Executor
	Env      []string
}

// Load searches upward within the current Git worktree. An explicit path selects
// its own implementation repository, even when called from a different checkout.
func (l Loader) Load(ctx context.Context, dir, override string) (Loaded, error) {
	location, start, err := l.locate(ctx, dir, override)
	if err != nil {
		return Loaded{}, err
	}
	if override == "" {
		for current := start; ; current = filepath.Dir(current) {
			candidate := filepath.Join(current, Filename)
			_, err := os.Lstat(candidate)
			if err == nil {
				location.Path = candidate
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return Loaded{}, fileError("config_read_failed", "read", candidate, err)
			}
			if current == location.RepositoryRoot || current == filepath.Dir(current) {
				return Loaded{}, &Error{Code: "config_not_found", Message: "no .planctl.yaml found in this worktree; run planctl init"}
			}
		}
	}
	data, err := os.ReadFile(location.Path)
	if err != nil {
		code := "config_read_failed"
		if errors.Is(err, os.ErrNotExist) {
			code = "config_not_found"
		}
		return Loaded{}, fileError(code, "read", location.Path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return Loaded{}, err
	}
	return Loaded{Location: location, Config: cfg}, nil
}

// Init never overwrites a file, directory, or symlink. O_EXCL also protects
// against another invocation creating the same file between discovery and write.
func (l Loader) Init(ctx context.Context, dir, override string) (Location, error) {
	location, _, err := l.locate(ctx, dir, override)
	if err != nil {
		return Location{}, err
	}
	if err := ctx.Err(); err != nil {
		return Location{}, err
	}
	file, err := os.OpenFile(location.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return Location{}, &Error{Code: "config_exists", Message: fmt.Sprintf("configuration already exists at %s", location.Path), Cause: err}
		}
		return Location{}, fileError("config_write_failed", "create", location.Path, err)
	}
	_, writeErr := file.WriteString(minimal)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		// Keep the partial file in place rather than risk removing a replacement.
		return Location{}, fileError("config_write_failed", "write", location.Path, err)
	}
	return location, nil
}

func (l Loader) locate(ctx context.Context, dir, override string) (Location, string, error) {
	if dir == "" {
		dir = "."
	}
	start, err := filepath.Abs(dir)
	if err != nil {
		return Location{}, "", fileError("config_path_invalid", "resolve", dir, err)
	}
	var target string
	if override != "" {
		target = override
		if !filepath.IsAbs(target) {
			target = filepath.Join(start, target)
		}
		target = filepath.Clean(target)
		start = filepath.Dir(target)
	}
	start, err = filepath.EvalSymlinks(start)
	if err != nil {
		return Location{}, "", fileError("config_path_invalid", "resolve configuration directory", dir, err)
	}
	root, err := git.New(l.Executor, start, l.Env).Root(ctx)
	if err != nil {
		return Location{}, "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Location{}, "", fileError("config_path_invalid", "resolve repository", root, err)
	}
	if target == "" {
		target = filepath.Join(root, Filename)
	} else {
		target = filepath.Join(start, filepath.Base(target))
	}
	return Location{Path: target, RepositoryRoot: root}, start, nil
}

func fileError(code, action, path string, cause error) error {
	return &Error{Code: code, Message: fmt.Sprintf("could not %s %s: %v", action, path, cause), Cause: cause}
}
