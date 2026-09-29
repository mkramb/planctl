package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mkramb/planctl/internal/git"
	"github.com/mkramb/planctl/internal/process"
)

// Mock is a stateful, in-memory GitHub that implements Cmd: it answers gh
// invocations by reading and mutating its own records, and observes real
// pushes to registered bare remotes so review heads track the actual branch.
// It lives here, next to the provider it fakes, and is used by integration
// tests (and never by production code).
type Mock struct {
	mu              sync.Mutex
	nextID          int
	repos           map[string]*git.Client // repo name -> bare remote (may be nil)
	remoteIDs       map[string]string      // remote URL -> repo name
	prs             map[string]*prRecord   // "repo:number" -> record
	byBranch        map[string][]int       // "repo:head" -> numbers
	createFailure   error
	failAfterCreate bool
	lookupFailure   error
}

type prRecord struct {
	number     int
	title      string
	body       string
	url        string
	state      State
	headBranch string
	baseBranch string
	decisions  []Decision
	feedback   []Feedback
}

func NewMock() *Mock {
	return &Mock{
		repos:     make(map[string]*git.Client),
		remoteIDs: make(map[string]string),
		prs:       make(map[string]*prRecord),
		byBranch:  make(map[string][]int),
	}
}

// RegisterRepository maps a repository name to a local bare remote so the mock
// can observe pushes and resolve branch heads.
func (m *Mock) RegisterRepository(name, remote string, env []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remoteIDs[remote] = name
	m.repos[name] = git.New(process.Runner{}, remote, env)
}

func (m *Mock) Run(ctx context.Context, args ...string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", errors.New("empty gh command")
	}
	switch args[0] {
	case "repo":
		return m.repoView(args[1:])
	case "pr":
		return m.prCommand(args[1:])
	case "api":
		return m.apiCommand(args)
	default:
		return "", fmt.Errorf("unexpected gh command: %s", strings.Join(args, " "))
	}
}

func (m *Mock) repoView(rest []string) (string, error) {
	pos, _ := splitArgs(rest)
	if len(pos) < 2 {
		return "", errors.New("repo view missing name")
	}
	name := pos[1]
	if resolved, ok := m.remoteIDs[name]; ok {
		name = resolved
	}
	if _, ok := m.repos[name]; !ok {
		return "", &Error{Command: "repo", Detail: "could not resolve to a repository"}
	}
	return fmt.Sprintf(`{"nameWithOwner":%q}`, name), nil
}

func (m *Mock) prCommand(rest []string) (string, error) {
	if len(rest) == 0 {
		return "", errors.New("missing pr subcommand")
	}
	switch rest[0] {
	case "create":
		return m.prCreate(rest[1:])
	case "list":
		return m.prList(rest[1:])
	case "view":
		return m.prView(rest[1:])
	case "edit":
		return m.prEdit(rest[1:])
	case "close":
		return m.prClose(rest[1:])
	default:
		return "", fmt.Errorf("unexpected pr subcommand %q", rest[0])
	}
}

func (m *Mock) prCreate(rest []string) (string, error) {
	_, flags := splitArgs(rest)
	repo := flags["repo"][0]
	head := flags["head"][0]
	base := flags["base"][0]
	title := flags["title"][0]
	body := flags["body"][0]

	failure, after := m.createFailure, m.failAfterCreate
	m.createFailure = nil
	if failure != nil && !after {
		return "", failure
	}
	for _, num := range m.byBranch[repo+":"+head] {
		if m.prs[prKey(repo, num)].state == Open {
			return "", &Error{Command: "pr", Detail: "a pull request already exists"}
		}
	}
	if remote := m.repos[repo]; remote != nil {
		_, found, err := remote.ResolveCommit(context.Background(), "refs/heads/"+head)
		if err != nil {
			return "", err
		}
		if !found {
			return "", errors.New("review head must exist on the remote before review creation")
		}
	}
	m.nextID++
	num := m.nextID
	m.prs[prKey(repo, num)] = &prRecord{
		number: num, title: title, body: body,
		url:   "https://review.invalid/" + repo + "/" + strconv.Itoa(num),
		state: Open, headBranch: head, baseBranch: base,
	}
	m.byBranch[repo+":"+head] = append(m.byBranch[repo+":"+head], num)
	if failure != nil {
		return "", failure
	}
	return m.prs[prKey(repo, num)].url, nil
}

