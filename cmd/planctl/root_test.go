package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/mkramb/planctl/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanceledCommandProducesStructuredFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	exit := Run(ctx, []string{"version", "--json"}, Dependencies{Stdout: &stdout, Stderr: &stderr})
	var result output.ErrorResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, 130, exit)
	assert.Equal(t, 1, result.Version)
	assert.Equal(t, "canceled", result.Error.Code)
	assert.Empty(t, stderr.String())
}

func TestHelpAndVersionNeedNoRepositoryOrProvider(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--help"}, {"--version"}, {"version", "--json"}} {
		var stdout, stderr bytes.Buffer
		exit := Run(t.Context(), args, Dependencies{Stdout: &stdout, Stderr: &stderr})
		assert.Zero(t, exit, "args: %v", args)
		assert.NotEmpty(t, stdout.String(), "args: %v", args)
		assert.Empty(t, stderr.String(), "args: %v", args)
	}
}
