package config_test

import (
	"testing"

	"github.com/mkramb/planctl/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := config.Parse([]byte("version: 1\n"))
	require.NoError(t, err)
	assert.Equal(t, config.Config{
		Version:      1,
		Review:       config.Review{Provider: "github", RequiredApprovals: 1},
		Repositories: config.Repositories{Plans: "current"},
		Plan:         config.Plan{Directory: ".plans", Retention: "pr-only"},
		Branch:       config.Branch{Base: "", Pattern: "plan/{slug}"},
		PullRequest:  config.PullRequest{Draft: false, Title: "Plan: {title}"},
	}, cfg)
}

func TestExpandedConfiguration(t *testing.T) {
	t.Parallel()
	cfg, err := config.Parse([]byte(`version: 1
review:
  provider: github
  required_approvals: 2
repositories:
  plans: acme/engineering-plans
plan:
  directory: plans/payments
  retention: repository
branch:
  base: master
  pattern: proposal/{slug}
pull_request:
  draft: true
  title: "Proposal: {title}"
`))
	require.NoError(t, err)
	assert.Equal(t, config.Config{
		Version:      1,
		Review:       config.Review{Provider: "github", RequiredApprovals: 2},
		Repositories: config.Repositories{Plans: "acme/engineering-plans"},
		Plan:         config.Plan{Directory: "plans/payments", Retention: "repository"},
		Branch:       config.Branch{Base: "master", Pattern: "proposal/{slug}"},
		PullRequest:  config.PullRequest{Draft: true, Title: "Proposal: {title}"},
	}, cfg)
}

func TestInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		yaml string
		code string
	}{
		{"empty", "", "invalid_config"},
		{"malformed", "version: [", "invalid_config"},
		{"sequence", "- version: 1", "invalid_config"},
		{"missing version", "plan: {}", "unsupported_config_version"},
		{"future version", "version: 2", "unsupported_config_version"},
		{"fractional version", "version: 1.5", "invalid_config"},
		{"provider", "version: 1\nreview:\n  provider: gitlab", "unsupported_review_provider"},
		{"unknown field", "version: 1\ncurrent_plan: add-sso", "invalid_config"},
		{"nested unknown field", "version: 1\nreview:\n  reviewers: [alice]", "invalid_config"},
		{"duplicate field", "version: 1\nversion: 1", "invalid_config"},
		{"multiple documents", "version: 1\n---\nversion: 1", "invalid_config"},
		{"empty extra document", "version: 1\n---\n", "invalid_config"},
		{"zero approvals", "version: 1\nreview:\n  required_approvals: 0", "invalid_config"},
		{"negative approvals", "version: 1\nreview:\n  required_approvals: -1", "invalid_config"},
		{"fractional approvals", "version: 1\nreview:\n  required_approvals: 1.5", "invalid_config"},
		{"string approvals", "version: 1\nreview:\n  required_approvals: many", "invalid_config"},
		{"retention", "version: 1\nplan:\n  retention: delete", "invalid_config"},
		{"repository URL", "version: 1\nrepositories:\n  plans: https://github.com/acme/plans", "invalid_config"},
		{"repository path", "version: 1\nrepositories:\n  plans: ../plans", "invalid_config"},
		{"dot repository", "version: 1\nrepositories:\n  plans: acme/..", "invalid_config"},
		{"absolute directory", "version: 1\nplan:\n  directory: /tmp/plans", "invalid_config"},
		{"traversal", "version: 1\nplan:\n  directory: plans/../../other", "invalid_config"},
		{"git directory", "version: 1\nplan:\n  directory: .git/plans", "invalid_config"},
		{"windows directory", "version: 1\nplan:\n  directory: C:\\plans", "invalid_config"},
		{"empty directory", "version: 1\nplan:\n  directory: ''", "invalid_config"},
		{"base flag", "version: 1\nbranch:\n  base: --help", "invalid_config"},
		{"base revision", "version: 1\nbranch:\n  base: main~1", "invalid_config"},
		{"missing slug", "version: 1\nbranch:\n  pattern: plans", "invalid_config"},
		{"multiple slugs", "version: 1\nbranch:\n  pattern: '{slug}/{slug}'", "invalid_config"},
		{"unknown placeholder", "version: 1\nbranch:\n  pattern: '{unknown}/{slug}'", "invalid_config"},
		{"invalid branch", "version: 1\nbranch:\n  pattern: 'plan//{slug}'", "invalid_config"},
		{"empty title", "version: 1\npull_request:\n  title: ''", "invalid_config"},
		{"unknown title placeholder", "version: 1\npull_request:\n  title: '{slug}'", "invalid_config"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Parse([]byte(tt.yaml))
			var failure *config.Error
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, tt.code, failure.Code)
			assert.NotEmpty(t, failure.Message)
		})
	}
}
