# Architecture

## Overview

`planctl` sends files to a GitHub pull request for review and waits for a
decision. It drives the locally installed `git` and `gh` CLIs; Git and GitHub
hold all state, `.planctl.yaml` holds only optional config, and there is no
local database or server.

GitHub is the only review provider. The production code path in
`internal/github` is what everything (including tests) exercises; the
integration-test mock is a stateful in-memory `gh` fake that lives next to the
provider in `internal/github/mock.go`.

## Structure

```
cmd/planctl/     the CLI (package main) and its co-located tests
internal/
  config/        config discovery, defaults, validation, init
  git/           the user's real git
  github/        the GitHub provider + review model + in-memory mock
  output/        typed results rendered as text or JSON
  plan/          file selection, worktrees, and the review lifecycle
  process/       runs external programs; the only place that execs
integrations/    the /planctl slash commands shipped to agents
```

The flow is: Cobra command -> operation -> git / github -> typed result -> text
or JSON output.

## Configuration

`planctl` works with no configuration. When no `.planctl.yaml` is present it
uses the defaults below; `planctl init` writes a file you can customize.

```yaml
version: 1
review:
  provider: github      # always github; any other value is an error
branch:
  base: ""              # default branch when empty
  pattern: "review/{slug}"
pull_request:
  title: "Review: {title}"
```

Config is searched upward from the working directory to the Git worktree root.
The nearest file wins; invalid files are errors, not silently skipped. `--config`
selects an explicit file. Configuration holds settings only, never review IDs or
status.

## File selection and publishing

- Arguments may be files, folders (expanded to their tracked and untracked,
  non-ignored files), or omitted entirely (auto-detect changed files from
  `git status`).
- The branch is `branch.pattern` (default `review/{slug}`), where the slug comes
  from the first selected file's base name.
- The selected files are copied into an isolated worktree at the base commit,
  committed, and pushed. Files previously published but no longer selected are
  removed. The caller's checkout is never touched.

Worktrees live at:

```text
<user cache>/planctl/worktrees/<repo-name>-<repo-key>/<slug>/
```

The key is a hash of Git's shared repository directory. Per-slug OS locks
prevent concurrent writes; republishing reuses the same worktree and review.

## Review loop and approval

`planctl` publishes, prints the pull request URL, and blocks until a decision.
It returns when any feedback is submitted (a comment or a change request), or
when the review is approved. Comments tied to an older revision are filtered
out, so each round only surfaces current feedback.

- **No reviewers**: the agent revises the files automatically and re-runs.
- **With `--reviewer`**: the agent asks the user to apply or ignore each comment.

The review is approved when it is open, the required approvals exist for the
current revision, and no change request remains. The required count is the
number of requested reviewers, or one when none are requested ("just us"). A new
publish pushes a new revision, so previous approvals no longer count. Comment-
only reviews do not erase decisions; dismissed reviews do.

## Retries

- No changes: reuse the commit and review.
- Push fails: keep local commits for a retry; never force-push.
- Review creation fails: look it up again in case only the response was lost.
- Closed or merged review: stop rather than reopen or replace it.
- Conflicting or ambiguous review identity: stop rather than guess.

Each review description carries a versioned metadata block with the slug and
reviewer list; republishing preserves the description and feedback.
