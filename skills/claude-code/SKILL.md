---
name: planctl
description: Collaborative review of coding-agent implementation plans before implementation begins. Use when asked to implement a significant or non-trivial change.
---

# planctl

`planctl` routes implementation plans through a GitHub pull request so the team
can review them before any code is written. Git stores the plan; GitHub hosts the
review; you provide the intelligence.

## When to use

Use this skill when asked to implement a significant change, a feature, or
anything a reviewer should sign off on before coding starts. Skip it for trivial
one-line fixes.

## Workflow

### 1. Create a plan

```sh
planctl create "<short title>" --json
```

The response contains `plan.absolute_path`. Write the Markdown plan there
(Mermaid is fine). Do not create or edit implementation files yet.

### 2. Submit it for review and wait

```sh
planctl review --json
```

This publishes the plan as a GitHub pull request and then blocks. It returns
only when the review reaches a decision:

- `implementation.allowed == true` -> the plan is approved; go to step 4.
- `blocked_reasons` includes `changes_requested` -> go to step 3.

While `planctl review` is running, do not write implementation code and do not
run other commands.

### 3. Handle requested changes

```sh
planctl feedback --json
```

Address each comment in `plan.absolute_path`, then run `planctl review --json`
again (step 2).

### 4. Implement once approved

Only when `planctl review` reports `implementation.allowed == true` do you begin
implementation, in the repository's normal working checkout (not the plan
worktree).

### 5. Complete after implementation

When the implementation is done and merged:

```sh
planctl complete --json
```

## Notes

- `planctl review` is the gate: do not implement until it says
  `implementation.allowed` is `true`.
- If `planctl` reports `ambiguous_plan`, pass `--plan <slug>`.
- Plan edits happen in the path from `plan.absolute_path`, which lives in a
  managed Git worktree outside your normal checkout. Ensure your editor or agent
  workspace can write to that path.
- A `/planctl` slash command is available for the user to run planctl commands
  directly (`/planctl status`, `/planctl review`, and so on).
