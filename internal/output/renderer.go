// Package output renders typed command results for humans and automation.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mkramb/planctl/internal/config"
	"github.com/mkramb/planctl/internal/github"
	"github.com/mkramb/planctl/internal/plan"
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

type ReviewResult struct {
	ID         string       `json:"id"`
	Provider   string       `json:"provider"`
	Repository string       `json:"repository"`
	URL        string       `json:"url"`
	State      github.State `json:"state"`
}

type PlanResult struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Branch        string      `json:"branch"`
	Base          string      `json:"base"`
	Files         []string    `json:"files"`
	WorkspacePath string      `json:"workspace_path"`
	Status        plan.Status `json:"status"`
}

type ApprovalResult struct {
	Current  int `json:"current"`
	Required int `json:"required"`
}

type FeedbackItem struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
	Path   string `json:"path,omitempty"`
	Line   *int   `json:"line,omitempty"`
}

// OutcomeResult is the single result of the review loop.
type OutcomeResult struct {
	Version        int            `json:"version"`
	Plan           PlanResult     `json:"plan"`
	Review         *ReviewResult  `json:"review,omitempty"`
	Status         plan.Status    `json:"status"`
	Approval       ApprovalResult `json:"approval"`
	Reviewers      []string       `json:"reviewers"`
	Feedback       []FeedbackItem `json:"feedback"`
	Allowed        bool           `json:"allowed"`
	BlockedReasons []string       `json:"blocked_reasons"`
}

type SkillInstall struct {
	Agent string `json:"agent"`
	Path  string `json:"path"`
}

type SkillsResult struct {
	Version   int            `json:"version"`
	Installed []SkillInstall `json:"installed"`
}

type UninstallResult struct {
	Version int            `json:"version"`
	Removed []SkillInstall `json:"removed"`
}

func planResult(p plan.Plan, status plan.Status) PlanResult {
	files := p.Files
	if files == nil {
		files = []string{}
	}
	return PlanResult{
		ID: p.ID, Title: p.Title, Branch: p.Branch, Base: p.Base, Files: files,
		WorkspacePath: p.WorkspacePath, Status: status,
	}
}

func reviewResult(eval plan.Evaluation) *ReviewResult {
	if eval.Review == nil {
		return nil
	}
	return &ReviewResult{
		ID: eval.Review.Ref.ID, Provider: eval.Review.Ref.Provider, Repository: eval.Review.Ref.Repository, URL: eval.Review.URL,
		State: eval.Review.State,
	}
}

func NewOutcomeResult(eval plan.Evaluation) OutcomeResult {
	items := make([]FeedbackItem, len(eval.Feedback))
	for i, f := range eval.Feedback {
		items[i] = FeedbackItem{ID: f.ID, Author: f.Author, Body: f.Body, Path: f.Path, Line: f.Line}
	}
	reasons := eval.BlockedReasons
	if reasons == nil {
		reasons = []string{}
	}
	reviewers := eval.Reviewers
	if reviewers == nil {
		reviewers = []string{}
	}
	return OutcomeResult{
		Version: Version, Plan: planResult(eval.Plan, eval.Status), Review: reviewResult(eval),
		Status: eval.Status, Approval: ApprovalResult{Current: eval.Approvals, Required: eval.RequiredApprovals},
		Reviewers: reviewers, Feedback: items, Allowed: eval.Allowed, BlockedReasons: reasons,
	}
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

func (r Renderer) Outcome(result OutcomeResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nFiles    %s\nStatus   %s\n", result.Plan.Title, strings.Join(result.Plan.Files, ", "), statusLabel(result.Status))
	if result.Review != nil {
		fmt.Fprintf(&b, "Review   %s\n", result.Review.URL)
	}
	fmt.Fprintf(&b, "Approvals %d / %d\n", result.Approval.Current, result.Approval.Required)
	if result.Allowed {
		fmt.Fprintf(&b, "Approved on base %s\n", result.Plan.Base)
	} else {
		for _, item := range result.Feedback {
			fmt.Fprintf(&b, "\n%s", item.Author)
			if item.Path != "" {
				fmt.Fprintf(&b, " — %s", item.Path)
				if item.Line != nil {
					fmt.Fprintf(&b, ":%d", *item.Line)
				}
			}
			fmt.Fprintf(&b, "\n\n  %s\n", item.Body)
		}
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func (r Renderer) Skills(result SkillsResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	for _, installed := range result.Installed {
		fmt.Fprintf(&b, "Installed /planctl for %s at %s\n", installed.Agent, installed.Path)
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func (r Renderer) Uninstall(result UninstallResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	for _, removed := range result.Removed {
		fmt.Fprintf(&b, "Removed /planctl for %s at %s\n", removed.Agent, removed.Path)
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

var statusLabels = map[plan.Status]string{
	plan.StatusDraft: "Draft", plan.StatusInReview: "In review",
	plan.StatusChangesRequested: "Changes requested", plan.StatusApproved: "Approved",
	plan.StatusClosed: "Closed", plan.StatusMerged: "Merged",
}

func statusLabel(status plan.Status) string {
	return statusLabels[status]
}

func writeJSON(w io.Writer, result any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
