package cli

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/overworks/mantisbt-cli/internal/config"
	"github.com/overworks/mantisbt-cli/internal/mantis"
)

// Version, Commit, and Date are set from main at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// apiCall is a prepared request, deferred until configuration is loaded.
type apiCall func(*mantis.Client) (any, error)

// downloaded is returned by commands that save a response to disk. It keeps the
// raw response around so --json stays raw while human output reports where the
// bytes landed.
type downloaded struct {
	raw  any
	path string
	size int
}

// Main is the CLI entry point. It returns a process exit code.
func Main(argv []string) int {
	root := flag.NewFlagSet("mantisbt-cli", flag.ContinueOnError)
	var urlFlag, tokenFlag string
	var jsonFlag, versionFlag bool
	root.StringVar(&urlFlag, "url", "", "MantisBT base URL. Defaults to MANTISBT_URL.")
	root.StringVar(&tokenFlag, "token", "", "MantisBT API token. Defaults to MANTISBT_TOKEN.")
	root.BoolVar(&jsonFlag, "json", false, "Print raw JSON responses.")
	root.BoolVar(&versionFlag, "version", false, "Print version and exit.")
	root.Usage = func() { rootUsage(root.Output()) }

	if err := root.Parse(argv); err != nil {
		return 2
	}

	if versionFlag {
		fmt.Printf("mantisbt-cli %s (commit %s, built %s)\n", Version, Commit, Date)
		return 0
	}

	args := root.Args()
	if len(args) == 0 {
		rootUsage(os.Stderr)
		return 2
	}

	call, code := route(args)
	if call == nil {
		return code
	}

	cfg, err := config.Load(urlFlag, tokenFlag, jsonFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}

	client := mantis.NewClient(cfg.URL, cfg.Token)
	result, err := call(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mantisbt-cli: %s\n", err)
		return 1
	}

	printResult(result, cfg)
	return 0
}

// route walks the subcommand tree and returns the matching API call, or nil
// and an exit code when the arguments do not resolve to a command.
func route(args []string) (apiCall, int) {
	switch args[0] {
	case "auth":
		return routeAuth(args[1:])
	case "issues":
		return routeIssues(args[1:])
	case "issue":
		return routeIssue(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown command %q\n", args[0])
		return nil, 2
	}
}

func routeAuth(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli auth whoami")
		return nil, 2
	}
	switch args[0] {
	case "whoami":
		return func(c *mantis.Client) (any, error) {
			return c.Get("/api/rest/users/me", nil)
		}, 0
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown auth command %q\n", args[0])
		return nil, 2
	}
}

func routeIssues(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issues list [--page-size N] [--page N]")
		return nil, 2
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("issues list", flag.ContinueOnError)
		pageSize := fs.String("page-size", "50", "Number of issues to request.")
		page := fs.String("page", "1", "Page number to request.")
		project := fs.String("project", "", "Filter by project id (server-side).")
		filterID := fs.String("filter", "", "Predefined filter: assigned, reported, monitored, unassigned, or a stored filter id (server-side).")
		selectFields := fs.String("select", "", "Comma-separated fields to return, e.g. id,summary,status (server-side).")
		status := fs.String("status", "", "Keep only issues with this status name (client-side, current page only).")
		search := fs.String("search", "", "Keep only issues whose summary contains this text (client-side, current page only).")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, 2
		}

		params := map[string]string{"page_size": *pageSize, "page": *page}
		if *project != "" {
			params["project_id"] = *project
		}
		if *filterID != "" {
			params["filter_id"] = *filterID
		}
		if *selectFields != "" {
			params["select"] = *selectFields
		}

		statusFilter := *status
		searchFilter := strings.ToLower(*search)
		return func(c *mantis.Client) (any, error) {
			result, err := c.Get("/api/rest/issues", params)
			if err != nil {
				return nil, err
			}
			if statusFilter == "" && searchFilter == "" {
				return result, nil
			}
			return filterIssues(result, statusFilter, searchFilter), nil
		}, 0
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown issues command %q\n", args[0])
		return nil, 2
	}
}

