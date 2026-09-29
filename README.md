# planctl

<p align="center">
  <img src="docs/assets/logo.png" alt="planctl logo" width="400" />
</p>

Send files to a GitHub pull request for review and loop on the feedback until it's approved.

`/planctl plan.md` publishes `plan.md` as a PR, prints the URL, and waits. When someone comments, it returns the comments so your agent can revise and re-run — repeat until the review is approved. Point it at a plan, a folder, several files, or nothing (auto-detects your changes).

## Quick start

```sh
gh auth login
planctl install --agent opencode   # install the /planctl command, then restart OpenCode
```

In your repo:

```
/planctl plan.md
```

No config file is required. `planctl init` writes one only if you want to customize the defaults.

## Usage

`planctl [files...] [flags]`

- Pass files, folders, or nothing (auto-detects changed files).
- `--reviewer <handle>` (repeatable) requests reviewers — every requested reviewer must approve. With reviewers, the agent asks you to **apply or ignore** each comment; without reviewers, it revises automatically.
- `--title` and `--base` override the defaults; `--poll`/`--timeout` bound the wait; `--json` for agents.

## Commands

| Command | What it does |
|---|---|
| `planctl [files...]` | Publish files (or your changes), print the PR, loop until approved |
| `planctl init` | Write a `.planctl.yaml` (optional) |
| `planctl install` | Install the `/planctl` command for an agent |
| `planctl uninstall` | Remove it |
| `planctl version` | Print the version |

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | sh
curl -fsSL https://raw.githubusercontent.com/mkramb/planctl/master/install.sh | PREFIX="$HOME/.local/bin" sh
```

`PREFIX` chooses the directory, `PLANCTL_VERSION` pins a version. Requires `git` and `gh`.

## Documentation

- [Architecture](docs/architecture.md) — how publishing, review, and approval work
- [Development](docs/development.md) — building, testing, and releasing

## License

[MIT](LICENSE)
