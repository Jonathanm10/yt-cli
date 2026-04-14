# AGENTS.md

This repository allows AI-assisted contributions, but the public contributor surface is intentionally small and repo-focused.

## Expectations

- Keep `yt-cli` focused on hosted YouTrack operations.
- Prefer small, reviewable diffs.
- Do not add new dependencies without a strong reason.
- Preserve machine-readable JSON output behavior unless the change explicitly updates the contract.
- Run fresh verification before claiming completion.

## Build and test

```bash
go test ./...
go build ./cmd/yt-cli
```

## Public/private repo surface

- `AGENTS.md` is intentionally public and provides lightweight contributor guidance.
- Public AI contributor assets also live in `.codex/agents/`, `.codex/prompts/`, and `.codex/skills/`.
- Local `.codex` runtime/state files and all `.omx/` workflow state are **not** part of the public repo surface.

## Docs-first rule for public changes

If a change affects installation, authentication, contributor workflow, or release readiness, update the relevant docs in the same change.
