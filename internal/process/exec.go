// Package process is the single boundary for external program execution.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

type Request struct {
	Executable string
	Args       []string
	Dir        string
	// Env replaces the inherited environment when non-nil.
	Env []string
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Executor interface {
	Run(context.Context, Request) (Result, error)
}

type Runner struct{}

// Error deliberately excludes arguments, environment, and subprocess output:
// those may contain credentials or repository content.
type Error struct {
	Executable string
	ExitCode   int
	Cause      error
}

func (e *Error) Error() string {
	if errors.Is(e.Cause, exec.ErrNotFound) {
		return fmt.Sprintf("%s was not found in PATH", e.Executable)
	}
	if errors.Is(e.Cause, context.Canceled) || errors.Is(e.Cause, context.DeadlineExceeded) {
		return fmt.Sprintf("%s was canceled", e.Executable)
	}
	if e.ExitCode >= 0 {
		return fmt.Sprintf("%s exited with status %d", e.Executable, e.ExitCode)
	}
	return fmt.Sprintf("could not start %s", e.Executable)
}

func (e *Error) Unwrap() error { return e.Cause }

func (Runner) Run(ctx context.Context, req Request) (Result, error) {
	cmd := exec.CommandContext(ctx, req.Executable, req.Args...)
	cmd.Dir = req.Dir
	cmd.Env = req.Env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	result.ExitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return result, &Error{Executable: req.Executable, ExitCode: result.ExitCode, Cause: err}
}
