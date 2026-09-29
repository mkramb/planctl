package process_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/mkramb/planctl/internal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type helperOutput struct {
	Dir   string
	Value string
	Args  []string
}

// Execute the test binary itself so process tests need neither a shell nor any
// external language runtime. Only Runner starts the child process.
func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("PLANCTL_PROCESS_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "inspect":
		dir, err := os.Getwd()
		if err != nil {
			os.Exit(9)
		}
		separator := slices.Index(os.Args, "--")
		if err := json.NewEncoder(os.Stdout).Encode(helperOutput{
			Dir: dir, Value: os.Getenv("PLANCTL_TEST_VALUE"), Args: os.Args[separator+1:],
		}); err != nil {
			os.Exit(9)
		}
		_, _ = fmt.Fprint(os.Stderr, "separate diagnostics")
		os.Exit(0)
	case "fail":
		_, _ = fmt.Fprint(os.Stdout, "partial output")
		_, _ = fmt.Fprint(os.Stderr, "sensitive child diagnostic")
		os.Exit(7)
	case "wait":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(8)
	}
}

func helperRequest(t *testing.T, mode string) process.Request {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	return process.Request{
		Executable: executable, Args: []string{"-test.run=^TestProcessHelper$", "--"},
		Dir: t.TempDir(), Env: append(os.Environ(), "PLANCTL_PROCESS_HELPER="+mode),
	}
}

func TestRunnerPreservesArgumentsDirectoryAndStreams(t *testing.T) {
	t.Parallel()
	req := helperRequest(t, "inspect")
	args := []string{"an argument with spaces", "$(not-a-shell-command)", "--flag=value"}
	req.Args = append(req.Args, args...)
	req.Env = append(req.Env, "PLANCTL_TEST_VALUE=explicit value")
	result, err := (process.Runner{}).Run(t.Context(), req)
	require.NoError(t, err)
	var got helperOutput
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &got))
	wantDir, err := filepath.EvalSymlinks(req.Dir)
	require.NoError(t, err)
	assert.Equal(t, wantDir, got.Dir)
	assert.Equal(t, "explicit value", got.Value)
	assert.Equal(t, args, got.Args)
	assert.Zero(t, result.ExitCode)
	assert.Equal(t, "separate diagnostics", result.Stderr)
}

func TestRunnerNonzeroExitPreservesOutputWithoutLeakingIt(t *testing.T) {
	t.Parallel()
	result, err := (process.Runner{}).Run(t.Context(), helperRequest(t, "fail"))
	var commandErr *process.Error
	require.ErrorAs(t, err, &commandErr)
	assert.Equal(t, 7, commandErr.ExitCode)
	assert.Equal(t, 7, result.ExitCode)
	assert.Equal(t, "partial output", result.Stdout)
	assert.Equal(t, "sensitive child diagnostic", result.Stderr)
	assert.NotContains(t, err.Error(), "sensitive", "error must not leak raw child diagnostics")
}

func TestRunnerMissingExecutable(t *testing.T) {
	t.Parallel()
	result, err := (process.Runner{}).Run(t.Context(), process.Request{Executable: "planctl-test-missing-executable-9234"})
	require.ErrorIs(t, err, exec.ErrNotFound)
	assert.Equal(t, -1, result.ExitCode)
}

func TestRunnerCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, err := (process.Runner{}).Run(ctx, helperRequest(t, "wait"))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
