# Authentication

`yt-cli` uses a base-URL-driven auth flow for the current preview release. It is intended to work with both YouTrack Cloud and self-hosted YouTrack instances.

## Interactive login

```bash
./bin/yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud
```

The CLI opens your YouTrack authentication page in the browser so you can complete SSO and create a permanent token. Paste the token into the terminal when prompted.

Replace the example URL with your own YouTrack base URL. For example, that may be a YouTrack Cloud URL such as `https://your-instance.youtrack.cloud` or a self-hosted URL such as `https://youtrack.example.com`.

## Non-interactive login

```bash
printf '%s' "$YT_TOKEN" | ./bin/yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud --token-stdin
```

## Environment override

```bash
YTCLI_TOKEN="$YT_TOKEN" YTCLI_BASE_URL="https://your-instance.youtrack.cloud" ./bin/yt-cli issue view ABC-123
```

Environment tokens are never persisted locally.

## Current security note

For the current preview release, stored tokens are kept in separate local files with restrictive permissions. If you prefer not to persist local token material, use `YTCLI_TOKEN` or `--token-stdin`.
