# Validation Status

## Current state

`yt-cli` has passing local build and test verification.

Verified locally:

```bash
go test ./...
go build ./cmd/yt-cli
```

## Hosted validation

**Status:** pending / incomplete

The project still needs a recorded hosted YouTrack smoke pass covering:
- interactive auth against a real hosted instance
- issue view
- issue search
- issue create
- issue transition
- issue assign

Until that is complete, the repository should be treated as a **public preview** rather than a fully validated general release.
