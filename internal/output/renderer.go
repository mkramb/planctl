// Package output renders typed command results for humans and automation.
package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/plan"
	"github.com/mkramb/planctl/internal/review"
)

const Version = 1

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResult struct {
	Version int         `json:"version"`
	Error   ErrorDetail `json:"error"`
}

type VersionResult struct {
	Version int    `json:"version"`
	Build   string `json:"build"`
}

type InitResult struct {
	Version int             `json:"version"`
	Config  config.Location `json:"config"`
}

type CreateResult struct {
	Version int       `json:"version"`
	Plan    plan.Plan `json:"plan"`
}

type ReviewResult struct {
	ID       string       `json:"id"`
	Provider string       `json:"provider"`
	URL      string       `json:"url"`
	State    review.State `json:"state"`
	Draft    bool         `json:"draft"`
}

type PublishResult struct {
	Version int          `json:"version"`
	Plan    plan.Plan    `json:"plan"`
	Review  ReviewResult `json:"review"`
	Commit  string       `json:"commit"`
}

type Renderer struct {
	Stdout io.Writer
	Stderr io.Writer
	JSON   bool
}

func (r Renderer) Error(result ErrorResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stderr, "error: %s\n", result.Error.Message)
	return err
}

func (r Renderer) Version(result VersionResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stdout, "planctl %s\n", result.Build)
	return err
}

func (r Renderer) Init(result InitResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stdout, "Created %s\n", result.Config.Path)
	return err
}

func (r Renderer) Create(result CreateResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stdout, "Created plan: %s\nPlan file: %s\nBranch: %s\n", result.Plan.Title, result.Plan.AbsolutePath, result.Plan.Branch)
	return err
}

func (r Renderer) Publish(result PublishResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stdout, "Published plan: %s\nReview: %s\nBranch: %s\nCommit: %s\n",
		result.Plan.Title, result.Review.URL, result.Plan.Branch, result.Commit)
	return err
}

func writeJSON(w io.Writer, result any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
