---
name: mantisbt
description: Manage MantisBT issues from the command line with the mantisbt-cli tool — list and search issues, show an issue, create and update issues, add or delete notes, upload and download issue attachments, and delete issues. Use whenever the user wants to read or change MantisBT bug-tracker data (issues, notes, attachments, statuses, assignments, projects).
license: MIT
metadata:
  version: 0.1.0
allowed-tools: Bash(mantisbt-cli *)
---

## What this is

`mantisbt-cli` is a single-binary client for the MantisBT REST API. Use it to
read and modify issues in a MantisBT instance.

## Setup (required before any call)

The tool needs a base URL and an API token. These usually come from the
environment:

```bash
export MANTISBT_URL="https://mantis.example.com"
export MANTISBT_TOKEN="…"
```

`MANTISBT_URL` may point at the MantisBT web root, or at the REST API root if
the instance exposes one, such as `https://mantis.example.com/api/rest` or
`https://mantis.example.com/api/rest/index.php`.

If they are not set, every command fails with a "missing configuration" error.
You may instead pass `--url` / `--token` before the subcommand. Check
connectivity with `mantisbt-cli auth whoami`.

## Parsing output

For human-readable summaries, run commands as-is. **When you need to read
fields programmatically, add the global `--json` flag** (before the subcommand)
and parse the raw JSON response — do not scrape the formatted text.

```bash
mantisbt-cli --json issue get 1234
```

## Commands

```bash
# Read
mantisbt-cli auth whoami
mantisbt-cli issues list [--page-size N] [--page N]
mantisbt-cli issue get <id>

# Create (summary, description, project, category are required)
mantisbt-cli issue create --summary "…" --description "…" \
  --project <id-or-name> --category <name> [--priority <name>] [--severity <name>]

# Update — only the fields you pass are changed (partial update)
mantisbt-cli issue update <id> [--summary …] [--description …] \
  [--status <name>] [--handler <username>] [--priority <name>] [--severity <name>]

# Notes
mantisbt-cli issue note add <id> --text "…" [--private]
mantisbt-cli issue note delete <id> <note_id> [--yes]

# Attachments
mantisbt-cli issue file add <id> <path>...
mantisbt-cli issue file list <id>
mantisbt-cli issue file get <id> <file_id> [--output <path>]

# Delete an issue
mantisbt-cli issue delete <id> [--yes]
```

## Filtering `issues list`

Server-side (sent to the API): `--project <id>`, `--filter assigned|reported|monitored|unassigned`, `--select id,summary,status`.

Client-side, applied to the fetched page only: `--status <name>`, `--search <text>`.
Because MantisBT does not filter by status or summary server-side, raise
`--page-size` when using these so more issues are considered.

## Safety rules for destructive commands

`issue delete` and `issue note delete` permanently remove data.

- Do **not** pass `--yes` unless the user has explicitly approved that specific
  deletion. Without `--yes` the tool prompts for confirmation, and on a
  non-interactive shell (how you run it) it will abort instead of deleting — so
  to actually delete you must both confirm with the user and pass `--yes`.
- Before deleting, fetch the issue (`issue get <id>`) and show the user what
  will be removed.

## Tips

- Enum-like fields (`--status`, `--priority`, `--severity`, `--category`) take
  the MantisBT name (e.g. `--status resolved`), not a numeric id.
- `--project` on `issue create` accepts a project id or name; on `issues list`
  it must be a numeric project id.
- A successful write prints the affected issue/note; a successful delete prints
  `OK`. A non-zero exit code means the call failed — read stderr.
- `issue file add` uploads every path in one request and prints `OK`; run
  `issue file list <id>` afterwards to see the stored attachment ids.
- `issue file get` without `--output` writes the attachment to its own filename
  in the current directory and refuses to overwrite an existing file, so pass
  `--output` when you need a specific destination.
- MantisBT returns attachment content inline as base64, so `--json issue file
  list` includes every attachment's full contents — prefer the plain output
  when you only need ids, names, and sizes.
