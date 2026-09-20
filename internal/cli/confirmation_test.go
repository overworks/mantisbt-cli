package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/mantis"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeleteRequiresExplicitConfirmationForNonInteractiveInput(t *testing.T) {
	commands := []struct {
		name string
		args []string
		path string
	}{
		{"issue", []string{"issue", "delete", "123"}, "/api/rest/issues/123"},
		{"note", []string{"issue", "note", "delete", "123", "9"}, "/api/rest/issues/123/notes/9"},
	}
	for _, command := range commands {
		for _, kind := range []string{"pipe", "file", "null"} {
			for _, yes := range []bool{false, true} {
				name := command.name + "/" + kind
				if yes {
					name += "/--yes"
				}
				t.Run(name, func(t *testing.T) {
					input := nonInteractiveInput(t, kind)
					previous := os.Stdin
					os.Stdin = input
					t.Cleanup(func() { os.Stdin = previous })

					args := append([]string(nil), command.args...)
					if yes {
						args = append(args, "--yes")
					}
					call, code := route(args)
					if call == nil {
						t.Fatalf("command did not resolve (exit code %d)", code)
					}
					requests := 0
					client := mantis.NewClient("https://mantis.example.com", "test-token")
					client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						requests++
						if r.Method != http.MethodDelete || r.URL.Path != command.path {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						}
						return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
					})
					_, err := call(client)
					if yes {
						if err != nil || requests != 1 {
							t.Fatalf("explicit confirmation: err=%v, requests=%d", err, requests)
						}
					} else if err == nil || !strings.Contains(err.Error(), "--yes") || requests != 0 {
						t.Fatalf("non-interactive deletion must be rejected with --yes guidance: err=%v, requests=%d", err, requests)
					}
				})
			}
		}
	}
}

func nonInteractiveInput(t *testing.T, kind string) *os.File {
	t.Helper()
	var input *os.File
	var err error
	switch kind {
	case "pipe":
		var writer *os.File
		input, writer, err = os.Pipe()
		if err == nil {
			_, err = writer.WriteString("yes\n")
			writer.Close()
		}
	case "file":
		path := filepath.Join(t.TempDir(), "answer.txt")
		if err = os.WriteFile(path, []byte("yes\n"), 0o600); err == nil {
			input, err = os.Open(path)
		}
	case "null":
		input, err = os.Open(os.DevNull)
	default:
		t.Fatalf("unknown input kind %q", kind)
	}
	if input != nil {
		t.Cleanup(func() { input.Close() })
	}
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestConfirmRejectsNonInteractiveInputBeforePrompt(t *testing.T) {
	for _, kind := range []string{"pipe", "file", "null"} {
		t.Run(kind, func(t *testing.T) {
			input := nonInteractiveInput(t, kind)
			var output bytes.Buffer
			if err := confirm(input, &output, "Delete? "); err == nil {
				t.Fatal("expected non-interactive input to be rejected")
			}
			if output.Len() != 0 {
				t.Fatalf("must not prompt on non-interactive input: %q", output.String())
			}
			if kind != "null" {
				data, err := io.ReadAll(input)
				if err != nil || string(data) != "yes\n" {
					t.Fatalf("rejected input should not be consumed: %q, err=%v", data, err)
				}
			}
		})
	}
}
