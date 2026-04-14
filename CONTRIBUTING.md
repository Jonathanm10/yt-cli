# Contributing to yt-cli

Thanks for your interest in contributing.

## Before you start

- Read the README and relevant docs in `docs/`.
- Keep the project focused on hosted YouTrack CLI behavior.
- Open an issue first for large changes or behavior changes.

## Local setup

Requirements:
- Go 1.26+

Build and test:

```bash
go test ./...
go build -o ./bin/yt-cli ./cmd/yt-cli
```

## Development expectations

- Keep diffs small and easy to review.
- Add or update tests when behavior changes.
- Keep stdout machine-readable for command results.
- Keep warnings/prompts/debug output on stderr.
- Update docs when changing auth, install, output contracts, or contributor workflow.

## Pull requests

Please include:
- a short problem statement
- what changed
- how you tested it
- any known limitations or follow-ups

## AI-assisted contributions

AI-assisted contributions are welcome. If you use an AI tool, review the result carefully and make sure the final change matches the repo’s documented behavior and scope.

Public AI contributor assets live in:
- `AGENTS.md`
- `.codex/agents/`
- `.codex/prompts/`
- `.codex/skills/`

Local workflow state under `.omx/` and local `.codex` runtime files are not part of the public repo surface.