func routeIssue(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue <get|create|update|delete|note|file> ...")
		return nil, 2
	}
	switch args[0] {
	case "get":
		fs := flag.NewFlagSet("issue get", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return nil, 2
		}
		rest := fs.Args()
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue get <issue_id>")
			return nil, 2
		}
		issueID := rest[0]
		return func(c *mantis.Client) (any, error) {
			return c.Get("/api/rest/issues/"+issueID, nil)
		}, 0
	case "create":
		return routeIssueCreate(args[1:])
	case "update":
		return routeIssueUpdate(args[1:])
	case "delete":
		return routeIssueDelete(args[1:])
	case "note":
		return routeIssueNote(args[1:])
	case "file":
		return routeIssueFile(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown issue command %q\n", args[0])
		return nil, 2
	}
}

func routeIssueCreate(args []string) (apiCall, int) {
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	summary := fs.String("summary", "", "Issue summary (required).")
	description := fs.String("description", "", "Issue description (required).")
	project := fs.String("project", "", "Project id or name (required).")
	category := fs.String("category", "", "Category name (required).")
	priority := fs.String("priority", "", "Priority name (optional).")
	severity := fs.String("severity", "", "Severity name (optional).")
	if err := fs.Parse(args); err != nil {
		return nil, 2
	}

	var missing []string
	if *summary == "" {
		missing = append(missing, "--summary")
	}
	if *description == "" {
		missing = append(missing, "--description")
	}
	if *project == "" {
		missing = append(missing, "--project")
	}
	if *category == "" {
		missing = append(missing, "--category")
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "issue create: missing required flags: %s\n", strings.Join(missing, ", "))
		return nil, 2
	}

	body := map[string]any{
		"summary":     *summary,
		"description": *description,
		"category":    map[string]any{"name": *category},
		"project":     projectRef(*project),
	}
	if *priority != "" {
		body["priority"] = map[string]any{"name": *priority}
	}
	if *severity != "" {
		body["severity"] = map[string]any{"name": *severity}
	}

	return func(c *mantis.Client) (any, error) {
		return c.Post("/api/rest/issues", body)
	}, 0
}

func routeIssueUpdate(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue update <issue_id> [--summary ...] [--status ...] ...")
		return nil, 2
	}
	issueID := args[0]

	fs := flag.NewFlagSet("issue update", flag.ContinueOnError)
	summary := fs.String("summary", "", "New summary.")
	description := fs.String("description", "", "New description.")
	status := fs.String("status", "", "New status name.")
	handler := fs.String("handler", "", "Assignee username.")
	priority := fs.String("priority", "", "New priority name.")
	severity := fs.String("severity", "", "New severity name.")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, 2
	}

	// Only send fields the user actually set, so unspecified fields are left
	// untouched by the partial update.
	body := map[string]any{}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "summary":
			body["summary"] = *summary
		case "description":
			body["description"] = *description
		case "status":
			body["status"] = map[string]any{"name": *status}
		case "handler":
			body["handler"] = map[string]any{"name": *handler}
		case "priority":
			body["priority"] = map[string]any{"name": *priority}
		case "severity":
			body["severity"] = map[string]any{"name": *severity}
		}
	})
	if len(body) == 0 {
		fmt.Fprintln(os.Stderr, "issue update: nothing to update; provide at least one field flag")
		return nil, 2
	}

	return func(c *mantis.Client) (any, error) {
		return c.Patch("/api/rest/issues/"+issueID, body)
	}, 0
}

func routeIssueDelete(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue delete <issue_id> [--yes]")
		return nil, 2
	}
	issueID := args[0]

	fs := flag.NewFlagSet("issue delete", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "Skip the confirmation prompt.")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, 2
	}

	skipPrompt := *yes
	return func(c *mantis.Client) (any, error) {
		if !skipPrompt && !confirm(fmt.Sprintf("Delete issue #%s? This cannot be undone. [y/N] ", issueID)) {
			return nil, fmt.Errorf("aborted")
		}
		return c.Delete("/api/rest/issues/" + issueID)
	}, 0
}

