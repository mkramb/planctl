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

Git and GitHub hold the state. `.planctl.yaml` holds only config.
There is no local database or server.

`ReviewProvider` in the diagram is a testing seam, not a plugin system: the
production implementation is GitHub, and the "mock" branch is an in-memory fake
that lets integration tests run offline.

## Packages

The flow is: Cobra command -> operation -> Git / ReviewProvider -> typed result
-> text or JSON output.

- `cmd/planctl` - startup and exit code only.
- `internal/cli` - commands, flags, wiring.
- `internal/config` - config discovery, defaults, validation, and initialization.
- `internal/plan` - plan identity, templates, and managed worktrees.
- `internal/process` - runs external programs; the only place that execs.
- `internal/git` - the user's real `git`.
- `internal/review` - review types and the `Provider` interface (a test seam).
- `internal/output` - shared results rendered as text or JSON.
- `internal/github` - GitHub details and `gh` calls.

## Rules

- planctl supports GitHub only. The `Provider` interface exists so tests can use
  a fake; it is not a plugin system.
- GitHub-specific code stays in `internal/github`.
- Everything is reproducible from config + Git + GitHub.

## Plan workflow

- Each plan gets its own managed Git worktree, so plan edits never touch the
  user's working checkout. The plan branch starts from the base branch, not the
  user's current branch.
- Plan identity = repository + slug (e.g. `add-sso` -> branch `plan/add-sso`,
  file `.plans/add-sso.md`). Commands take `--plan <slug>`; if no plan can be
  picked automatically, that's an error.
- `publish` commits only the plan file, pushes, and creates or updates one
  review per plan. Reviews are created ready for review, not drafts.

Worktrees live at:

```text
<user cache>/planctl/worktrees/<repo-name>-<repo-key>/<slug>/
```

The key is a hash of Git's shared repository directory. Worktrees from one clone
reuse the same plan location; separate clones get separate locations. Commands
return the full editable path. Per-plan OS locks prevent concurrent writes and
release when the process exits. Unpublished edits are never discarded.

`create` can retry after a branch or worktree was created but the template was
not written. It refuses unrelated paths and branches containing other work.
If Git still lists a missing worktree, repair that registration before retrying.

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
