//go:build integration

package main

import (
	"testing"
	"time"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startLoop runs `planctl` in the background and returns a channel that
// receives its result when the command unblocks.
func startLoop(t *testing.T, env *Environment, args ...string) <-chan Result {
	t.Helper()
	ch := make(chan Result, 1)
	command := append(append([]string{}, args...), "--poll", "20ms")
	go func() { ch <- env.Run(t, command...) }()
	return ch
}

// findReview waits briefly and returns the published review for a branch.
func findReview(t *testing.T, env *Environment, branch string) github.Ref {
	t.Helper()
	var found *github.Review
	for i := 0; i < 300 && found == nil; i++ {
		r, err := env.Provider.FindReview(t.Context(), github.FindRequest{Repository: "acme/payments", HeadBranch: branch})
		if err == nil {
			found = &r
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotNil(t, found, "review was not published on %s", branch)
	return found.Ref
}

func waitOutcome(t *testing.T, ch <-chan Result) output.OutcomeResult {
	t.Helper()
	select {
	case result := <-ch:
		result.RequireSuccess(t)
		return DecodeJSON[output.OutcomeResult](t, result)
	case <-time.After(5 * time.Second):
		t.Fatal("command did not unblock")
		return output.OutcomeResult{}
	}
}

func waitOutcomeError(t *testing.T, ch <-chan Result, code string) {
	t.Helper()
	select {
	case result := <-ch:
		assert.Equal(t, 1, result.ExitCode)
		failure := DecodeJSON[output.ErrorResult](t, result)
		assert.Equal(t, code, failure.Error.Code)
	case <-time.After(5 * time.Second):
		t.Fatal("command did not unblock")
	}
}
