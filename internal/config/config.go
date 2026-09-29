// Package config reads configuration and initializes repositories.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const Filename = ".planctl.yaml"

const minimal = "version: 1\n\nplan:\n  retention: pr-only\n"

type Config struct {
	Version      int          `yaml:"version"`
	Review       Review       `yaml:"review"`
	Repositories Repositories `yaml:"repositories"`
	Plan         Plan         `yaml:"plan"`
	Branch       Branch       `yaml:"branch"`
	PullRequest  PullRequest  `yaml:"pull_request"`
}

type Review struct {
	Provider          string `yaml:"provider"`
	RequiredApprovals int    `yaml:"required_approvals"`
}

type Repositories struct {
	Plans string `yaml:"plans"`
}

type Plan struct {
	Directory string `yaml:"directory"`
	Retention string `yaml:"retention"`
}

type Branch struct {
	// An empty base means the repository's default branch, resolved when needed.
	Base    string `yaml:"base"`
	Pattern string `yaml:"pattern"`
}

type PullRequest struct {
	Draft bool   `yaml:"draft"`
	Title string `yaml:"title"`
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func defaults() Config {
	return Config{
		Review:       Review{Provider: "github", RequiredApprovals: 1},
		Repositories: Repositories{Plans: "current"},
		Plan:         Plan{Directory: ".plans", Retention: "pr-only"},
		Branch:       Branch{Pattern: "plan/{slug}"},
		PullRequest:  PullRequest{Draft: false, Title: "Plan: {title}"},
	}
}

func Parse(data []byte) (Config, error) {
	cfg := defaults()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, &Error{Code: "invalid_config", Message: "configuration must be a YAML mapping with known fields", Cause: err}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, &Error{Code: "invalid_config", Message: "configuration must contain exactly one YAML document", Cause: err}
	}
	// yaml.v3 converts floats to integers when decoding into int fields. Reject
	// that coercion: 1.5 approvals must not silently become one approval.
	var fields struct {
		Version yaml.Node `yaml:"version"`
		Review  struct {
			RequiredApprovals yaml.Node `yaml:"required_approvals"`
		} `yaml:"review"`
	}
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return Config{}, &Error{Code: "invalid_config", Message: "could not read configuration fields", Cause: err}
	}
	for _, field := range []struct {
		name string
		node yaml.Node
	}{
		{"version", fields.Version},
		{"review.required_approvals", fields.Review.RequiredApprovals},
	} {
		if field.node.Kind != 0 && field.node.Tag != "!!int" {
			return Config{}, &Error{Code: "invalid_config", Message: field.name + " must be an integer"}
		}
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

func (c Config) validate() error {
	if c.Version != 1 {
		return &Error{Code: "unsupported_config_version", Message: fmt.Sprintf("unsupported configuration version %d; set version: 1", c.Version)}
	}
	if c.Review.Provider != "github" {
		return &Error{Code: "unsupported_review_provider", Message: fmt.Sprintf("unsupported review provider %q; planctl currently supports github", c.Review.Provider)}
	}
	invalid := func(message string) error { return &Error{Code: "invalid_config", Message: message} }
	if c.Review.RequiredApprovals < 1 {
		return invalid("review.required_approvals must be at least 1")
	}
	if c.Repositories.Plans != "current" && (!repositoryName.MatchString(c.Repositories.Plans) || strings.HasSuffix(c.Repositories.Plans, "/.") || strings.HasSuffix(c.Repositories.Plans, "/..")) {
		return invalid("repositories.plans must be current or an owner/repository name")
	}
	if c.Plan.Retention != "pr-only" && c.Plan.Retention != "repository" {
		return invalid("plan.retention must be pr-only or repository")
	}
	if c.Plan.Directory == "" || path.IsAbs(c.Plan.Directory) || strings.ContainsAny(c.Plan.Directory, "\\:\x00") {
		return invalid("plan.directory must be a relative path inside the repository")
	}
	for _, part := range strings.Split(c.Plan.Directory, "/") {
		if part == ".." || strings.EqualFold(part, ".git") {
			return invalid("plan.directory must not contain .. or .git")
		}
	}
	if c.Branch.Base != "" && !validBranch(c.Branch.Base) {
		return invalid("branch.base must be a valid Git branch name")
	}
	if strings.Count(c.Branch.Pattern, "{slug}") != 1 || !validBranch(strings.ReplaceAll(c.Branch.Pattern, "{slug}", "example")) || strings.ContainsAny(strings.ReplaceAll(c.Branch.Pattern, "{slug}", ""), "{}") {
		return invalid("branch.pattern must be a valid branch pattern containing exactly one {slug}")
	}
	if strings.TrimSpace(c.PullRequest.Title) == "" || strings.ContainsAny(strings.ReplaceAll(c.PullRequest.Title, "{title}", ""), "{}\r\n") {
		return invalid("pull_request.title must be a nonempty single-line title using only the {title} placeholder")
	}
	return nil
}

func validBranch(name string) bool {
	if name == "" || name == "@" || name == "HEAD" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.ContainsAny(name, "~^:?*[\\") {
		return false
	}
	for _, char := range name {
		if char <= ' ' || char == 127 {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
