// Package output renders typed command results for humans and automation.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
	ID         string       `json:"id"`
	Provider   string       `json:"provider"`
	Repository string       `json:"repository"`
	URL        string       `json:"url"`
	State      review.State `json:"state"`
	Draft      bool         `json:"draft"`
}

type PublishResult struct {
	Version int          `json:"version"`
	Plan    plan.Plan    `json:"plan"`
	Review  ReviewResult `json:"review"`
	Commit  string       `json:"commit"`
}

type PlanStatusResult struct {
	ID     string      `json:"id"`
	Status plan.Status `json:"status"`
}

type ApprovalResult struct {
	Current  int `json:"current"`
	Required int `json:"required"`
}

type StatusResult struct {
	Version       int              `json:"version"`
	Plan          PlanStatusResult `json:"plan"`
	Review        *ReviewResult    `json:"review,omitempty"`
	Approval      ApprovalResult   `json:"approval"`
	FeedbackCount int              `json:"feedback_count"`
}

type FeedbackItem struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
	Path   string `json:"path,omitempty"`
	Line   *int   `json:"line,omitempty"`
}

type FeedbackResult struct {
	Version  int            `json:"version"`
	Plan     plan.Plan      `json:"plan"`
	Status   plan.Status    `json:"status"`
	Review   *ReviewResult  `json:"review,omitempty"`
	Feedback []FeedbackItem `json:"feedback"`
}

type ContextPlanResult struct {
	ID            string      `json:"id"`
	Title         string      `json:"title"`
	Path          string      `json:"path"`
	Status        plan.Status `json:"status"`
	WorkspacePath string      `json:"workspace_path"`
	AbsolutePath  string      `json:"absolute_path"`
}

type RepositoriesResult struct {
	Implementation string `json:"implementation"`
	Plans          string `json:"plans"`
}

type ImplementationResult struct {
	Allowed        bool     `json:"allowed"`
	Base           string   `json:"base"`
	BlockedReasons []string `json:"blocked_reasons"`
}

type ContextResult struct {
	Version        int                  `json:"version"`
	Plan           ContextPlanResult    `json:"plan"`
	Repositories   RepositoriesResult   `json:"repositories"`
	Review         *ReviewResult        `json:"review,omitempty"`
	Implementation ImplementationResult `json:"implementation"`
}

type CompleteResult struct {
	Version          int               `json:"version"`
	Plan             ContextPlanResult `json:"plan"`
	Review           ReviewResult      `json:"review"`
	Retention        string            `json:"retention"`
	PrunedRemote     bool              `json:"pruned_remote"`
	WorkspaceRemoved bool              `json:"workspace_removed"`
}

type SkillInstall struct {
	Agent string `json:"agent"`
	Path  string `json:"path"`
}

type SkillsResult struct {
	Version   int            `json:"version"`
	Installed []SkillInstall `json:"installed"`
}

func NewCompleteResult(c plan.Completion) CompleteResult {
	return CompleteResult{
		Version: Version,
		Plan: ContextPlanResult{
			ID: c.Plan.ID, Title: c.Plan.Title, Path: c.Plan.Path, Status: statusForState(c.Review.State),
			WorkspacePath: c.Plan.WorkspacePath, AbsolutePath: c.Plan.AbsolutePath,
		},
		Review:           ReviewResult{ID: c.Review.Ref.ID, Provider: c.Review.Ref.Provider, Repository: c.Review.Ref.Repository, URL: c.Review.URL, State: c.Review.State, Draft: c.Review.Draft},
		Retention:        c.Retention,
		PrunedRemote:     c.PrunedRemote,
		WorkspaceRemoved: c.WorkspaceRemoved,
	}
}

func statusForState(state review.State) plan.Status {
	switch state {
	case review.Merged:
		return plan.StatusMerged
	case review.Closed:
		return plan.StatusClosed
	default:
		return plan.StatusInReview
	}
}

func reviewResult(eval plan.Evaluation) *ReviewResult {
	if eval.Review == nil {
		return nil
	}
	return &ReviewResult{
		ID: eval.Review.Ref.ID, Provider: eval.Review.Ref.Provider, Repository: eval.Review.Ref.Repository, URL: eval.Review.URL,
		State: eval.Review.State, Draft: eval.Review.Draft,
	}
}