func routeIssueNote(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue note <add|delete> ...")
		return nil, 2
	}
	switch args[0] {
	case "add":
		return routeIssueNoteAdd(args[1:])
	case "delete":
		return routeIssueNoteDelete(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown note command %q\n", args[0])
		return nil, 2
	}
}

func routeIssueNoteAdd(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue note add <issue_id> --text <text> [--private]")
		return nil, 2
	}
	issueID := args[0]

	fs := flag.NewFlagSet("issue note add", flag.ContinueOnError)
	text := fs.String("text", "", "Note text (required).")
	private := fs.Bool("private", false, "Make the note private.")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, 2
	}
	if *text == "" {
		fmt.Fprintln(os.Stderr, "issue note add: --text is required")
		return nil, 2
	}

	body := map[string]any{"text": *text}
	if *private {
		body["view_state"] = map[string]any{"name": "private"}
	}

	return func(c *mantis.Client) (any, error) {
		return c.Post("/api/rest/issues/"+issueID+"/notes", body)
	}, 0
}

func routeIssueNoteDelete(args []string) (apiCall, int) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue note delete <issue_id> <note_id> [--yes]")
		return nil, 2
	}
	issueID, noteID := args[0], args[1]

	fs := flag.NewFlagSet("issue note delete", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "Skip the confirmation prompt.")
	if err := fs.Parse(args[2:]); err != nil {
		return nil, 2
	}

	skipPrompt := *yes
	return func(c *mantis.Client) (any, error) {
		if !skipPrompt && !confirm(fmt.Sprintf("Delete note #%s on issue #%s? [y/N] ", noteID, issueID)) {
			return nil, fmt.Errorf("aborted")
		}
		return c.Delete("/api/rest/issues/" + issueID + "/notes/" + noteID)
	}, 0
}

func routeIssueFile(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue file <add|list|get> ...")
		return nil, 2
	}
	switch args[0] {
	case "add":
		return routeIssueFileAdd(args[1:])
	case "list":
		return routeIssueFileList(args[1:])
	case "get":
		return routeIssueFileGet(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown file command %q\n", args[0])
		return nil, 2
	}
}

func routeIssueFileAdd(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue file add <issue_id> <path>...")
		return nil, 2
	}
	issueID := args[0]

	fs := flag.NewFlagSet("issue file add", flag.ContinueOnError)
	if err := fs.Parse(args[1:]); err != nil {
		return nil, 2
	}
	paths := fs.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "issue file add: at least one file path is required")
		return nil, 2
	}

	// Files are read when the command runs, so routing stays free of I/O and a
	// missing path is reported as a runtime error rather than a usage error.
	return func(c *mantis.Client) (any, error) {
		files := make([]any, 0, len(paths))
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			files = append(files, map[string]any{
				"name":    filepath.Base(path),
				"content": base64.StdEncoding.EncodeToString(data),
			})
		}
		return c.Post("/api/rest/issues/"+issueID+"/files", map[string]any{"files": files})
	}, 0
}

func routeIssueFileList(args []string) (apiCall, int) {
	fs := flag.NewFlagSet("issue file list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return nil, 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue file list <issue_id>")
		return nil, 2
	}
	issueID := rest[0]

	return func(c *mantis.Client) (any, error) {
		return c.Get("/api/rest/issues/"+issueID+"/files", nil)
	}, 0
}

func routeIssueFileGet(args []string) (apiCall, int) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue file get <issue_id> <file_id> [--output path]")
		return nil, 2
	}
	issueID, fileID := args[0], args[1]

	fs := flag.NewFlagSet("issue file get", flag.ContinueOnError)
	output := fs.String("output", "", "Destination path. Defaults to the attachment filename in the current directory.")
	if err := fs.Parse(args[2:]); err != nil {
		return nil, 2
	}

	dest := *output
	return func(c *mantis.Client) (any, error) {
		result, err := c.Get("/api/rest/issues/"+issueID+"/files/"+fileID, nil)
		if err != nil {
			return nil, err
		}
		file, ok := firstFile(result)
		if !ok {
			return nil, fmt.Errorf("attachment #%s not found on issue #%s", fileID, issueID)
		}

		path, err := attachmentPath(dest, valueToString(file["filename"]), fileID)
		if err != nil {
			return nil, err
		}

		encoded, _ := file["content"].(string)
		if encoded == "" {
			return nil, fmt.Errorf("attachment #%s has no content available for download", fileID)
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("attachment #%s: malformed content: %w", fileID, err)
		}
		// Attachments can carry sensitive data, so they land owner-only rather
		// than at the umask default.
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		return downloaded{raw: result, path: path, size: len(data)}, nil
	}, 0
}

