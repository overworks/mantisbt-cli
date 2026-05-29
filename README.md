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

### Read

```bash
mantisbt-cli auth whoami
mantisbt-cli issues list --page-size 20 --page 1
mantisbt-cli issue get 1234
mantisbt-cli --json issue get 1234
mantisbt-cli --version
```

#### Filtering `issues list`

```bash
# Server-side filters (passed to the API):
mantisbt-cli issues list --project 3
mantisbt-cli issues list --filter assigned          # assigned|reported|monitored|unassigned
mantisbt-cli issues list --select id,summary,status

# Client-side filters (applied to the fetched page only — combine with --page-size):
mantisbt-cli issues list --status resolved
mantisbt-cli issues list --search login
```

The MantisBT REST API only supports `project_id` and `filter_id` as
server-side issue filters, so `--status` and `--search` are applied locally to
the issues returned by the current page. Raise `--page-size` if you need them to
consider more issues.

### Write

```bash
# Create an issue (summary, description, project, category are required).
mantisbt-cli issue create \
  --summary "Login fails on Safari" \
  --description "Steps to reproduce ..." \
  --project 3 --category General \
  --priority high

# Update only the fields you pass (partial update).
mantisbt-cli issue update 1234 --status resolved --handler alice

# Add a note (optionally private).
mantisbt-cli issue note add 1234 --text "Confirmed, looking into it" --private

# Delete an issue or note (prompts for confirmation; --yes skips it).
mantisbt-cli issue delete 1234 --yes
mantisbt-cli issue note delete 1234 5 --yes
```

`--project` accepts a numeric id or a project name. Destructive commands prompt
for confirmation on an interactive terminal and refuse on a non-interactive one
unless `--yes` is given.

Global flags (`--url`, `--token`, `--json`) go before the subcommand. Use
`--json` to print the raw JSON response instead of the formatted output.

## Development

```bash
make test           # go test ./...
make vet
make fmt
```

### Releases

Releases are automated with [GoReleaser](https://goreleaser.com/). Pushing a
`v*` tag triggers the `release` GitHub Actions workflow, which cross-compiles
binaries for linux, macOS, and Windows (amd64 + arm64), builds archives plus a
checksums file, and publishes them to a GitHub Release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

To dry-run the whole release locally without publishing:

```bash
goreleaser release --snapshot --clean   # artifacts land in dist/
```

`make release` is also available for a quick cross-compile without GoReleaser.
A Homebrew tap is a natural follow-up for public distribution.

## Layout

```
main.go                     entry point, version stamping
internal/config/            flag + environment resolution
internal/mantis/            HTTP client for the REST API
internal/cli/               subcommand routing and output formatting
```