func NewStatusResult(eval plan.Evaluation) StatusResult {
	return StatusResult{
		Version:       Version,
		Plan:          PlanStatusResult{ID: eval.Plan.ID, Status: eval.Status},
		Review:        reviewResult(eval),
		Approval:      ApprovalResult{Current: eval.Approvals, Required: eval.RequiredApprovals},
		FeedbackCount: len(eval.Feedback),
	}
}

func NewFeedbackResult(eval plan.Evaluation) FeedbackResult {
	items := make([]FeedbackItem, len(eval.Feedback))
	for i, f := range eval.Feedback {
		items[i] = FeedbackItem{ID: f.ID, Author: f.Author, Body: f.Body, Path: f.Path, Line: f.Line}
	}
	return FeedbackResult{Version: Version, Plan: eval.Plan, Status: eval.Status, Review: reviewResult(eval), Feedback: items}
}

func NewContextResult(eval plan.Evaluation) ContextResult {
	reasons := eval.BlockedReasons
	if reasons == nil {
		reasons = []string{}
	}
	return ContextResult{
		Version: Version,
		Plan: ContextPlanResult{
			ID: eval.Plan.ID, Title: eval.Plan.Title, Path: eval.Plan.Path, Status: eval.Status,
			WorkspacePath: eval.Plan.WorkspacePath, AbsolutePath: eval.Plan.AbsolutePath,
		},
		Repositories: RepositoriesResult{Implementation: eval.Repository, Plans: eval.PlansRepository},
		Review:       reviewResult(eval),
		Implementation: ImplementationResult{
			Allowed: eval.Allowed, Base: eval.Plan.Base, BlockedReasons: reasons,
		},
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

func statusLabel(status plan.Status) string {
	labels := map[plan.Status]string{
		plan.StatusDraft: "Draft", plan.StatusInReview: "In review",
		plan.StatusChangesRequested: "Changes requested", plan.StatusApproved: "Approved",
		plan.StatusClosed: "Closed", plan.StatusMerged: "Merged",
	}
	return labels[status]
}

func (r Renderer) Status(result StatusResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", result.Plan.ID)
	if result.Review != nil {
		fmt.Fprintf(&b, "Provider    %s\nReview      #%s\n", result.Review.Provider, result.Review.ID)
	}
	fmt.Fprintf(&b, "Status      %s\n", statusLabel(result.Plan.Status))
	if result.Review != nil {
		fmt.Fprintf(&b, "Approvals   %d / %d\n", result.Approval.Current, result.Approval.Required)
		fmt.Fprintf(&b, "Feedback    %d\n", result.FeedbackCount)
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func (r Renderer) Feedback(result FeedbackResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", result.Plan.Title)
	if result.Review != nil {
		fmt.Fprintf(&b, "%s #%s\n", result.Review.Provider, result.Review.ID)
	}
	fmt.Fprintf(&b, "\n%s\n", strings.ToUpper(statusLabel(result.Status)))
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
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func (r Renderer) Context(result ContextResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nPlan file    %s\nStatus       %s\n", result.Plan.Title, result.Plan.AbsolutePath, statusLabel(result.Plan.Status))
	if result.Implementation.Allowed {
		fmt.Fprintf(&b, "Implementation allowed on base %s\n", result.Implementation.Base)
	} else {
		fmt.Fprintf(&b, "Implementation blocked: %s\n", strings.Join(result.Implementation.BlockedReasons, ", "))
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
		fmt.Fprintf(&b, "Installed %s skill at %s\n", installed.Agent, installed.Path)
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func (r Renderer) Complete(result CompleteResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Completed plan: %s\nRetention: %s\nReview: %s\n", result.Plan.Title, result.Retention, result.Review.URL)
	if result.PrunedRemote {
		fmt.Fprintf(&b, "Remote branch: pruned\n")
	}
	if result.WorkspaceRemoved {
		fmt.Fprintf(&b, "Worktree: removed\n")
	} else {
		fmt.Fprintf(&b, "Worktree: kept (has local work)\n")
	}
	_, err := fmt.Fprint(r.Stdout, b.String())
	return err
}

func writeJSON(w io.Writer, result any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