// firstFile pulls the first entry out of a {"files": [...]} response.
func firstFile(result any) (map[string]any, bool) {
	m, ok := result.(map[string]any)
	if !ok {
		return nil, false
	}
	list, ok := m["files"].([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	file, ok := list[0].(map[string]any)
	return file, ok
}

// attachmentPath decides where a downloaded attachment is written. An explicit
// --output wins and may overwrite; otherwise the server-supplied filename is
// used, reduced to its base name so the server cannot pick the directory, and
// an existing file is never clobbered without the user naming it.
func attachmentPath(output, filename, fileID string) (string, error) {
	if output != "" {
		return output, nil
	}
	name := filepath.Base(filename)
	if filename == "" || name == "." || name == string(filepath.Separator) {
		return "", fmt.Errorf("attachment #%s has no usable filename; pass --output", fileID)
	}
	switch _, err := os.Stat(name); {
	case err == nil:
		return "", fmt.Errorf("%s already exists; pass --output to choose a destination", name)
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("check %s: %w", name, err)
	}
	return name, nil
}

// projectRef renders a project reference, using an id when the value is numeric
// and a name otherwise.
func projectRef(v string) map[string]any {
	if id, err := strconv.Atoi(v); err == nil {
		return map[string]any{"id": id}
	}
	return map[string]any{"name": v}
}

// confirm prints a prompt and reads a yes/no answer from stdin. A non-interactive
// stdin (EOF) is treated as "no".
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// filterIssues applies client-side filters to a list response. status is
// matched case-insensitively against the issue status name; search is a
// lower-cased substring matched against the summary. The caller guarantees at
// least one filter is non-empty.
func filterIssues(result any, status, search string) any {
	m, ok := result.(map[string]any)
	if !ok {
		return result
	}
	raw, ok := m["issues"].([]any)
	if !ok {
		return result
	}

	kept := make([]any, 0, len(raw))
	for _, item := range raw {
		issue, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if status != "" && !strings.EqualFold(ValueName(issue["status"]), status) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(valueToString(issue["summary"])), search) {
			continue
		}
		kept = append(kept, issue)
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["issues"] = kept
	return out
}

func printResult(result any, cfg config.Config) {
	if d, ok := result.(downloaded); ok {
		if cfg.JSONOutput {
			printJSON(d.raw)
			return
		}
		fmt.Printf("wrote %s (%d bytes)\n", d.path, d.size)
		return
	}

	if cfg.JSONOutput {
		printJSON(result)
		return
	}

	if result == nil {
		fmt.Println("OK")
		return
	}

	if m, ok := result.(map[string]any); ok {
		if issues, ok := m["issues"]; ok {
			printIssues(issues)
			return
		}
		if issue, ok := m["issue"].(map[string]any); ok {
			printIssue(issue)
			return
		}
		if note, ok := m["note"].(map[string]any); ok {
			printNote(note)
			return
		}
		if files, ok := m["files"]; ok {
			printFiles(files)
			return
		}
		if user, ok := m["user"].(map[string]any); ok {
			printUser(user)
			return
		}
	}

	printJSON(result)
}

func printUser(user map[string]any) {
	name := firstNonEmpty(user, "real_name", "name", "email")
	if name == "" {
		name = "unknown"
	}
	id := "unknown"
	if v, ok := user["id"]; ok {
		id = valueToString(v)
	}
	fmt.Printf("%s (%s)\n", name, id)
}

func printIssues(issues any) {
	list, ok := issues.([]any)
	if !ok {
		printJSON(issues)
		return
	}
	for _, item := range list {
		if issue, ok := item.(map[string]any); ok {
			printIssue(issue)
		}
	}
}

func printNote(note map[string]any) {
	id := "unknown"
	if v, ok := note["id"]; ok {
		id = valueToString(v)
	}
	reporter := ValueName(note["reporter"])
	text := valueToString(note["text"])
	fmt.Printf("note #%s by %s: %s\n", id, reporter, text)
}

func printFiles(files any) {
	list, ok := files.([]any)
	if !ok {
		printJSON(files)
		return
	}
	for _, item := range list {
		if file, ok := item.(map[string]any); ok {
			fmt.Println(fileLine(file))
		}
	}
}

// fileLine renders one attachment. Attachment responses carry the base64
// content inline, which is deliberately left out of the human-readable form.
func fileLine(file map[string]any) string {
	id := "unknown"
	if v, ok := file["id"]; ok {
		id = valueToString(v)
	}
	line := fmt.Sprintf("file #%s %s (%s bytes", id, valueToString(file["filename"]), valueToString(file["size"]))
	if ct := truthyString(file["content_type"]); ct != "" {
		line += ", " + ct
	}
	line += ")"
	if reporter := ValueName(file["reporter"]); reporter != "" {
		line += " by " + reporter
	}
	return line
}

func printIssue(issue map[string]any) {
	issueID := "unknown"
	if v, ok := issue["id"]; ok {
		issueID = valueToString(v)
	}
	summary := ""
	if v, ok := issue["summary"]; ok {
		summary = valueToString(v)
	}
	status := ValueName(issue["status"])
	project := ValueName(issue["project"])
	fmt.Printf("#%s [%s] %s - %s\n", issueID, status, project, summary)
}

// ValueName extracts a human-readable label from a MantisBT enum/object value,
// preferring name, then label, then id.
func ValueName(value any) string {
	if m, ok := value.(map[string]any); ok {
		if s := truthyString(m["name"]); s != "" {
			return s
		}
		if s := truthyString(m["label"]); s != "" {
			return s
		}
		if s := truthyString(m["id"]); s != "" {
			return s
		}
		return ""
	}
	return truthyString(value)
}

func firstNonEmpty(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := truthyString(m[key]); s != "" {
			return s
		}
	}
	return ""
}

// truthyString mirrors Python's `str(value or "")`: falsy values (nil, "", 0,
// false) render as the empty string.
func truthyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == 0 {
			return ""
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if !t {
			return ""
		}
		return "true"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// valueToString renders a value without applying truthiness, used where the
// original code only substitutes a default for missing keys.
func valueToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "mantisbt-cli: failed to render JSON: %s\n", err)
	}
}

