package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/mantis"
)

func TestAllPagesPreservesQueriesAndFiltersAfterPagination(t *testing.T) {
	call, code := route([]string{"issues", "list", "--all", "--page-size", "20", "--project", "3", "--filter", "assigned",
		"--select", "id, summary, status", "--status", "resolved", "--search", "LOGIN"})
	if call == nil {
		t.Fatalf("route failed: %d", code)
	}
	pages := []string{
		`{"issues":[{"id":1,"summary":"Other","status":{"name":"new"}}]}`,
		`{"issues":[{"id":2,"summary":"LOGIN broken","status":{"name":"resolved"}}]}`,
		`{"issues":[]}`,
	}
	client := mantis.NewClient("https://mantis.example", "test-token")
	calls := 0
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > len(pages) {
			t.Fatal("pagination did not stop at empty page")
		}
		query := r.URL.Query()
		for key, want := range map[string]string{"page": strconv.Itoa(calls), "page_size": "20", "project_id": "3", "filter_id": "assigned", "select": "id,summary,status"} {
			if query.Get(key) != want {
				t.Errorf("query %s=%q, want %q", key, query.Get(key), want)
			}
		}
		if query.Has("status") || query.Has("search") || query.Has("all") || query.Has("max-pages") {
			t.Fatal("client-only options were forwarded")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(pages[calls-1]))}, nil
	})
	result, err := call(client)
	if err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	issues := result.(map[string]any)["issues"].([]any)
	if len(issues) != 1 || issues[0].(map[string]any)["id"] != float64(2) {
		t.Fatalf("unexpected filtered result: %v", result)
	}
}

func TestAllPagesRejectsIncompleteOrUnsafeTraversals(t *testing.T) {
	for _, tc := range []struct {
		name, second, want string
		maxPages           int64
	}{
		{"repeated page", `{"issues":[{"id":1}]}`, "repeats issue", 10},
		{"invalid envelope", `[]`, "issues array", 10},
		{"missing array", `{}`, "issues array", 10},
		{"null array", `{"issues":null}`, "issues array", 10},
		{"invalid item", `{"issues":[null]}`, "issue object", 10},
		{"missing id", `{"issues":[{"summary":"missing"}]}`, "valid id", 10},
		{"invalid id", `{"issues":[{"id":0}]}`, "valid id", 10},
		{"page limit", `{"issues":[{"id":2}]}`, "--max-pages", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := mantis.NewClient("https://mantis.example", "test-token")
			client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 2 {
					t.Fatal("expected pagination to fail within two requests")
				}
				body := `{"issues":[{"id":1}]}`
				if calls == 2 {
					body = tc.second
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			result, err := listAllIssues(client, map[string]string{"page_size": "1"}, "", "", tc.maxPages)
			if result != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("result=%v err=%v", result, err)
			}
		})
	}
}

func TestAllPagesCombinedSizeIsBounded(t *testing.T) {
	client := mantis.NewClient("https://mantis.example", "test-token")
	client.MaxResponseBytes = 60
	calls := 0
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 2 {
			t.Fatal("combined size limit should stop after the second page")
		}
		body := fmt.Sprintf(`{"issues":[{"id":%d,"summary":"abcdefghij"}]}`, calls)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result, err := listAllIssues(client, map[string]string{"page_size": "1"}, "", "", 10)
	if result != nil || err == nil || !strings.Contains(err.Error(), "combined result exceeds") {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestAllPagesBoundsIDsEvenWhenEveryIssueIsFilteredOut(t *testing.T) {
	client := mantis.NewClient("https://mantis.example", "test-token")
	client.MaxResponseBytes = 32
	calls := 0
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 5 {
			t.Fatal("filtered IDs were not bounded")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"issues":[{"id":%d}]}`, calls)))}, nil
	})
	result, err := listAllIssues(client, nil, "resolved", "", 10)
	if result != nil || err == nil || !strings.Contains(err.Error(), "tracked issue IDs") {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestMainAllPagesJSONAndFailures(t *testing.T) {
	for _, failSecondPage := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("page") {
			case "1":
				fmt.Fprint(w, `{"issues":[{"id":1,"summary":"first"}]}`)
			case "2":
				if failSecondPage {
					w.WriteHeader(503)
					fmt.Fprint(w, `{"message":"offline"}`)
					return
				}
				fmt.Fprint(w, `{"issues":[{"id":2,"summary":"second"}]}`)
			case "3":
				fmt.Fprint(w, `{"issues":[]}`)
			default:
				t.Errorf("unexpected page: %s", r.URL.RawQuery)
				w.WriteHeader(400)
			}
		}))
		code, stdout, stderr := runMain(t, []string{"--url", server.URL, "--token", "test-token", "--json", "issues", "list", "--all"}, false)
		server.Close()
		if failSecondPage {
			if code != 1 || stdout != "" || !strings.Contains(stderr, "page 2") || !strings.Contains(stderr, "503") {
				t.Fatalf("partial result escaped: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		} else {
			var output struct{ Issues []struct{ ID int } }
			if err := json.Unmarshal([]byte(stdout), &output); err != nil || code != 0 || stderr != "" || len(output.Issues) != 2 {
				t.Fatalf("code=%d stdout=%q stderr=%q err=%v", code, stdout, stderr, err)
			}
		}
	}
}

func TestAllPagesEmptyResult(t *testing.T) {
	client, _, _ := recordingServer(t, `{"issues":[]}`)
	result, err := listAllIssues(client, nil, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil || string(data) != `{"issues":[]}` {
		t.Fatalf("result=%s err=%v", data, err)
	}
}

func TestAllPagesValidatesOptions(t *testing.T) {
	for _, flags := range [][]string{
		{"--all", "--page", "1"}, {"--max-pages", "5"}, {"--all", "--max-pages", "0"},
		{"--all", "--max-pages", "-1"}, {"--all", "--max-pages", "9223372036854775807"},
		{"--all", "--select", "summary"},
	} {
		if call, code := route(append([]string{"issues", "list"}, flags...)); call != nil || code != 2 {
			t.Errorf("flags=%v call=%t code=%d", flags, call != nil, code)
		}
	}
}
