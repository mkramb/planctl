# Architecture and agreed lifecycle

## Boundaries

The execution path is Cobra → lifecycle operation → Git / ReviewProvider → typed
result → text or JSON. `cmd/planctl` contains only process startup and exit handling.
Application code lives in `internal`; no public SDK or provider registry exists.

- `cli`: Cobra commands, dependency construction, error/exit handling.
- `process`: installed executable execution, cancellation, explicit directory and
  environment, captured stdout/stderr, typed errors.
- `git`: real installed Git, independent of the review platform.
- `review`: provider-neutral review models and a narrow provider interface.
- `output`: shared versioned results and text/JSON rendering.
- Future packages: `config`, `plan`, and `github`, as their behavior is built.

Git and the review provider are authoritative. `.planctl.yaml` contains only
configuration. GitHub-specific data and all `gh` execution belong in the GitHub
adapter. There is no authoritative local state file or database.

## Review decisions

New GitHub reviews are **ready for review by default**, not drafts. Reviewers are
assigned manually on GitHub; there are no reviewer-assignment flags or settings.
Humans approve or request changes on GitHub. `planctl` reads those decisions.

Implementation is allowed only when the review is open and ready, enough distinct
effective approvals apply to the published head, no active changes request remains,
and the local plan matches the published head. A comment-only review does not
erase an approval or changes request. Dismissal and supersession must be respected.
New published revisions need fresh approval. Unknown approval state is an error.

## Identity and selection

A plan is identified by implementation repository plus slug. Commands accept
`--plan <slug>`. Otherwise use a recognized plan branch or the sole discoverable
candidate; ambiguity is an error. No current-plan setting is persisted.

Review metadata is versioned and carries plan identity. Lookup considers repository,
head branch, and metadata. Explicit lookup can find terminal reviews after their
remote branch has been deleted.

## Worktrees

Each plan uses a separate managed worktree, even when the caller already uses an
implementation worktree. The plan branch starts at the configured planning base,
not at the caller's feature branch. Creating the local branch happens in `create`;
committing, pushing, and creating the review happen in `publish`.

Proposed local layout:

```text
<OS user cache>/planctl/worktrees/<repo-name>-<repo-key>/<plan-key>/
```

The repository key is derived from the canonical Git common-directory path, so
worktrees of one clone share a plan workspace and independent clones do not.
The directory name is not public plan identity. Validate an existing directory
before reuse. An unmanaged worktree already holding the branch is a clear error.
Never force duplicate checkouts or overwrite unrelated directories.

Return repository-relative `path`, plus local `workspace_path` and `absolute_path`.
Agents edit the returned absolute path and implement in the original checkout.
Some agent environments need permission to edit outside their project directory.

Publishing commits only the selected plan and checks the branch diff for unrelated
changes. Serialize mutations per plan. Never automatically delete a dirty worktree;
unpublished files remain valuable even under a cache path.

Same-repository defaults are `plan/{slug}` and `.plans/{slug}.md`. A dedicated
plans repository uses a managed clone and namespaced defaults:

```text
plan/{implementation-owner}/{implementation-repo}/{slug}
.plans/{implementation-owner}/{implementation-repo}/{slug}.md
```

Configuration, Git, and review metadata must support fresh-machine reconstruction
of published plans. Invocation inside a managed worktree must resolve the correct
implementation repository rather than accidentally using a plans-repository config.

## Completion

`complete` is an explicit declaration that implementation has finished; V1 does
not independently verify delivery. Require approval/readiness before initiating:

- `pr-only`: close the plan review without merging.
- `repository`: merge the plan-only review into the plans repository's configured
  base, respecting GitHub protection and merge requirements.

`complete --prune-remote` optionally deletes the remote plan branch after successful
finalization. **Disabled by default**, and supported for both retention modes.
Pruning never removes the plan document from a merged base branch. An absent remote
branch counts as success. If finalization succeeds but pruning fails, report the
partial outcome and allow cleanup retry without repeating finalization.

Repeated completion of the expected terminal state is idempotent; incompatible
terminal states are conflicts. Remove managed worktrees only after successful
finalization and only when clean.
