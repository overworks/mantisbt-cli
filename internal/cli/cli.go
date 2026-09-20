package cli

import (
	"encoding/base64"
	"encoding/json"
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
	size int64
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
	maxResponseSize := root.Int64("max-response-size", mantis.DefaultMaxResponseBytes, "Maximum response size in bytes (default 64 MiB).")
	root.Usage = func() { rootUsage(root.Output()) }

	if err := root.Parse(argv); err != nil {
		return flagExitCode(err)
	}

	if versionFlag {
		if _, err := fmt.Fprintf(os.Stdout, "mantisbt-cli %s (commit %s, built %s)\n", Version, Commit, Date); err != nil {
			fmt.Fprintf(os.Stderr, "mantisbt-cli: failed to write output: %s\n", err)
			return 1
		}
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
	if !validByteLimit("mantisbt-cli", "--max-response-size", *maxResponseSize) {
		return 2
	}

	cfg, err := config.Load(urlFlag, tokenFlag, jsonFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}

	client := mantis.NewClient(cfg.URL, cfg.Token)
	client.MaxResponseBytes = *maxResponseSize
	result, err := call(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mantisbt-cli: %s\n", err)
		return 1
	}

	if err := printResult(os.Stdout, result, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "mantisbt-cli: failed to write output: %s\n", err)
		return 1
	}
	return 0
}

// route walks the subcommand tree and returns the matching API call, or nil
// and an exit code when the arguments do not resolve to a command.
func route(args []string) (apiCall, int) {
	if len(args) == 0 {
		rootUsage(os.Stderr)
		return nil, 2
	}
	switch args[0] {
	case "-h", "--help":
		rootUsage(os.Stderr)
		return nil, 0
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
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli auth whoami")
		if len(args) > 0 {
			return nil, 0
		}
		return nil, 2
	}
	switch args[0] {
	case "whoami":
		fs := commandFlags("auth whoami", "")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, flagExitCode(err)
		}
		if hasUnexpectedArgs(fs) {
			return nil, 2
		}
		return func(c *mantis.Client) (any, error) {
			return c.Get("/api/rest/users/me", nil)
		}, 0
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown auth command %q\n", args[0])
		return nil, 2
	}
}