func (m *Mock) prList(rest []string) (string, error) {
	if m.lookupFailure != nil {
		err := m.lookupFailure
		m.lookupFailure = nil
		return "", err
	}
	_, flags := splitArgs(rest)
	repo := flags["repo"][0]
	head := flags["head"][0]
	numbers := append([]int(nil), m.byBranch[repo+":"+head]...)
	sort.Ints(numbers)
	type listed struct {
		Number int `json:"number"`
	}
	out := make([]listed, 0, len(numbers))
	for _, n := range numbers {
		out = append(out, listed{Number: n})
	}
	data, _ := json.Marshal(out)
	return string(data), nil
}

func (m *Mock) prView(rest []string) (string, error) {
	pos, _ := splitArgs(rest)
	if len(pos) < 1 {
		return "", errors.New("pr view missing number")
	}
	num, err := strconv.Atoi(pos[0])
	if err != nil {
		return "", err
	}
	_, flags := splitArgs(rest)
	repo := flags["repo"][0]
	rec := m.prs[prKey(repo, num)]
	if rec == nil {
		return "", &Error{Command: "pr", Detail: "no pull requests found"}
	}
	head := ""
	if remote := m.repos[repo]; remote != nil {
		commit, found, err := remote.ResolveCommit(context.Background(), "refs/heads/"+rec.headBranch)
		if err != nil {
			return "", err
		}
		if found {
			head = commit
		}
	}
	data, _ := json.Marshal(prView{
		Number: rec.number, Title: rec.title, Body: rec.body, URL: rec.url,
		State: prStateString(rec.state), HeadRefName: rec.headBranch, BaseRefName: rec.baseBranch,
		HeadRefOid: head,
	})
	return string(data), nil
}

func (m *Mock) prEdit(rest []string) (string, error) {
	// Reviewers live in the PR body metadata that planctl manages; adding them
	// requires no state change here.
	return "", nil
}

func (m *Mock) prClose(rest []string) (string, error) {
	pos, _ := splitArgs(rest)
	if len(pos) < 1 {
		return "", errors.New("pr close missing number")
	}
	num, err := strconv.Atoi(pos[0])
	if err != nil {
		return "", err
	}
	_, flags := splitArgs(rest)
	repo := flags["repo"][0]
	rec := m.prs[prKey(repo, num)]
	if rec == nil {
		return "", &Error{Command: "pr", Detail: "no pull requests found"}
	}
	rec.state = Closed
	return "", nil
}

func (m *Mock) apiCommand(args []string) (string, error) {
	endpoint := args[len(args)-1]
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	// repos/<owner>/<repo>/pulls/<n>/<kind>
	if len(parts) < 6 || parts[0] != "repos" || parts[3] != "pulls" {
		return "", fmt.Errorf("unexpected api endpoint %q", endpoint)
	}
	repo := parts[1] + "/" + parts[2]
	num, err := strconv.Atoi(parts[4])
	if err != nil {
		return "", err
	}
	rec := m.prs[prKey(repo, num)]
	if rec == nil {
		return "", &Error{Command: "api", Detail: "no pull requests found"}
	}
	switch parts[5] {
	case "reviews":
		reviews := make([]apiReview, 0, len(rec.decisions))
		for _, d := range rec.decisions {
			id, _ := strconv.ParseInt(d.ID, 10, 64)
			reviews = append(reviews, apiReview{
				ID: id, User: apiUser{Login: d.Author}, State: decisionStateString(d.State),
				CommitID: d.CommitID,
			})
		}
		data, _ := json.Marshal(reviews)
		return string(data), nil
	case "comments":
		comments := make([]apiReviewComment, 0, len(rec.feedback))
		for _, f := range rec.feedback {
			id, _ := strconv.ParseInt(f.ID, 10, 64)
			comments = append(comments, apiReviewComment{
				ID: id, User: apiUser{Login: f.Author}, Body: f.Body, Path: f.Path, Line: f.Line,
			})
		}
		data, _ := json.Marshal(comments)
		return string(data), nil
	default:
		return "", fmt.Errorf("unexpected api endpoint %q", endpoint)
	}
}

