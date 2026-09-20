# mantisbt-cli

[![CI](https://github.com/overworks/mantisbt-cli/actions/workflows/ci.yml/badge.svg?branch=0.x)](https://github.com/overworks/mantisbt-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/overworks/mantisbt-cli)](https://github.com/overworks/mantisbt-cli/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/overworks/mantisbt-cli)](https://goreportcard.com/report/github.com/overworks/mantisbt-cli)
[![License: MIT](https://img.shields.io/github/license/overworks/mantisbt-cli)](LICENSE)

A small command line client for the MantisBT REST API, written in Go and
distributed as a single static binary. Standard library only — no third-party
dependencies, no runtime required on the target machine.

## Install

### Install script (Linux, macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/overworks/mantisbt-cli/0.x/install.sh | sh
```

The script detects your OS and architecture, downloads the matching binary from
the latest release, verifies its SHA-256 checksum, and installs it to
`~/.local/bin`. Set `INSTALL_DIR` to change the location, or `VERSION` to pin a
release:

```bash
curl -fsSL https://raw.githubusercontent.com/overworks/mantisbt-cli/0.x/install.sh | VERSION=v0.1.0 sh
```

### Manual download

Grab a prebuilt archive for your platform from the
[releases page](https://github.com/overworks/mantisbt-cli/releases) (Windows
builds are published as `.zip`), extract it, and put `mantisbt-cli` on your
`PATH`.

### Build from source

```bash
make build          # -> bin/mantisbt-cli
```

Go 1.26 or newer is required. Make builds use `CGO_ENABLED=0` and `-trimpath`,
matching the release builds. Version information comes from Git: the nearest
`v*` version tag (plus commit distance and dirty state when applicable), the
full commit hash, and the commit date in UTC. The leading `v` is omitted from
the binary's version. Override `VERSION`, `COMMIT`, or `DATE` when building
from a source archive; without Git metadata they default to `dev`, `none`,
and `unknown`.

## Configuration

Point the CLI at your MantisBT instance with environment variables:

```bash
export MANTISBT_URL="https://mantis.example.com"
export MANTISBT_TOKEN="your-api-token"
```

`MANTISBT_URL` can point at the MantisBT web root, or directly at a REST API
root when the server exposes one:

```bash
export MANTISBT_URL="https://mantis.example.com/api/rest"
export MANTISBT_URL="https://mantis.example.com/api/rest/index.php"
```

The URL must be an absolute HTTP or HTTPS URL without embedded credentials,
query parameters, or a fragment. Redirects are followed only within the same
origin (scheme, host, and port) and only when the HTTP method is preserved.
For redirects to another origin, set the final server URL explicitly.

Every command also accepts explicit connection flags, which take precedence
over the environment:

```bash
mantisbt-cli --url https://mantis.example.com --token your-api-token auth whoami
```

Responses are limited to 64 MiB by default. Set the global
`--max-response-size <bytes>` before the command to change this limit. HTTP error
messages include at most 64 KiB of the response body.

## Usage

### Read

```bash
mantisbt-cli auth whoami
mantisbt-cli issues list --page-size 20 --page 1
mantisbt-cli issues list --all --project 3
mantisbt-cli issue get 1234
mantisbt-cli --json issue get 1234
mantisbt-cli --version
```

`issue get` shows the description, assignee, issue metadata, notes (including
visibility), and attachment names/sizes when present. Issue lists and write
results use the compact one-line format. `--json issue get` retains the full
API response, including attachment content if returned by the server.

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
the issues returned by the current page. Use `--all` to apply them across every
page, or raise `--page-size` to consider more issues in a single request.

When combining `--select` with local filters, include `status` for `--status`
and `summary` for `--search`. A selection that omits a required field is rejected
before contacting the server.

`issues list --all` starts at page 1 and continues until the API returns an empty
page. It cannot be combined with `--page`. When using `--select`, include `id`
so the CLI can detect repeated issues. Local filters do not affect pagination.
`--json` produces one combined `{"issues": [...]}` response.

No results are printed until all pages succeed. Repeated issue IDs or failed
pages stop the command with an error. The default request limit is 10,000 pages,
including the final empty page; adjust it with `--max-pages N`. The combined
result's compact JSON must fit within `--max-response-size` as well. The same
limit bounds the retained IDs used to detect repeated issues (8 bytes per ID,
excluding runtime overhead), even when local filters exclude them. Narrow
`--project`/`--filter`/`--select` or raise the limits for larger exports.
Pagination is not a snapshot: changing issues while listing can shift page
boundaries; repeated IDs cause an error so you can retry against stable data.

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

### Attachments

```bash
# Attach one or more local files to an existing issue.
mantisbt-cli issue file add 1234 fix.patch screenshot.png

# Raise the combined upload limit to 64 MiB (put options before file paths).
mantisbt-cli issue file add 1234 --max-upload-size 67108864 large.zip

# Compose with Git to attach a patch.
git diff > fix.patch
mantisbt-cli issue file add 1234 fix.patch

# List the attachments on an issue.
mantisbt-cli issue file list 1234

# Download one, either to a chosen path or to its own filename here.
mantisbt-cli issue file get 1234 5 --output ./patches/fix.patch
mantisbt-cli issue file get 1234 5
```

Without `--output`, the attachment is written to its own filename in the
current directory and any existing file or symbolic link is refused. Pass
`--output` to choose the destination explicitly; the destination is replaced
only after a temporary file has been written successfully. If the destination
is a symbolic link, the link itself is replaced and its target is left untouched.
On Linux and macOS, downloaded files have owner-only permissions (`0600`),
including when replacing an existing file.

Uploads accept regular files and are limited to 32 MiB of combined file data per
request by default; use `--max-upload-size <bytes>` to adjust this. Size limits
must be positive. The response limit above includes JSON and base64 overhead.
Downloads decode base64 directly to disk, and empty attachments are supported.

MantisBT returns attachment content inline as base64, so `--json` output for
`file list` includes every attachment's full contents.

`--project` accepts a numeric id or a project name. Destructive commands prompt
for confirmation on an interactive terminal and refuse on a non-interactive one
unless `--yes` is given. Piping or redirecting `yes` into a command does not
authorize deletion.

Global flags (`--url`, `--token`, `--json`, `--max-response-size`) go before the
subcommand. Use `--json` to print the raw JSON response instead of the formatted
output.

For commands with IDs and options, put the IDs first, for example
`issue update 123 --status resolved`. Unexpected arguments are rejected.
Issue, note, attachment, and stored-filter IDs must be positive decimal integers;
`--page` and `--page-size` must also be positive. `issues list --project 0`
selects all projects.

`--help` and `--version` work without connection settings. Exit codes are `0`
for success or help, `1` for API, file, or output failures, and `2` for invalid
arguments or missing configuration.

## Development

```bash
make test           # go test ./...
make vet
make fmt
```

CI checks formatting on Linux and runs vet and the test suite on Linux, macOS,
and Windows using the Go version declared in `go.mod`.

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
It uses the same build flags and covers linux, macOS, and Windows on both
amd64 and arm64, writing standalone binaries to `dist/`.
A Homebrew tap is a natural follow-up for public distribution.

## Agent skill

A distributable [Agent Skill](https://www.anthropic.com/news/skills) for driving
`mantisbt-cli` ships in [`skills/mantisbt/`](skills/mantisbt/SKILL.md). It
teaches an AI agent the commands, when to use `--json` for parsing, the issue
filters, and how to handle destructive commands safely. The format is portable
across skill-aware agents (Claude Code, Codex, Cursor, OpenCode, …).

Install it from this repo with [`skills`](https://github.com/vercel-labs/skills),
which auto-discovers the `skills/` layout and copies the skill into your agent's
config directory:

```bash
npx skills add overworks/mantisbt-cli            # install into the detected agent(s)
npx skills add overworks/mantisbt-cli --list     # preview skills without installing
npx skills add overworks/mantisbt-cli -g         # install globally (all projects)
```

To pin a branch or skill explicitly:

```bash
npx skills add https://github.com/overworks/mantisbt-cli/tree/0.x/skills/mantisbt
```

Or copy `skills/mantisbt/` into your agent's skills directory by hand (e.g.
`~/.claude/skills/`).

## Layout

```
main.go                     entry point, version stamping
internal/config/            flag + environment resolution
internal/mantis/            HTTP client for the REST API
internal/cli/               subcommand routing and output formatting
```
