package cli

import (
	"strings"
	"testing"
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
		"--select", "id,summary", "--status", "resolved", "--search", "login"}
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