func rootUsage(w io.Writer) {
	fmt.Fprint(w, `mantisbt-cli - a small client for the MantisBT REST API

usage:
  mantisbt-cli [--url URL] [--token TOKEN] [--json] <command> [args]

commands:
  auth whoami            Show the authenticated user.
  issues list            List accessible issues.
                         [--page-size N] [--page N] [--project id]
                         [--filter assigned|reported|monitored|unassigned]
                         [--select fields] [--status name] [--search text]
  issue get <id>         Show an issue by id.
  issue create           Create an issue.
                         --summary --description --project --category
                         [--priority] [--severity]
  issue update <id>      Update issue fields.
                         [--summary] [--description] [--status] [--handler]
                         [--priority] [--severity]
  issue delete <id>      Delete an issue. [--yes]
  issue note add <id>    Add a note. --text [--private]
  issue note delete <id> <note_id>
                         Delete a note. [--yes]
  issue file add <id> <path>...
                         Attach one or more local files to an issue.
  issue file list <id>   List the attachments on an issue.
  issue file get <id> <file_id>
                         Download an attachment. [--output path]

global flags:
  --url      MantisBT base URL. Defaults to MANTISBT_URL.
  --token    MantisBT API token. Defaults to MANTISBT_TOKEN.
  --json     Print raw JSON responses.
  --version  Print version and exit.
`)
}
