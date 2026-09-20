package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRouteRejectsUnexpectedArguments(t *testing.T) {
	cases := [][]string{
		{},
		{"auth", "whoami", "extra"},
		{"issues", "list", "extra", "--status", "resolved"},
		{"issue", "create", "--summary", "s", "--description", "d", "--project", "1", "--category", "c", "extra"},
		{"issue", "update", "123", "--summary", "changed", "typo", "--status", "resolved"},
		{"issue", "delete", "123", "--yes", "extra"},
		{"issue", "note", "add", "123", "--text", "note", "extra", "--private"},
		{"issue", "note", "delete", "123", "9", "--yes", "extra"},
		{"issue", "file", "get", "123", "5", "extra", "--output", "out"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			call, code := route(args)
			if call != nil || code != 2 {
				t.Fatalf("expected usage error, got call=%v, code=%d", call != nil, code)
			}
		})
	}
}

func TestRouteRejectsInvalidNumbers(t *testing.T) {
	cases := [][]string{
		{"issue", "update", "bad", "--status", "resolved"},
		{"issue", "delete", "0", "--yes"},
		{"issue", "note", "add", "-1", "--text", "note"},
		{"issue", "note", "delete", "123", "0", "--yes"},
		{"issue", "note", "delete", "bad", "9", "--yes"},
		{"issue", "file", "add", "bad", "file.txt"},
		{"issue", "file", "list", "bad"},
		{"issue", "file", "get", "123", "bad"},
		{"issue", "file", "get", "bad", "5"},
		{"issues", "list", "--page", "0"},
		{"issues", "list", "--page", "-1"},
		{"issues", "list", "--page-size", "0"},
		{"issues", "list", "--page-size", "1.5"},
		{"issues", "list", "--project", "name"},
		{"issues", "list", "--project", "-1"},
		{"issues", "list", "--filter", "0"},
		{"issues", "list", "--filter", "typo"},
	}
	for _, id := range []string{"0", "bad", "1/notes/9", "1?x=2", "1#fragment", "9223372036854775808"} {
		cases = append(cases, []string{"issue", "get", id})
	}
	for _, project := range []string{"0", "-1", "+1", "9223372036854775808"} {
		cases = append(cases, []string{"issue", "create", "--summary", "s", "--description", "d", "--category", "c", "--project", project})
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			call, code := route(args)
			if call != nil || code != 2 {
				t.Fatalf("expected usage error, got call=%v, code=%d", call != nil, code)
			}
		})
	}
}

func TestRouteAcceptsValidReferences(t *testing.T) {
	cases := [][]string{
		{"issues", "list", "--project", "0", "--filter", "12"},
		{"issues", "list", "--filter", "unassigned"},
		{"issue", "get", "00123"},
		{"issue", "create", "--summary", "s", "--description", "d", "--category", "c", "--project", "My Project"},
		{"issue", "create", "--summary", "s", "--description", "d", "--category", "c", "--project", "42"},
	}
	for _, args := range cases {
		if call, code := route(args); call == nil || code != 0 {
			t.Errorf("%v: expected valid command, code=%d", args, code)
		}
	}
}

func TestIssueUpdateSendsOnlyExplicitFields(t *testing.T) {
	call, code := route([]string{"issue", "update", "123", "--description", "", "--status", "resolved"})
	if call == nil {
		t.Fatalf("command did not resolve (exit code %d)", code)
	}
	client, req, raw := recordingServer(t, `{}`)
	if _, err := call(client); err != nil {
		t.Fatal(err)
	}
	if req.Method != http.MethodPatch || req.URL.Path != "/api/rest/issues/123" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(*raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["description"] != "" || body["status"].(map[string]any)["name"] != "resolved" {
		t.Fatalf("partial update changed unspecified fields or lost an explicit empty value: %v", body)
	}
}
