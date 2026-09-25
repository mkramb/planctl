# V1 implementation plan

Module: `github.com/mkramb/planctl`. License: MIT.

## Milestones

| # | Milestone | Deliverables | Status |
|---|---|---|---|
| 1 | Foundation and test harness | mise, Go, Cobra, process execution, typed output, real-Git harness, stateful fake provider | Complete |
| 2 | Configuration and init | Configuration discovery, defaults, validation, safe `init` | Pending |
| 3 | Create and managed worktrees | Slugs, plan identity, templates, isolated worktrees, plan selection | Pending |
| 4 | Publish with fake provider | Plan-only commits, real pushes, idempotent review creation and republishing | Pending |
| 5 | Review lifecycle | `status`, `feedback`, `context`, effective current-revision approvals | Pending |
| 6 | GitHub adapter | `gh` integration, ready PRs, discovery, feedback, approvals, fixture tests | Pending |
| 7 | Completion | Close or merge, safe cleanup, optional remote branch pruning, retries | Pending |
| 8 | Dedicated plans repositories | Managed clones, namespacing, rediscovery on another machine | Pending |
| 9 | Skills, docs, release | Claude Code/OpenCode skills, CI, documentation, portable binaries | Pending |

## Milestone exit criteria

1. Help works. Tests drive fresh CLI instances with real Git commits and pushes
   and a persistent, stateful fake provider. JSON failures have a stable envelope.
2. `init` handles root/nested invocation, explicit config paths, existing config,
   missing Git, and unsupported configuration with structured errors.
3. An agent can edit the returned plan path without affecting its implementation
   checkout. Duplicate/ambiguous identities and worktree collisions are explicit.
4. Create/edit/publish/revise/republish produces one review. No-op publishes,
   interrupted operations, and non-fast-forward pushes are covered.
5. Feedback, changes requests, approval thresholds, review dismissal/supersession,
   draft readiness, and stale approvals are tested through complete commands.
6. Production GitHub replaces the fake without lifecycle changes. Fixture tests
   cover review states, approvals, pagination, threads, malformed responses, and
   authentication/API errors.
7. Both retention modes, repeated completion, pruning opt-in, and partial failures
   are covered. Dirty worktrees are preserved.
8. Separate repositories, identity collisions, fresh clones, deleted caches,
   pruned remote branches, and both retention modes are covered.
9. Both agents complete a manual GitHub workflow; documented release checks pass.

## Milestone 1 verification

Go 1.27.1 is pinned through mise. `mise run fmt`, `mise run vet`, `mise run test`,
and `mise run build` pass. Tests use Testify assertions, real temporary Git
repositories and bare remotes, and a stateful fake review provider. Coverage
includes linked-worktree discovery, preservation of unrelated files, review
history across revisions, process cancellation, JSON usage errors, and a compiled
binary smoke test. Lifecycle commands begin in milestone 2.

## Release workflow

`init` → `create` → edit Markdown/Mermaid → `publish` → human feedback → revise
and republish → current-head approval → `context` permits implementation →
implement in implementation checkout → `complete`.

Normal tests exercise real CLI/configuration/filesystem/Git and a stateful fake
review provider. They do not access GitHub or the network. GitHub translation is
tested separately at the process boundary. Live GitHub verification is opt-in.

## Out of scope

No server, database, custom review UI, LLM APIs, plan validation/checks, hooks,
policy engine, provider plugins, non-GitHub production providers, reviewer
assignment, multi-implementation-repository plans, or public Go API.
