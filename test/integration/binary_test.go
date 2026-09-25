package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/mkramb/planctl/internal/process"
	"github.com/mkramb/planctl/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A small subprocess smoke test checks the wiring that in-process Cobra tests
// cannot: main, actual exit statuses, and output from the compiled executable.
func TestBuiltBinary(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "planctl")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	runner := process.Runner{}
	build, err := runner.Run(t.Context(), process.Request{
		Executable: "go", Args: []string{"build", "-o", binary, "./cmd/planctl"}, Dir: root,
		Env: append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local"),
	})
	require.NoError(t, err, "build binary: %s", build.Stderr)
	for _, scenario := range []struct {
		name string
		args []string
		exit int
	}{
		{name: "version", args: []string{"version", "--json"}},
		{name: "usage failure", args: []string{"--unknown", "--json"}, exit: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			result, err := runner.Run(t.Context(), process.Request{
				Executable: binary, Args: scenario.args, Dir: t.TempDir(),
			})
			assert.Equal(t, scenario.exit, result.ExitCode)
			assert.Equal(t, scenario.exit == 0, err == nil, "process error: %v", err)
			assert.Empty(t, result.Stderr)
			captured := testutil.Result{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}
			if scenario.exit == 0 {
				version := testutil.DecodeJSON[output.VersionResult](t, captured)
				assert.Equal(t, 1, version.Version)
				assert.Equal(t, "dev", version.Build)
			} else {
				failure := testutil.DecodeJSON[output.ErrorResult](t, captured)
				assert.Equal(t, 1, failure.Version)
				assert.Equal(t, "invalid_arguments", failure.Error.Code)
			}
		})
	}
}
