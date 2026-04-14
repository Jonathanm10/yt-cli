# Installation

## Install from GitHub

```bash
go install github.com/Jonathanm10/yt-cli/cmd/yt-cli@latest
```

## Local build

```bash
go test ./...
go build -o ./bin/yt-cli ./cmd/yt-cli
```

Run it with:

```bash
./bin/yt-cli --help
./bin/yt-cli auth --help
./bin/yt-cli issue create --help
```

## Optional local PATH install

```bash
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/yt-cli" ./cmd/yt-cli
export PATH="$HOME/.local/bin:$PATH"
```

## Requirements

- Go 1.26+
- network access to your YouTrack instance
