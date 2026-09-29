//go:build integration

package main

import (
	"context"
	"os"
	"testing"

	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/process"
)

// TestRealGitHubResolution verifies the GitHub provider against the real gh CLI
// and network. It runs only under the "gh" build tag, and skips unless a test
// repository is provided.
//
//	go test -tags gh ./cmd/planctl/ -run TestRealGitHubResolution
func TestRealGitHubResolution(t *testing.T) {
	repo := os.Getenv("PLANCTL_GH_TEST_REPO")
	if repo == "" {
		t.Skip("set PLANCTL_GH_TEST_REPO to an owner/repository you can access (e.g. acme/payments)")
	}
	provider := github.NewProvider(process.Runner{})
	got, err := provider.ResolveRepository(context.Background(), "https://github.com/"+repo+".git")
	if err != nil {
		t.Skipf("gh unavailable or unauthenticated: %v", err)
	}
	if got != repo {
		t.Fatalf("resolved %q, want %q", got, repo)
	}
}
