# Architecture

## Overview

```text
              +------------+
              | coding     |
              | agent/human|
              +-----+------+
                    |
                    v
              +------------+
              |  planctl   |
              +--+------+--+
                 |      |
                 v      v
             +-----+  +-----------------+
             | Git |  | ReviewProvider  |
             +-----+  +-------+---------+
                              |
                       +------+------+
                       v             v
                  +---------+   +--------+
                  |  GitHub |   |  mock  |
                  |  (prod) |   | (tests)|
                  +---------+   +--------+
```

Git and the review provider hold the state. `.planctl.yaml` holds only config.
There is no local database or server.

## Packages

The flow is: Cobra command -> operation -> Git / ReviewProvider -> typed result
-> text or JSON output.

- `cmd/planctl` - startup and exit code only.
- `internal/cli` - commands, flags, wiring.
- `internal/process` - runs external programs; the only place that execs.
- `internal/git` - the user's real `git`.
- `internal/review` - provider-neutral review types and interface.
- `internal/output` - shared results rendered as text or JSON.
- `internal/github` - GitHub details and `gh` calls (to be built).

## Rules

- GitHub is the only production provider in V1. No plugin framework.
- GitHub-specific code stays in `internal/github`.
- Everything is reproducible from config + Git + the review provider.

## Plan workflow

- Each plan gets its own managed Git worktree, so plan edits never touch the
  user's working checkout. The plan branch starts from the base branch, not the
  user's current branch.
- Plan identity = repository + slug (e.g. `add-sso` -> branch `plan/add-sso`,
  file `.plans/add-sso.md`). Commands take `--plan <slug>`; if no plan can be
  picked automatically, that's an error.
- `publish` commits only the plan file, pushes, and creates or updates one
  review per plan. Reviews are created ready for review, not drafts.

## Reviews and approval

- Reviewers, approvals, and requested changes all happen on GitHub.
  `planctl` only reads them.
- Implementation is allowed when: the review is open and not a draft, enough
  approvals exist for the current revision, no unresolved change request, and
  the local plan matches what was published. A new publish needs fresh approvals.

## Completion

- `pr-only` retention: `complete` closes the review without merging.
- `repository` retention: `complete` merges the plan into the base branch.
- `--prune-remote` (off by default) deletes the remote plan branch afterwards.
  Safe to rerun; already-finished steps are skipped.
