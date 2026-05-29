# AGENTS.md

Guidance for AI agents working in this repository. (`CLAUDE.md` is a symlink to
this file.)

## Project

`mantisbt-cli` is a command-line client for the MantisBT REST API, written in
Go and shipped as a single static binary. **Standard library only — do not add
third-party dependencies** unless explicitly asked; keeping the dependency set
empty is a deliberate design goal.

## Commands

A Go toolchain matching `go.mod` (Go 1.26+) is required.

```bash
make build      # build ./bin/mantisbt-cli
make test       # go test ./...
make vet        # go vet ./...
make fmt        # gofmt -w .
make release    # cross-compile to dist/ (also done by GoReleaser on v* tags)
```

Before committing, code must be `gofmt`-clean and pass `go vet ./...` and
`go test ./...` — CI (`.github/workflows/ci.yml`) enforces all three.

## Layout

```
main.go                  entry point; version/commit/date stamped via -ldflags
internal/config/         flag + environment (MANTISBT_URL/MANTISBT_TOKEN) resolution
internal/mantis/         HTTP client (Get/Post/Patch/Delete) for the REST API
internal/cli/            subcommand routing, request building, output formatting
skills/mantisbt/         distributable Agent Skill (see README "Agent skill")
.goreleaser.yaml         release config; bundles the binary + skill into archives
```

## Architecture & conventions

- `cli.Main` parses global flags (`--url`, `--token`, `--json`, `--version`),
  then `route()` walks the subcommand tree and returns an `apiCall`
  (`func(*mantis.Client) (any, error)`) plus an exit code. Config is loaded and
  the client built only after a command resolves, then the result goes to
  `printResult`.
- Subcommand flags use per-command `flag.FlagSet`s. The positional id comes
  first (`issue update <id> --status ...`), because Go's `flag` package stops
  parsing at the first non-flag argument.
- Partial updates: `routeIssueUpdate` uses `fs.Visit` to send only the fields
  the user actually set.
- REST responses are decoded into `any`; formatting helpers type-assert
  `map[string]any` / `[]any`. `truthyString` mirrors Python's `value or ""`
  falsiness; `valueToString` does not. Keep these distinct.
- `--json` (global) prints the raw response (HTML escaping off, indented). All
  human-readable output lives in `printResult` and friends.

### Adding a command

1. Add a `case` in the relevant `route*` function in `internal/cli/cli.go`
   (or a new `routeX` for a new top-level command, wired in `route`).
2. Build the request in the returned `apiCall` closure via the client's
   `Get`/`Post`/`Patch`/`Delete`.
3. If the response shape is new, extend `printResult` to format it.
4. Update `rootUsage`, add a test in `internal/cli/cli_test.go`, and document
   it in `README.md` and `skills/mantisbt/SKILL.md`.

## Behavior to preserve

- Destructive commands (`issue delete`, `issue note delete`) prompt for
  confirmation and abort on a non-interactive stdin unless `--yes` is passed.
  Never weaken this.
- `issues list` filters: `--project`/`--filter`/`--select` are server-side
  (query params); `--status`/`--search` are client-side over the fetched page.
- MantisBT enum fields (status, priority, severity, category) are sent by
  name (`{"name": ...}`); `issue create`'s `--project` accepts an id or a name,
  while the `issues list --project` filter is id-only (`project_id`).

## Releases

Pushing a `v*` tag triggers GoReleaser via GitHub Actions. Versions are stamped
into the binary through `-ldflags` (`main.version/commit/date`); do not hardcode
a version string elsewhere.