func routeIssues(args []string) (apiCall, int) {
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issues list [--page-size N] [--page N]")
		if len(args) > 0 {
			return nil, 0
		}
		return nil, 2
	}
	switch args[0] {
	case "list":
		fs := commandFlags("issues list", "")
		pageSize := fs.String("page-size", "50", "Number of issues to request.")
		page := fs.String("page", "1", "Page number to request.")
		all := fs.Bool("all", false, "Fetch every page, starting at page 1.")
		maxPages := fs.Int64("max-pages", 10000, "Maximum pages to request with --all.")
		project := fs.String("project", "", "Filter by project id (server-side).")
		filterID := fs.String("filter", "", "Predefined filter: assigned, reported, monitored, unassigned, or a stored filter id (server-side).")
		selectFields := fs.String("select", "", "Comma-separated fields to return, e.g. id,summary,status (server-side).")
		status := fs.String("status", "", "Keep only issues with this status name (client-side, fetched pages only).")
		search := fs.String("search", "", "Keep only issues whose summary contains this text (client-side, fetched pages only).")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, flagExitCode(err)
		}
		if hasUnexpectedArgs(fs) ||
			!validInteger(fs.Name(), "--page-size", *pageSize, 1) ||
			!validInteger(fs.Name(), "--page", *page, 1) ||
			(*project != "" && !validInteger(fs.Name(), "--project", *project, 0)) ||
			!validFilter(*filterID) {
			return nil, 2
		}
		selection, err := validateSelection(*selectFields, *status, *search)
		if err != nil {
			fmt.Fprintf(os.Stderr, "issues list: %s\n", err)
			return nil, 2
		}
		if err := validateAllPages(fs, *all, *maxPages, selection); err != nil {
			fmt.Fprintf(os.Stderr, "issues list: %s\n", err)
			return nil, 2
		}

		params := map[string]string{"page_size": *pageSize, "page": *page}
		if *project != "" {
			params["project_id"] = *project
		}
		if *filterID != "" {
			params["filter_id"] = *filterID
		}
		if selection != "" {
			params["select"] = selection
		}

		statusFilter := *status
		searchFilter := strings.ToLower(*search)
		return func(c *mantis.Client) (any, error) {
			if *all {
				return listAllIssues(c, params, statusFilter, searchFilter, *maxPages)
			}
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
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue <get|create|update|delete|note|file> ...")
		if len(args) > 0 {
			return nil, 0
		}
		return nil, 2
	}
	switch args[0] {
	case "get":
		fs := commandFlags("issue get", "<issue_id>")
		ids, err := parseCommandFlags(fs, args[1:], 1)
		if err != nil {
			return nil, flagExitCode(err)
		}
		issueID := ids[0]
		if hasUnexpectedArgs(fs) || !validInteger(fs.Name(), "issue_id", issueID, 1) {
			return nil, 2
		}
		return func(c *mantis.Client) (any, error) {
			result, err := c.Get("/api/rest/issues/"+issueID, nil)
			if err != nil {
				return nil, err
			}
			return issueDetails{raw: result}, nil
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
	fs := commandFlags("issue create", "")
	summary := fs.String("summary", "", "Issue summary (required).")
	description := fs.String("description", "", "Issue description (required).")
	project := fs.String("project", "", "Project id or name (required).")
	category := fs.String("category", "", "Category name (required).")
	priority := fs.String("priority", "", "Priority name (optional).")
	severity := fs.String("severity", "", "Severity name (optional).")
	if err := fs.Parse(args); err != nil {
		return nil, flagExitCode(err)
	}
	if hasUnexpectedArgs(fs) {
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
	if !validProject(*project) {
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
	fs := commandFlags("issue update", "<issue_id>")
	summary := fs.String("summary", "", "New summary.")
	description := fs.String("description", "", "New description.")
	status := fs.String("status", "", "New status name.")
	handler := fs.String("handler", "", "Assignee username.")
	priority := fs.String("priority", "", "New priority name.")
	severity := fs.String("severity", "", "New severity name.")
	ids, err := parseCommandFlags(fs, args, 1)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID := ids[0]
	if hasUnexpectedArgs(fs) || !validInteger(fs.Name(), "issue_id", issueID, 1) {
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
	fs := commandFlags("issue delete", "<issue_id>")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt.")
	ids, err := parseCommandFlags(fs, args, 1)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID := ids[0]
	if hasUnexpectedArgs(fs) || !validInteger(fs.Name(), "issue_id", issueID, 1) {
		return nil, 2
	}

	skipPrompt := *yes
	return func(c *mantis.Client) (any, error) {
		if !skipPrompt {
			if err := confirm(os.Stdin, os.Stderr, fmt.Sprintf("Delete issue #%s? This cannot be undone. [y/N] ", issueID)); err != nil {
				return nil, err
			}
		}
		return c.Delete("/api/rest/issues/" + issueID)
	}, 0
}

func routeIssueNote(args []string) (apiCall, int) {
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue note <add|delete> ...")
		if len(args) > 0 {
			return nil, 0
		}
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
	fs := commandFlags("issue note add", "<issue_id>")
	text := fs.String("text", "", "Note text (required).")
	private := fs.Bool("private", false, "Make the note private.")
	ids, err := parseCommandFlags(fs, args, 1)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID := ids[0]
	if hasUnexpectedArgs(fs) || !validInteger(fs.Name(), "issue_id", issueID, 1) {
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
	fs := commandFlags("issue note delete", "<issue_id> <note_id>")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt.")
	ids, err := parseCommandFlags(fs, args, 2)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID, noteID := ids[0], ids[1]
	if hasUnexpectedArgs(fs) ||
		!validInteger(fs.Name(), "issue_id", issueID, 1) ||
		!validInteger(fs.Name(), "note_id", noteID, 1) {
		return nil, 2
	}

	skipPrompt := *yes
	return func(c *mantis.Client) (any, error) {
		if !skipPrompt {
			if err := confirm(os.Stdin, os.Stderr, fmt.Sprintf("Delete note #%s on issue #%s? [y/N] ", noteID, issueID)); err != nil {
				return nil, err
			}
		}
		return c.Delete("/api/rest/issues/" + issueID + "/notes/" + noteID)
	}, 0
}

func routeIssueFile(args []string) (apiCall, int) {
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue file <add|list|get> ...")
		if len(args) > 0 {
			return nil, 0
		}
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
	fs := commandFlags("issue file add", "<issue_id> <path>...")
	maxUploadSize := fs.Int64("max-upload-size", defaultMaxUploadBytes, "Maximum combined file size in bytes (default 32 MiB). Put this flag before file paths.")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: mantisbt-cli issue file add <issue_id> [flags] <path>...")
		fs.PrintDefaults()
	}
	ids, err := parseCommandFlags(fs, args, 1)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID := ids[0]
	if !validInteger(fs.Name(), "issue_id", issueID, 1) || !validByteLimit(fs.Name(), "--max-upload-size", *maxUploadSize) {
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
		remaining := *maxUploadSize
		for _, path := range paths {
			data, err := readUpload(path, remaining)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			files = append(files, map[string]any{
				"name":    filepath.Base(path),
				"content": data, // encoding/json encodes []byte as base64.
			})
			remaining -= int64(len(data))
		}
		return c.Post("/api/rest/issues/"+issueID+"/files", map[string]any{"files": files})
	}, 0
}

func routeIssueFileList(args []string) (apiCall, int) {
	fs := commandFlags("issue file list", "<issue_id>")
	ids, err := parseCommandFlags(fs, args, 1)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID := ids[0]
	if hasUnexpectedArgs(fs) || !validInteger(fs.Name(), "issue_id", issueID, 1) {
		return nil, 2
	}

	return func(c *mantis.Client) (any, error) {
		return c.Get("/api/rest/issues/"+issueID+"/files", nil)
	}, 0
}

func routeIssueFileGet(args []string) (apiCall, int) {
	fs := commandFlags("issue file get", "<issue_id> <file_id>")
	output := fs.String("output", "", "Destination path. Defaults to the attachment filename in the current directory.")
	ids, err := parseCommandFlags(fs, args, 2)
	if err != nil {
		return nil, flagExitCode(err)
	}
	issueID, fileID := ids[0], ids[1]
	if hasUnexpectedArgs(fs) ||
		!validInteger(fs.Name(), "issue_id", issueID, 1) ||
		!validInteger(fs.Name(), "file_id", fileID, 1) {
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

		encoded, ok := file["content"].(string)
		if !ok {
			return nil, fmt.Errorf("attachment #%s has no content available for download", fileID)
		}
		if encoded == "" {
			if size := valueToString(file["size"]); size != "" && size != "0" {
				return nil, fmt.Errorf("attachment #%s reports a non-zero size but has empty content", fileID)
			}
		}
		data := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
		size, err := writeAttachment(path, data, dest != "")
		if err != nil {
			return nil, fmt.Errorf("attachment #%s: %w", fileID, err)
		}
		return downloaded{raw: result, path: path, size: size}, nil
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

// projectRef renders a project reference, using an id when the value is numeric
// and a name otherwise.
func projectRef(v string) map[string]any {
	if id, err := strconv.Atoi(v); err == nil {
		return map[string]any{"id": id}
	}
	return map[string]any{"name": v}
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

func printResult(w io.Writer, result any, cfg config.Config) error {
	if detail, ok := result.(issueDetails); ok {
		if cfg.JSONOutput {
			return printJSON(w, detail.raw)
		}
		return printIssueDetails(w, detail.raw)
	}
	if d, ok := result.(downloaded); ok {
		if cfg.JSONOutput {
			return printJSON(w, d.raw)
		}
		_, err := fmt.Fprintf(w, "wrote %s (%d bytes)\n", d.path, d.size)
		return err
	}

	if cfg.JSONOutput {
		return printJSON(w, result)
	}

	if result == nil {
		_, err := fmt.Fprintln(w, "OK")
		return err
	}

	if m, ok := result.(map[string]any); ok {
		if issues, ok := m["issues"]; ok {
			return printIssues(w, issues)
		}
		if issue, ok := m["issue"].(map[string]any); ok {
			return printIssue(w, issue)
		}
		if note, ok := m["note"].(map[string]any); ok {
			return printNote(w, note)
		}
		if files, ok := m["files"]; ok {
			return printFiles(w, files)
		}
		if user, ok := m["user"].(map[string]any); ok {
			return printUser(w, user)
		}
	}

	return printJSON(w, result)
}

func printUser(w io.Writer, user map[string]any) error {
	name := firstNonEmpty(user, "real_name", "name", "email")
	if name == "" {
		name = "unknown"
	}
	id := "unknown"
	if v, ok := user["id"]; ok {
		id = valueToString(v)
	}
	_, err := fmt.Fprintf(w, "%s (%s)\n", name, id)
	return err
}

func printIssues(w io.Writer, issues any) error {
	list, ok := issues.([]any)
	if !ok {
		return printJSON(w, issues)
	}
	for _, item := range list {
		if issue, ok := item.(map[string]any); ok {
			if err := printIssue(w, issue); err != nil {
				return err
			}
		}
	}
	return nil
}

func printNote(w io.Writer, note map[string]any) error {
	id := "unknown"
	if v, ok := note["id"]; ok {
		id = valueToString(v)
	}
	reporter := ValueName(note["reporter"])
	text := valueToString(note["text"])
	_, err := fmt.Fprintf(w, "note #%s by %s: %s\n", id, reporter, text)
	return err
}

func printFiles(w io.Writer, files any) error {
	list, ok := files.([]any)
	if !ok {
		return printJSON(w, files)
	}
	for _, item := range list {
		if file, ok := item.(map[string]any); ok {
			if _, err := fmt.Fprintln(w, fileLine(file)); err != nil {
				return err
			}
		}
	}
	return nil
}

// fileLine renders one attachment. Attachment responses carry the base64
// content inline, which is deliberately left out of the human-readable form.
func fileLine(file map[string]any) string {
	id := "unknown"
	if v, ok := file["id"]; ok {
		id = valueToString(v)
	}
	line := fmt.Sprintf("file #%s %s (%s bytes", id, firstNonEmpty(file, "filename", "name"), valueToString(file["size"]))
	if ct := truthyString(file["content_type"]); ct != "" {
		line += ", " + ct
	}
	line += ")"
	if reporter := ValueName(file["reporter"]); reporter != "" {
		line += " by " + reporter
	}
	return line
}

func printIssue(w io.Writer, issue map[string]any) error {
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
	_, err := fmt.Fprintf(w, "#%s [%s] %s - %s\n", issueID, status, project, summary)
	return err
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

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func rootUsage(w io.Writer) {
	fmt.Fprint(w, `mantisbt-cli - a small client for the MantisBT REST API

usage:
  mantisbt-cli [--url URL] [--token TOKEN] [--json] <command> [args]

commands:
  auth whoami            Show the authenticated user.
  issues list            List accessible issues.
                         [--page-size N] [--page N] [--project id]
                         [--all] [--max-pages N]
                         [--filter assigned|reported|monitored|unassigned]
                         [--select fields] [--status name] [--search text]
  issue get <id>         Show an issue with its description, notes, and attachments.
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
  issue file add <id> [--max-upload-size bytes] <path>...
                         Attach one or more local files to an issue.
  issue file list <id>   List the attachments on an issue.
  issue file get <id> <file_id>
                         Download an attachment. [--output path]

global flags:
  --url      MantisBT base URL. Defaults to MANTISBT_URL.
  --token    MantisBT API token. Defaults to MANTISBT_TOKEN.
  --json     Print raw JSON responses.
  --version  Print version and exit.
  --max-response-size  Maximum response size in bytes (default 67108864).
`)
}
