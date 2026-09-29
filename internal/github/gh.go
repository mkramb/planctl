package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mkramb/planctl/internal/process"
)

// Cmd runs gh commands. Tests substitute it with canned output.
type Cmd interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// Error carries a short, sanitized gh diagnostic (no arguments, no env).
type Error struct {
	Command string
	Detail  string
	Cause   error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return "gh " + e.Command + " failed: " + e.Detail
	}
	return "gh " + e.Command + " failed"
}

func (e *Error) Unwrap() error { return e.Cause }

// NotFound reports whether gh indicated the resource does not exist.
func NotFound(err error) bool {
	var ghErr *Error
	if !errors.As(err, &ghErr) {
		return false
	}
	detail := strings.ToLower(ghErr.Detail)
	return strings.Contains(detail, "not found") || strings.Contains(detail, "no pull requests found") ||
		strings.Contains(detail, "could not resolve")
}

type runner struct {
	exec process.Executor
}

func (r runner) Run(ctx context.Context, args ...string) (string, error) {
	result, err := r.exec.Run(ctx, process.Request{Executable: "gh", Args: args})
	if err != nil {
		detail := strings.TrimSpace(result.Stderr)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		command := "command"
		if len(args) > 0 {
			command = args[0]
		}
		return "", &Error{Command: command, Detail: detail, Cause: err}
	}
	return result.Stdout, nil
}

// RepoFromRemote extracts owner/repo from a Git remote URL. It accepts HTTPS
// and SSH forms; other shapes (file paths for tests) have no GitHub repository.
func RepoFromRemote(remote string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", fmt.Errorf("remote URL is empty")
	}
	var path string
	switch {
	case strings.HasPrefix(trimmed, "git@"):
		_, rest, ok := strings.Cut(trimmed, ":")
		if !ok {
			return "", fmt.Errorf("cannot parse SSH remote %q", remote)
		}
		path = rest
	case strings.Contains(trimmed, "://"):
		_, rest, _ := strings.Cut(trimmed, "://")
		_, path, _ = strings.Cut(rest, "/")
	default:
		return "", fmt.Errorf("remote %q is not a GitHub URL", remote)
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("remote %q is not an owner/repository URL", remote)
	}
	return parts[0] + "/" + parts[1], nil
}
