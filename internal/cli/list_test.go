package cli

import "testing"

func TestRouteRejectsSelectionMissingFilterFields(t *testing.T) {
	cases := [][]string{
		{"--select", "id,summary", "--status", "resolved"},
		{"--select", "id,status", "--search", "login"},
		{"--select", "id", "--status", "resolved", "--search", "login"},
		{"--select", "id,,summary"},
		{"--select", " "},
	}
	for _, flags := range cases {
		call, code := route(append([]string{"issues", "list"}, flags...))
		if call != nil || code != 2 {
			t.Errorf("flags %v: expected usage error, got call=%v, code=%d", flags, call != nil, code)
		}
	}
}

func TestIssuesListSelectionAndLocalFilters(t *testing.T) {
	for _, selection := range []string{"", "id,summary,status", "id, summary, status"} {
		t.Run(selection, func(t *testing.T) {
			args := []string{"issues", "list", "--project", "3", "--filter", "assigned",
				"--page", "2", "--page-size", "20", "--status", "resolved", "--search", "LOGIN"}
			if selection != "" {
				args = append(args, "--select", selection)
			}
			call, code := route(args)
			if call == nil {
				t.Fatalf("command did not resolve (exit code %d)", code)
			}
			client, req, _ := recordingServer(t, `{"issues":[
				{"id":1,"summary":"Login fails","status":{"name":"resolved"}},
				{"id":2,"summary":"Login is slow","status":{"name":"new"}},
				{"id":3,"summary":"Logout fails","status":{"name":"resolved"}}
			]}`)
			result, err := call(client)
			if err != nil {
				t.Fatal(err)
			}
			query := req.URL.Query()
			wantSelect := ""
			if selection != "" {
				wantSelect = "id,summary,status"
			}
			for key, want := range map[string]string{"project_id": "3", "filter_id": "assigned", "page": "2", "page_size": "20", "select": wantSelect} {
				if got := query.Get(key); got != want {
					t.Errorf("query %s=%q, want %q", key, got, want)
				}
			}
			if query.Has("status") || query.Has("search") {
				t.Fatal("local filters must not be sent to the server")
			}
			issues := result.(map[string]any)["issues"].([]any)
			if len(issues) != 1 || issues[0].(map[string]any)["id"] != float64(1) {
				t.Fatalf("unexpected filtered result: %v", issues)
			}
		})
	}
}
