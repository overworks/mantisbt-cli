package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/mantis"
)

func TestRouteAcceptsIssueGetCommand(t *testing.T) {
	call, code := route([]string{"issue", "get", "123"})
	if call == nil {
		t.Fatalf("expected a resolved command, got nil (exit code %d)", code)
	}
}

func TestRouteRejectsIssueGetWithoutID(t *testing.T) {
	call, code := route([]string{"issue", "get"})
	if call != nil {
		t.Fatal("expected no command for missing issue id")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestRouteAcceptsWriteCommands(t *testing.T) {
	cases := [][]string{
		{"issue", "create", "--summary", "s", "--description", "d", "--project", "1", "--category", "c"},
		{"issue", "update", "123", "--status", "resolved"},
		{"issue", "delete", "123", "--yes"},
		{"issue", "note", "add", "123", "--text", "hello"},
		{"issue", "note", "delete", "123", "9", "--yes"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			call, code := route(args)
			if call == nil {
				t.Fatalf("expected a resolved command, got nil (exit code %d)", code)
			}
		})
	}
}

func TestRouteRejectsCreateWithoutRequiredFlags(t *testing.T) {
	call, code := route([]string{"issue", "create", "--summary", "only"})
	if call != nil {
		t.Fatal("expected no command when required flags are missing")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestRouteRejectsUpdateWithoutFields(t *testing.T) {
	call, code := route([]string{"issue", "update", "123"})
	if call != nil {
		t.Fatal("expected no command when no update fields are provided")
	}
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
}

func TestRouteAcceptsIssuesListFilters(t *testing.T) {
	args := []string{"issues", "list", "--project", "3", "--filter", "assigned",
		"--select", "id,summary,status", "--status", "resolved", "--search", "login"}
	call, code := route(args)
	if call == nil {
		t.Fatalf("expected a resolved command, got nil (exit code %d)", code)
	}
}

func TestFilterIssues(t *testing.T) {
	result := map[string]any{
		"issues": []any{
			map[string]any{"id": float64(1), "summary": "Login button broken", "status": map[string]any{"name": "new"}},
			map[string]any{"id": float64(2), "summary": "Logout is slow", "status": map[string]any{"name": "resolved"}},
			map[string]any{"id": float64(3), "summary": "login form typo", "status": map[string]any{"name": "resolved"}},
		},
	}

	issuesOf := func(v any) []any {
		return v.(map[string]any)["issues"].([]any)
	}

	// status is matched case-insensitively.
	if got := issuesOf(filterIssues(result, "RESOLVED", "")); len(got) != 2 {
		t.Fatalf("status filter: expected 2 issues, got %d", len(got))
	}

	// search is a case-insensitive substring on the summary.
	if got := issuesOf(filterIssues(result, "", "login")); len(got) != 2 {
		t.Fatalf("search filter: expected 2 issues, got %d", len(got))
	}

	// combined filters are ANDed together.
	got := issuesOf(filterIssues(result, "resolved", "login"))
	if len(got) != 1 {
		t.Fatalf("combined filter: expected 1 issue, got %d", len(got))
	}
	if id := got[0].(map[string]any)["id"]; id != float64(3) {
		t.Fatalf("combined filter: expected issue 3, got %v", id)
	}
}

func TestProjectRef(t *testing.T) {
	if ref := projectRef("42"); ref["id"] != 42 {
		t.Fatalf("numeric project should use id, got %v", ref)
	}
	if ref := projectRef("My Project"); ref["name"] != "My Project" {
		t.Fatalf("non-numeric project should use name, got %v", ref)
	}
}

func TestValueNamePrefersHumanReadableFields(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"name field", map[string]any{"id": float64(10), "name": "assigned"}, "assigned"},
		{"label field", map[string]any{"id": float64(20), "label": "new"}, "new"},
		{"plain string", "open", "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValueName(tc.input); got != tc.want {
				t.Fatalf("ValueName(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRouteAcceptsFileCommands(t *testing.T) {
	cases := [][]string{
		{"issue", "file", "add", "123", "fix.patch"},
		{"issue", "file", "add", "123", "fix.patch", "screenshot.png"},
		{"issue", "file", "list", "123"},
		{"issue", "file", "get", "123", "5"},
		{"issue", "file", "get", "123", "5", "--output", "out.bin"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			call, code := route(args)
			if call == nil {
				t.Fatalf("expected a resolved command, got nil (exit code %d)", code)
			}
		})
	}
}

func TestRouteRejectsIncompleteFileCommands(t *testing.T) {
	cases := [][]string{
		{"issue", "file"},
		{"issue", "file", "add"},
		{"issue", "file", "add", "123"},
		{"issue", "file", "list"},
		{"issue", "file", "list", "123", "456"},
		{"issue", "file", "get", "123"},
		{"issue", "file", "unknown", "123"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			call, code := route(args)
			if call != nil {
				t.Fatal("expected no command for incomplete arguments")
			}
			if code != 2 {
				t.Fatalf("expected exit code 2, got %d", code)
			}
		})
	}
}

// recordingServer captures the last request and replies with the given JSON.
func recordingServer(t *testing.T, body string) (*mantis.Client, *http.Request, *[]byte) {
	t.Helper()
	var gotReq http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotReq = *r
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return mantis.NewClient(srv.URL, "token"), &gotReq, &gotBody
}

func TestIssueFileAddUploadsBase64Content(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fix.patch")
	if err := os.WriteFile(path, []byte("diff --git a b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call, code := route([]string{"issue", "file", "add", "123", path})
	if call == nil {
		t.Fatalf("expected a resolved command (exit code %d)", code)
	}

	client, req, body := recordingServer(t, `{}`)
	if _, err := call(client); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if req.Method != http.MethodPost {
		t.Fatalf("expected POST, got %s", req.Method)
	}
	if req.URL.Path != "/api/rest/issues/123/files" {
		t.Fatalf("unexpected path: %s", req.URL.Path)
	}

	var payload struct {
		Files []struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(*body, &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %s", err)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(payload.Files))
	}
	// The upload sends the base name, never the local directory.
	if payload.Files[0].Name != "fix.patch" {
		t.Fatalf("unexpected name: %s", payload.Files[0].Name)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Files[0].Content)
	if err != nil {
		t.Fatalf("content is not base64: %s", err)
	}
	if string(decoded) != "diff --git a b\n" {
		t.Fatalf("unexpected content: %q", decoded)
	}
}

func TestIssueFileAddReportsMissingPath(t *testing.T) {
	call, code := route([]string{"issue", "file", "add", "123", filepath.Join(t.TempDir(), "nope.txt")})
	if call == nil {
		t.Fatalf("expected a resolved command (exit code %d)", code)
	}
	client, _, _ := recordingServer(t, `{}`)
	_, err := call(client)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !strings.Contains(err.Error(), "nope.txt") {
		t.Fatalf("error should name the missing file, got: %s", err)
	}
}

func TestIssueFileGetWritesAttachment(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "downloaded.bin")
	call, code := route([]string{"issue", "file", "get", "123", "5", "--output", dest})
	if call == nil {
		t.Fatalf("expected a resolved command (exit code %d)", code)
	}

	content := base64.StdEncoding.EncodeToString([]byte("hello attachment"))
	client, req, _ := recordingServer(t, `{"files":[{"id":5,"filename":"fix.patch","size":16,"content":"`+content+`"}]}`)
	result, err := call(client)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if req.URL.Path != "/api/rest/issues/123/files/5" {
		t.Fatalf("unexpected path: %s", req.URL.Path)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("attachment was not written: %s", err)
	}
	if string(data) != "hello attachment" {
		t.Fatalf("unexpected file content: %q", data)
	}

	// Attachments are written owner-only rather than at the umask default.
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("expected mode 0600, got %04o", mode)
	}

	d, ok := result.(downloaded)
	if !ok {
		t.Fatalf("expected a downloaded result, got %T", result)
	}
	if d.path != dest || d.size != int64(len("hello attachment")) {
		t.Fatalf("unexpected download summary: %+v", d)
	}
}

func TestIssueFileGetRejectsEmptyResponse(t *testing.T) {
	call, _ := route([]string{"issue", "file", "get", "123", "5", "--output", filepath.Join(t.TempDir(), "x")})
	client, _, _ := recordingServer(t, `{"files":[]}`)
	if _, err := call(client); err == nil {
		t.Fatal("expected an error when the attachment is missing")
	}
}

func TestAttachmentPath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "taken.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// An explicit --output wins, even over an existing file.
	if got, err := attachmentPath(existing, "server.txt", "5"); err != nil || got != existing {
		t.Fatalf("explicit output: got %q, err %v", got, err)
	}

	// A server-supplied path is reduced to its base name.
	if got, err := attachmentPath("", "../../etc/passwd", "5"); err != nil || got != "passwd" {
		t.Fatalf("base name: got %q, err %v", got, err)
	}

	// An unusable filename asks for --output instead of guessing.
	if _, err := attachmentPath("", "", "5"); err == nil {
		t.Fatal("expected an error for an empty filename")
	}
}

func TestFileLine(t *testing.T) {
	file := map[string]any{
		"id":           float64(5),
		"filename":     "fix.patch",
		"size":         float64(1024),
		"content_type": "text/plain",
		"reporter":     map[string]any{"name": "vboctor"},
		"content":      "ZGlmZg==",
	}
	got := fileLine(file)
	want := "file #5 fix.patch (1024 bytes, text/plain) by vboctor"
	if got != want {
		t.Fatalf("fileLine() = %q, want %q", got, want)
	}
	if strings.Contains(got, "ZGlmZg==") {
		t.Fatal("base64 content must not appear in human-readable output")
	}
}
