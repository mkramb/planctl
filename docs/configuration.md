# Configuration

Run `planctl init` in an existing Git repository. It creates:

```yaml
version: 1

plan:
  retention: pr-only
```

Existing files are never overwritten. Init works without a commit, remote, or
GitHub authentication. In a linked worktree, it writes to that worktree's root.

## Finding configuration

Configuration is searched upward from the working directory, stopping at the
Git worktree root. The nearest `.planctl.yaml` wins. Invalid files cause an error;
they are not skipped.

Use an explicit path to override discovery:

```sh
planctl --config /path/to/repository/.planctl.yaml init
```

Relative paths are resolved from the working directory. The file's directory
must already exist inside a Git worktree. Its repository becomes the
implementation repository, even when the command runs elsewhere.

## Defaults

```yaml
version: 1
review:
  provider: github
  required_approvals: 1
repositories:
  plans: current
plan:
  directory: .plans
  retention: pr-only
branch:
  pattern: "plan/{slug}"
pull_request:
  draft: false
  title: "Plan: {title}"
```

`create` stays offline. With no `branch.base`, it uses the cached `origin/HEAD`,
or the sole available `main`/`master` branch. If the choice is unclear, set
`branch.base` explicitly. A cached `origin/<base>` takes priority over the local
branch. The base must have a commit; fetch it first if needed.

- `required_approvals` must be a positive integer.
- `repositories.plans` accepts `current` or `owner/repository`.
- `retention` accepts `pr-only` or `repository`.
- `directory` must stay inside the repository, outside `.git`.
- Unknown fields and unsupported versions/providers are errors.
- Configuration holds settings, not review IDs or current plan status.

`init` and same-repository `create` are available. Publishing is tested through
the fake provider. The GitHub adapter, dedicated plans repositories, and completion
arrive in later stages.
