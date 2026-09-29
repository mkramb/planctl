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
		Version:     1,
		Review:      config.Review{Provider: "github"},
		Branch:      config.Branch{Base: "", Pattern: "review/{slug}"},
		PullRequest: config.PullRequest{Title: "Review: {title}"},
	}, cfg)
}

func TestExpandedConfiguration(t *testing.T) {
	t.Parallel()
	cfg, err := config.Parse([]byte(`version: 1
review:
  provider: github
branch:
  base: master
  pattern: proposal/{slug}
pull_request:
  title: "Proposal: {title}"
`))
	require.NoError(t, err)
	assert.Equal(t, config.Config{
		Version:     1,
		Review:      config.Review{Provider: "github"},
		Branch:      config.Branch{Base: "master", Pattern: "proposal/{slug}"},
		PullRequest: config.PullRequest{Title: "Proposal: {title}"},
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
		{"missing version", "branch:\n  base: master", "unsupported_config_version"},
		{"future version", "version: 2", "unsupported_config_version"},
		{"fractional version", "version: 1.5", "invalid_config"},
		{"provider", "version: 1\nreview:\n  provider: gitlab", "unsupported_review_provider"},
		{"unknown field", "version: 1\ncurrent_plan: add-sso", "invalid_config"},
		{"nested unknown field", "version: 1\nreview:\n  reviewers: [alice]", "invalid_config"},
		{"removed field", "version: 1\nplan:\n  retention: pr-only", "invalid_config"},
		{"removed draft field", "version: 1\npull_request:\n  draft: true", "invalid_config"},
		{"removed approvals field", "version: 1\nreview:\n  required_approvals: 2", "invalid_config"},
		{"duplicate field", "version: 1\nversion: 1", "invalid_config"},
		{"multiple documents", "version: 1\n---\nversion: 1", "invalid_config"},
		{"empty extra document", "version: 1\n---\n", "invalid_config"},
		{"base flag", "version: 1\nbranch:\n  base: --help", "invalid_config"},
		{"base revision", "version: 1\nbranch:\n  base: main~1", "invalid_config"},
		{"missing slug", "version: 1\nbranch:\n  pattern: reviews", "invalid_config"},
		{"multiple slugs", "version: 1\nbranch:\n  pattern: '{slug}/{slug}'", "invalid_config"},
		{"unknown placeholder", "version: 1\nbranch:\n  pattern: '{unknown}/{slug}'", "invalid_config"},
		{"invalid branch", "version: 1\nbranch:\n  pattern: 'review//{slug}'", "invalid_config"},
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
