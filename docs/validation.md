# Validation Status

## Current state

`yt-cli` has passing local build and test verification.

Verified locally:

```bash
go test ./...
go build ./cmd/yt-cli
```

## Live instance validation

**Status:** pending / incomplete

`yt-cli` is designed to work against both YouTrack Cloud and self-hosted deployments, but the project still needs recorded smoke-pass coverage against real YouTrack instances. Interactive browser-assisted auth has only been documented against YouTrack Cloud so far, and broader self-hosted coverage is still unrecorded.

The remaining validation work includes:
- interactive auth against a real YouTrack Cloud instance
- issue view
- issue search
- issue create
- issue transition
- issue assign

Until that is complete, the repository should be treated as a **public preview** rather than a fully validated general release.
