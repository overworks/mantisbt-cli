package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMainHelpSucceedsWithoutConfiguration(t *testing.T) {
	t.Setenv("MANTISBT_URL", "")
	t.Setenv("MANTISBT_TOKEN", "")
	commands := [][]string{
		{}, {"auth"}, {"auth", "whoami"}, {"issues"}, {"issues", "list"},
		{"issue"}, {"issue", "get"}, {"issue", "create"}, {"issue", "update"}, {"issue", "delete"},
		{"issue", "note"}, {"issue", "note", "add"}, {"issue", "note", "delete"},
		{"issue", "file"}, {"issue", "file", "add"}, {"issue", "file", "list"}, {"issue", "file", "get"},
		{"issue", "get", "123"}, {"issue", "update", "123"}, {"issue", "delete", "123"},
		{"issue", "note", "delete", "123"}, {"issue", "note", "delete", "123", "9"},
		{"issue", "file", "get", "123"}, {"issue", "file", "get", "123", "5"},
	}
	for _, command := range commands {
		for _, help := range []string{"-h", "--help"} {
			args := append(append([]string(nil), command...), help)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				code, _, stderr := runMain(t, args, false)
				if code != 0 || !strings.Contains(stderr, "usage:") || strings.Contains(stderr, "missing configuration") {
					t.Fatalf("help failed: code=%d, stderr=%q", code, stderr)
				}
			})
		}
	}
}

func TestMainReturnsUsageErrors(t *testing.T) {
	t.Setenv("MANTISBT_URL", "")
	t.Setenv("MANTISBT_TOKEN", "")
	for _, args := range [][]string{{}, {"--unknown"}, {"issue", "get"}, {"issue", "update", "123"}, {"auth", "whoami", "extra"}} {
		code, _, stderr := runMain(t, args, false)
		if code != 2 || stderr == "" || strings.Contains(stderr, "missing configuration") {
			t.Errorf("%v: code=%d, stderr=%q", args, code, stderr)
		}
	}
}

func TestMainReturnsFailureWhenOutputCannotBeWritten(t *testing.T) {
	client, _, _ := recordingServer(t, `{"user":{"id":1,"name":"Alice"}}`)
	cases := [][]string{
		{"--version"},
		{"--url", client.BaseURL, "--token", "test-token", "auth", "whoami"},
		{"--url", client.BaseURL, "--token", "test-token", "--json", "auth", "whoami"},
	}
	for _, args := range cases {
		code, _, stderr := runMain(t, args, true)
		if code != 1 || !strings.Contains(stderr, "failed to write output") {
			t.Errorf("%v: expected output failure, code=%d, stderr=%q", args, code, stderr)
		}
	}
}

func TestHelpFlagCanBeAFieldValue(t *testing.T) {
	call, code := route([]string{"issue", "update", "123", "--summary", "--help"})
	if call == nil || code != 0 {
		t.Fatalf("--help used as a field value must not trigger help: code=%d", code)
	}
}

func TestMainReportsMissingConfiguration(t *testing.T) {
	t.Setenv("MANTISBT_URL", "")
	t.Setenv("MANTISBT_TOKEN", "")
	code, stdout, stderr := runMain(t, []string{"auth", "whoami"}, false)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "missing configuration") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestMainValidatesURLAndSizeLimits(t *testing.T) {
	for _, flags := range [][]string{
		{"--url", "https://mantis.example/?query=value"},
		{"--url", "https://mantis.example/#dashboard"},
		{"--max-response-size", "0"}, {"--max-response-size", "-1"},
		{"--max-response-size", "9223372036854775807"},
	} {
		args := append([]string{"--url", "https://unused.example", "--token", "test-token"}, flags...)
		code, stdout, stderr := runMain(t, append(args, "auth", "whoami"), false)
		if code != 2 || stdout != "" || stderr == "" {
			t.Fatalf("flags=%v code=%d stdout=%q stderr=%q", flags, code, stdout, stderr)
		}
	}
}

func TestMainAppliesResponseSizeLimit(t *testing.T) {
	client, _, _ := recordingServer(t, `{"user":{"id":1,"name":"Alice"}}`)
	code, stdout, stderr := runMain(t, []string{"--url", client.BaseURL, "--token", "test-token", "--max-response-size", "10", "auth", "whoami"}, false)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "--max-response-size") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMainReportsAPIFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"request failed"}`)
			}))
			t.Cleanup(server.Close)
			code, stdout, stderr := runMain(t, []string{"--url", server.URL, "--token", "test-token", "auth", "whoami"}, false)
			if code != 1 || stdout != "" || !strings.Contains(stderr, fmt.Sprint(status)) {
				t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestMainJSONSuccessUsesExplicitConnectionFlags(t *testing.T) {
	t.Setenv("MANTISBT_URL", "https://unused.example")
	t.Setenv("MANTISBT_TOKEN", "unused-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/rest/users/me" || r.Header.Get("Authorization") != "flag-token" {
			t.Errorf("wrong endpoint or token: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"user":{"id":1,"name":"Alice"}}`)
	}))
	t.Cleanup(server.Close)
	code, stdout, stderr := runMain(t, []string{"--url", server.URL, "--token", "flag-token", "--json", "auth", "whoami"}, false)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
	var response struct {
		User struct{ Name string }
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil || response.User.Name != "Alice" {
		t.Fatalf("unexpected JSON output: %q, err=%v", stdout, err)
	}
}

// runMain captures real CLI output without pipes that could block on long help.
// Tests using process-wide standard streams must not run in parallel.
func runMain(t *testing.T, args []string, failStdout bool) (int, string, string) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	if failStdout {
		path := stdout.Name()
		stdout.Close()
		stdout, err = os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { stdout.Close() })
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stderr.Close() })
	previousOut, previousErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	defer func() { os.Stdout, os.Stderr = previousOut, previousErr }()
	code := Main(args)
	read := func(file *os.File) string {
		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	return code, read(stdout), read(stderr)
}
