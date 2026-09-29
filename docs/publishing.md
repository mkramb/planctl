# Publishing

The workflow currently runs through the integration-test provider. The standalone
binary will support it when the GitHub adapter is added.

```sh
planctl publish --json
planctl publish --plan add-sso --json
```

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
