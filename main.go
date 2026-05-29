package main

import (
	"os"

	"github.com/overworks/mantisbt-cli/internal/cli"
)

// These are overridden at build time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.Version = version
	cli.Commit = commit
	cli.Date = date
	os.Exit(cli.Main(os.Args[1:]))
}