// Test controls below simulate human actions and inject failures.

func (m *Mock) lookup(repo string, id string) (*prRecord, error) {
	num, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	rec := m.prs[prKey(repo, num)]
	if rec == nil {
		return nil, ErrNotFound
	}
	return rec, nil
}

func (m *Mock) Approve(ref Ref, author string) error {
	return m.addDecision(ref, author, Approved)
}

func (m *Mock) RequestChanges(ref Ref, author string) error {
	return m.addDecision(ref, author, ChangesRequested)
}

func (m *Mock) addDecision(ref Ref, author string, state DecisionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, err := m.lookup(ref.Repository, ref.ID)
	if err != nil {
		return err
	}
	if rec.state != Open {
		return fmt.Errorf("cannot review a %s review", rec.state)
	}
	head := ""
	if remote := m.repos[ref.Repository]; remote != nil {
		commit, found, err := remote.ResolveCommit(context.Background(), "refs/heads/"+rec.headBranch)
		if err != nil {
			return err
		}
		if found {
			head = commit
		}
	}
	rec.decisions = append(rec.decisions, Decision{
		ID: strconv.Itoa(len(rec.decisions) + 1), Author: author, State: state, CommitID: head,
	})
	return nil
}

func (m *Mock) AddFeedback(ref Ref, item Feedback) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, err := m.lookup(ref.Repository, ref.ID)
	if err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = strconv.Itoa(len(rec.feedback) + 1)
	}
	rec.feedback = append(rec.feedback, cloneFeedback(item))
	return nil
}

func (m *Mock) SetState(ref Ref, state State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, err := m.lookup(ref.Repository, ref.ID)
	if err != nil {
		return err
	}
	if state == Open {
		return errors.New("cannot reopen a review")
	}
	rec.state = state
	return nil
}

func (m *Mock) FailNextCreate(err error, afterCreation bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createFailure, m.failAfterCreate = err, afterCreation
}

func (m *Mock) FailNextLookup(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookupFailure = err
}

func (m *Mock) ReviewCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.prs)
}

func prKey(repo string, num int) string {
	return repo + ":" + strconv.Itoa(num)
}

func cloneFeedback(f Feedback) Feedback {
	if f.Line != nil {
		line := *f.Line
		f.Line = &line
	}
	return f
}

func prStateString(s State) string {
	switch s {
	case Merged:
		return "MERGED"
	case Closed:
		return "CLOSED"
	default:
		return "OPEN"
	}
}

func decisionStateString(s DecisionState) string {
	switch s {
	case Approved:
		return "APPROVED"
	case ChangesRequested:
		return "CHANGES_REQUESTED"
	case Commented:
		return "COMMENTED"
	case Dismissed:
		return "DISMISSED"
	default:
		return "PENDING"
	}
}

// splitArgs separates positional arguments from --flag values (repeatable flags
// accumulate). The returned map holds flags keyed without the leading dashes.
func splitArgs(args []string) ([]string, map[string][]string) {
	flags := map[string][]string{}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			positional = append(positional, a)
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if eq := strings.Index(name, "="); eq >= 0 {
			flags[name[:eq]] = append(flags[name[:eq]], name[eq+1:])
			continue
		}
		val := ""
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			i++
			val = args[i]
		}
		flags[name] = append(flags[name], val)
	}
	return positional, flags
}
