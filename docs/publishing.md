# Publishing

```sh
planctl publish --json
planctl publish --plan add-sso --json
```

Publishing drives GitHub through the locally installed `gh` CLI.

## Plan selection

Use `--plan <slug>` to choose a plan. Without it, use the current plan branch or
the only local plan branch. Multiple candidates produce an error. The plan must
have its managed worktree available.

## What publish does

1. Resolve `origin` and find an existing review.
2. Check that the branch history and index contain only plan changes.
3. Commit the plan if it changed, then push that commit without force.
4. Create a ready-for-review review, or reuse the existing review.

The caller's implementation checkout is untouched. Other untracked files in the
plan worktree stay untracked. Unrelated staged files, implementation commits, and
merge commits are rejected. Markdown content is not validated.

`origin` must have the same single fetch and push URL. Git settings that would
mirror branches or push extra tags are disabled for this push.

## Retries

- No changes: reuse the commit and review.
- Push fails: keep local commits for a retry; never force-push.
- Review creation fails: look it up again in case only the response was lost.
- Closed or merged review: stop rather than reopen it or create a replacement.
- Conflicting or ambiguous review identity: stop rather than guess.

Each review description contains a versioned metadata block with the plan slug
and implementation repository. Republish preserves the description and feedback.

## Review gate

`planctl review` publishes the plan and then blocks until the review reaches a
decision:

```sh
planctl review --json
planctl review --plan add-sso --poll 5s --timeout 30m
```

It returns when `implementation.allowed` is `true` (approved), or when the review
has `changes_requested` (so the agent can revise and run `review` again). A
closed or merged review is an error. Progress goes to stderr; stdout stays a
single JSON document. Use `--timeout` to bound the wait; the default waits
indefinitely and honors Ctrl-C.

## Review lifecycle

`status`, `feedback`, and `context` read the review from GitHub:

- `status` shows the lifecycle state: `draft`, `in_review`, `changes_requested`,
  `approved`, `closed`, or `merged`, plus approvals and feedback count.
- `feedback` lists review comments (author, path, line, body).
- `context` is for agents: it reports `implementation.allowed` and machine-readable
  `blocked_reasons`.

Implementation is allowed only when the review is open and not a draft, enough
distinct approvals exist for the published revision, no active changes request
remains, and the local plan matches what was published. Comment-only reviews do
not erase decisions; dismissed reviews do. Approvals of an older revision do not
count after republishing.

## Completion

`complete` finishes the plan lifecycle once implementation is done:

- `pr-only` retention closes the review without merging.
- `repository` retention merges the plan into the base branch.

Completion requires the review to be approved and ready. `--prune-remote` (off
by default) deletes the remote plan branch afterwards. A clean managed worktree
is removed after completion; one with local work is kept. Re-running `complete
--plan <slug>` after finalization is idempotent and retries branch pruning.
