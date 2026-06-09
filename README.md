# yt-cli

`yt-cli` is a standalone CLI for working with YouTrack from the terminal and automation, whether your instance is on YouTrack Cloud or self-hosted.

> **Status:** public preview. Core local build/test coverage is in place, but recorded live validation against real YouTrack instances is still in progress. See [docs/validation.md](docs/validation.md).

## Why this exists

YouTrack is great in the browser, but automation and terminal workflows often need a simple, scriptable interface for:

- authenticating once and reusing a profile
- viewing issues as JSON
- searching issues with native YouTrack queries
- creating and updating issues from scripts or agents
- adding comments, assigning issues, transitioning states, and uploading attachments

`yt-cli` is intentionally **YouTrack-only**. It does not include repo automation, planning, or agent-specific behavior.

## Current features

- YouTrack profiles with separate local token storage
- browser-assisted `auth login`
- non-interactive auth via `--token-stdin` and `YTCLI_TOKEN`
- versioned JSON output by default
- `issue` commands for view/search/create/update/comment/transition/assign/attach
- narrow typed custom-field support for `issue create`/`issue update` covering `SingleEnumIssueCustomField`, `MultiEnumIssueCustomField`, `StateIssueCustomField`, and `TextIssueCustomField`
- `workitem view` compatibility alias for read-only viewing
- `--raw` escape hatch for upstream-style payloads
- `--json-errors` for machine-readable failure handling

## Install

Requires Go 1.26+.

Install directly from GitHub:

```bash
go install github.com/Jonathanm10/yt-cli/cmd/yt-cli@latest
```

Or build locally:

```bash
go test ./...
go build -o ./bin/yt-cli ./cmd/yt-cli
```

After install, start with:

```bash
yt-cli --help
yt-cli issue --help
yt-cli issue create --help
```

## Quickstart

Use your own YouTrack base URL in the examples below. Both YouTrack Cloud URLs such as `https://your-instance.youtrack.cloud` and self-hosted URLs such as `https://youtrack.example.com` are supported.

### 1. Authenticate

```bash
./bin/yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud
```

For non-interactive environments:

```bash
printf '%s' "$YT_TOKEN" | ./bin/yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud --token-stdin
```

### 2. Verify auth

```bash
./bin/yt-cli auth status --profile sandbox
```

### 3. Use the CLI

```bash
./bin/yt-cli --help
./bin/yt-cli issue --help
./bin/yt-cli project list --profile sandbox
./bin/yt-cli issue view ABC-123 --profile sandbox
./bin/yt-cli issue search --query 'in:ABC #Unresolved' --top 5 --skip 0 --profile sandbox
```

## Command examples

Create an issue:

```bash
./bin/yt-cli issue create \
  --profile sandbox \
  --project ABC \
  --summary 'Example issue' \
  --description 'Created from yt-cli'
```

For custom fields on `issue create` and `issue update`, `yt-cli` currently supports a narrow typed path for `SingleEnumIssueCustomField`, `MultiEnumIssueCustomField`, `StateIssueCustomField`, and `TextIssueCustomField`. For multi-value enum fields, repeat the same `--field` flag for each value, for example `--field Platform=Android --field Platform=iOS`. Broader custom-field classes are still out of scope for this preview, and board targeting is not supported — choose the destination project explicitly.

Add a comment:

```bash
./bin/yt-cli issue comment ABC-123 --text 'Investigating now' --profile sandbox
```

Assign an issue:

```bash
./bin/yt-cli issue assign ABC-123 --user jane.doe --profile sandbox
```

Raw output mode:

```bash
./bin/yt-cli issue view ABC-123 --profile sandbox --raw
```

JSON errors:

```bash
./bin/yt-cli issue view DOES-NOT-EXIST --profile sandbox --json-errors
```

## Documentation

- [Installation](docs/install.md)
- [Authentication](docs/auth.md)
- [Validation status](docs/validation.md)
- [Release checklist](docs/releasing.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Releases

- Every successful push to `main` publishes an automatic GitHub **prerelease** with generated release notes and attached CLI binaries.
- These prereleases are intended to show the evolution of `main` and make regressions easier to spot during preview development.
- Stable semver releases remain a **manual maintainer action** and should only be cut when you want a promoted version.

## Development

```bash
go test ./...
go build -o ./bin/yt-cli ./cmd/yt-cli
```

The baseline CI workflow runs the same commands.

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR.

## Security

Please do **not** open public issues for security-sensitive reports. Use the process in [SECURITY.md](SECURITY.md).

## AI contributor tooling

This repo keeps a public `AGENTS.md` and selected public `.codex/` assets (`agents/`, `prompts/`, `skills/`) for AI-assisted contributors.

Local `.codex` state files and all `.omx/` workflow state are **not** part of the public project surface and are ignored from version control.

## License

This repository is licensed under the [MIT License](LICENSE).
