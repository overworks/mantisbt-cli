# mantisbt-cli

A small command line client for the MantisBT REST API, written in Go and
distributed as a single static binary. Standard library only — no third-party
dependencies, no runtime required on the target machine.

## Install

Download a prebuilt binary for your platform from the releases page and put it
on your `PATH`, or build from source:

```bash
make build          # -> bin/mantisbt-cli
```

## Configuration

Point the CLI at your MantisBT instance with environment variables:

```bash
export MANTISBT_URL="https://mantis.example.com"
export MANTISBT_TOKEN="your-api-token"
```

Every command also accepts explicit connection flags, which take precedence
over the environment:

```bash
mantisbt-cli --url https://mantis.example.com --token your-api-token auth whoami
```

## Usage

```bash
mantisbt-cli auth whoami
mantisbt-cli issues list --page-size 20 --page 1
mantisbt-cli issue get 1234
mantisbt-cli --json issue get 1234
mantisbt-cli --version
```

Global flags (`--url`, `--token`, `--json`) go before the subcommand. Use
`--json` to print the raw JSON response instead of the formatted output.

## Development

```bash
make test           # go test ./...
make vet
make fmt
```

### Cross-compiled release binaries

```bash
make release VERSION=0.1.0
```

This produces static binaries in `dist/` for linux, macOS, and Windows
(amd64 + arm64). The natural next step for public distribution is
[`goreleaser`](https://goreleaser.com/) wired to GitHub Releases plus a
Homebrew tap.

## Layout

```
main.go                     entry point, version stamping
internal/config/            flag + environment resolution
internal/mantis/            HTTP client for the REST API
internal/cli/               subcommand routing and output formatting
```
