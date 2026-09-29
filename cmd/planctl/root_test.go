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

	var helpOut, helpErr bytes.Buffer
	exit := Run(t.Context(), []string{"--help"}, Dependencies{Stdout: &helpOut, Stderr: &helpErr})
	assert.Zero(t, exit)
	assert.Empty(t, helpErr.String())
	for _, want := range []string{"planctl", "--json", "--reviewer"} {
		assert.Contains(t, helpOut.String(), want)
	}

	var versionOut, versionErr bytes.Buffer
	exit = Run(t.Context(), []string{"version", "--json"}, Dependencies{Stdout: &versionOut, Stderr: &versionErr, Version: "test"})
	assert.Zero(t, exit)
	assert.Empty(t, versionErr.String())
	var result output.VersionResult
	require.NoError(t, json.Unmarshal(versionOut.Bytes(), &result))
	assert.Equal(t, 1, result.Version)
	assert.Equal(t, "test", result.Build)
}
