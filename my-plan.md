# Plan: Verbose Logging

## Goal

Expand `--verbose` from exec-only diagnostics into a structured debug log that
covers the whole publish/review lifecycle, without changing default output,
exit codes, or the JSON contract.

## Current state

- `--verbose` exists (`cmd/planctl/root.go:54`) and wraps the executor with
  `process.Logging` (`internal/process/exec.go:61`), which writes `exec ...`
  lines to stderr.
- Nothing else is logged: plan resolution, workspace setup, publish steps,
  poll cycles, and evaluation decisions are silent.
- Config (`.planctl.yaml`) holds settings only; no `verbose` setting yet.

## Design

1. **Logger abstraction** — add a small `internal/output` logger (or extend
   `Renderer`) with a `Verbose(w io.Writer, enabled bool)` constructor and a
   `Debugf(format, args...)` method. When disabled it is a no-op, so call
   sites stay unconditional and cheap.
2. **Plumbing** — thread the logger through `Dependencies` in
   `cmd/planctl/root.go` into `plan.Service` (struct field, nil = disabled).
   Keep `process.Logging` as the exec-layer detail; the new logger covers
   plan-level events above it.
3. **Log points** (all to stderr, prefixed `debug:`):
   - plan resolution: source of files (explicit args vs. auto-detected
     changes), resolved file list, branch/base choices
   - publish: workspace path, review create/update decision, PR number/URL
   - wait loop: each poll's status, approvals, feedback count, blocked
     reasons (complements the existing `Status ...` line)
   - config: which config file was loaded (or none)
4. **Config option** — add optional `verbose: true` to `.planctl.yaml`,
   OR-ed with the flag, per the "settings only" rule.
5. **Safety rules** (documented on the logger):
   - never log environment, tokens, or file contents
   - stderr only; stdout and JSON payloads unchanged
   - `--json --verbose` must still emit valid JSON on stdout

## Out of scope

- Log levels beyond on/off, log files, timestamps/colors.
- Changing human (non-verbose) output.
