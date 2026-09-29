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

### 2. Publish for review

```sh
planctl publish --json
```

This commits the plan, pushes a branch, and opens (or updates) the review.
When a review is already open, republishing updates it instead of duplicating it.

Stop here and let the team review. Do not begin implementation while the plan is
awaiting review.

### 3. Check status when continuing

```sh
planctl context --json
```

Read the response:

- `implementation.allowed` must be `true` before you write implementation code.
- `blocked_reasons` explains why implementation is not yet allowed.

### 4. Handle requested changes

```sh
planctl feedback --json
```

Address each comment, then republish:

```sh
planctl publish --json
```

### 5. Implement once approved

Only begin implementation when `implementation.allowed == true`. Implement in the
implementation repository's working checkout, not the plan worktree.

### 6. Complete after implementation

When the implementation is done and merged:

```sh
planctl complete --json
```

## Notes

- If `planctl` reports `ambiguous_plan`, pass `--plan <slug>`.
- Plan edits happen in the path from `plan.absolute_path`, which lives in a
  managed Git worktree outside your normal checkout. Ensure your editor or agent
  workspace can write to that path.
- Never begin implementation while `implementation.allowed` is `false`.
