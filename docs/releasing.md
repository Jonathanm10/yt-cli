# Releasing yt-cli

## Public preview release checklist

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
6. Confirm `.github/` templates and CI are present.
7. Confirm no secrets or local-only files are being published.
8. Confirm the license choice is still correct for publication.

## Notes

Until live-instance validation is complete, releases should continue to be labeled as **preview**.
