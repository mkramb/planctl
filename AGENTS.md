# AGENTS.md

Guidance for coding agents working in this repository.

## What this is

`planctl` sends files (a Markdown plan, code, a folder, or all your changes) to a
GitHub pull request for review, then blocks until the review reaches a decision.
When feedback comes back, the agent revises and re-runs until approved.

## Commands

- `planctl [files...]` — publish files (or auto-detected changes), print the PR URL, loop until approved.
- `planctl init` — write an optional `.planctl.yaml`.
- `planctl install` / `planctl uninstall` — install/remove the `/planctl` agent command.
- `planctl version` — print the version.

Key flags: `--reviewer <handle>` (repeatable), `--title`, `--base`, `--poll`,
`--timeout`, `--json`.

## Review loop

`planctl` returns when any feedback is submitted or the review is approved:

- **No reviewers** — revise the files automatically and re-run.
- **`--reviewer`** — ask the user to apply or ignore each comment, then re-run.

The review is approved when the required approvals exist for the current
revision and no change request remains. The required count is the number of
requested reviewers, or one when none are requested.

## Layout

- `cmd/planctl/` — the CLI (package `main`) and its co-located tests.
- `internal/` — `config`, `git`, `github` (provider + model + in-memory mock), `output`, `plan`, `process`.
- `integrations/` — the embedded `/planctl` slash commands.
- `docs/` — `architecture.md`, `development.md`.

## Build, test, lint

```sh
mise run build              # bin/planctl
mise run test               # unit tests (no git/gh/network)
mise run test-integration   # integration tests (real Git, in-memory GitHub)
mise run validate           # fmt + vet + lint
```

## Conventions

- GitHub is the only review provider; there is no plugin/provider abstraction.
  The integration mock is `internal/github/mock.go` (an in-memory `gh` fake).
- `internal/process` is the only place that execs external programs.
- Commands emit versioned JSON on `--json`; human output is text. Exit codes:
  0 ok, 1 failure, 2 bad arguments, 130 canceled.
- Config is optional and holds settings only — never review state.
- Never commit secrets or credentials.
