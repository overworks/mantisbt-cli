package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

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
		if err := fs.Parse(args[1:]); err != nil {
			return nil, 2
		}
		ps, pg := *pageSize, *page
		return func(c *mantis.Client) (any, error) {
			return c.Get("/api/rest/issues", map[string]string{"page_size": ps, "page": pg})
		}, 0
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown issues command %q\n", args[0])
		return nil, 2
	}
}

func routeIssue(args []string) (apiCall, int) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mantisbt-cli issue get <issue_id>")
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
	default:
		fmt.Fprintf(os.Stderr, "mantisbt-cli: unknown issue command %q\n", args[0])
		return nil, 2
	}
}

func printResult(result any, cfg config.Config) {
	if cfg.JSONOutput {
		printJSON(result)
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
  issues list            List accessible issues. [--page-size N] [--page N]
  issue get <issue_id>   Show an issue by id.

global flags:
  --url      MantisBT base URL. Defaults to MANTISBT_URL.
  --token    MantisBT API token. Defaults to MANTISBT_TOKEN.
  --json     Print raw JSON responses.
  --version  Print version and exit.
`)
}
