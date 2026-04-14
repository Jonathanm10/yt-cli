# Releasing yt-cli

## Automatic main prereleases

`yt-cli` publishes a GitHub **prerelease** for every successful push to `main`.

The prerelease workflow:

1. waits for the `CI` workflow to succeed on `main`
2. checks out the tested commit SHA
3. builds preview binaries for:
   - `linux/amd64`
   - `darwin/amd64`
   - `windows/amd64`
4. generates `SHA256SUMS.txt`
5. creates a GitHub prerelease with generated release notes

Prerelease tags use the format:

```text
preview-main-r<run_number>.<run_attempt>-<short_sha>
```

Example:

```text
preview-main-r42.1-a1b2c3d
```

These prereleases are meant to expose the evolution of `main` and make regressions easier to spot during preview development.

## Repository / GitHub requirements

- GitHub Actions must be enabled for the repository.
- Workflow permissions must allow `contents: write` so the release workflow can create tags and prereleases with `GITHUB_TOKEN`.

## Manual stable releases

Stable semver releases are intentionally **manual**.

Before cutting a stable release:

1. Run:
   ```bash
   go test ./...
   go build -o ./bin/yt-cli ./cmd/yt-cli
   ```
2. Review `docs/validation.md` and confirm the live-instance validation status is accurate.
3. Confirm README examples still match the CLI.
4. Confirm `.gitignore` excludes local artifacts.
5. Confirm community files exist:
   - `LICENSE`
   - `CONTRIBUTING.md`
   - `SECURITY.md`
   - `CODE_OF_CONDUCT.md`
6. Confirm `.github/` templates, CI, and prerelease workflows are present.
7. Confirm no secrets or local-only files are being published.
8. Create and push the intended semver tag manually.
9. Draft or publish the stable GitHub release from that semver tag.

## Notes

Until live-instance validation is complete, stable releases should still be treated carefully and preview prereleases remain the default way to follow ongoing development.
