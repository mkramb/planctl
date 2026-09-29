// Package config reads configuration and initializes repositories.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

const Filename = ".planctl.yaml"

const minimal = "version: 1\n"

type Config struct {
	Version     int         `yaml:"version"`
	Review      Review      `yaml:"review"`
	Branch      Branch      `yaml:"branch"`
	PullRequest PullRequest `yaml:"pull_request"`
}

type Review struct {
	Provider string `yaml:"provider"`
}

type Branch struct {
	// An empty base means the repository's default branch, resolved when needed.
	Base    string `yaml:"base"`
	Pattern string `yaml:"pattern"`
}

type PullRequest struct {
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
		Review:      Review{Provider: "github"},
		Branch:      Branch{Pattern: "review/{slug}"},
		PullRequest: PullRequest{Title: "Review: {title}"},
	}
}

// Defaults returns the configuration used when no .planctl.yaml is present.
func Defaults() Config { return defaults() }

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
	var fields struct {
		Version yaml.Node `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return Config{}, &Error{Code: "invalid_config", Message: "could not read configuration fields", Cause: err}
	}
	if fields.Version.Kind != 0 && fields.Version.Tag != "!!int" {
		return Config{}, &Error{Code: "invalid_config", Message: "version must be an integer"}
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Version != 1 {
		return &Error{Code: "unsupported_config_version", Message: fmt.Sprintf("unsupported configuration version %d; set version: 1", c.Version)}
	}
	if c.Review.Provider != "github" {
		return &Error{Code: "unsupported_review_provider", Message: fmt.Sprintf("unsupported review provider %q; planctl supports GitHub only", c.Review.Provider)}
	}
	invalid := func(message string) error { return &Error{Code: "invalid_config", Message: message} }
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
