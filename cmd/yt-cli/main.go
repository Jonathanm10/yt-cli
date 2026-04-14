package main

import (
	"context"
	"os"

	"github.com/Jonathanm10/yt-cli/internal/cli"
)

func main() {
	app := cli.NewApp(cli.Options{
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Stdin:     os.Stdin,
		Env:       os.Environ(),
		ConfigDir: os.Getenv("YTCLI_CONFIG_DIR"),
	})
	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
