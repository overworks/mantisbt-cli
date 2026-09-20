package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/config"
)

type failedOutputWriter struct{ err error }

func (w failedOutputWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPrintResultPropagatesWriteFailures(t *testing.T) {
	cases := []struct {
		name   string
		result any
		json   bool
	}{
		{"success", nil, false},
		{"issue", map[string]any{"issue": map[string]any{"id": 1}}, false},
		{"issues", map[string]any{"issues": []any{map[string]any{"id": 1}}}, false},
		{"note", map[string]any{"note": map[string]any{"id": 1}}, false},
		{"files", map[string]any{"files": []any{map[string]any{"id": 1}}}, false},
		{"user", map[string]any{"user": map[string]any{"id": 1}}, false},
		{"download", downloaded{raw: map[string]any{}, path: "file", size: 3}, false},
		{"download JSON", downloaded{raw: map[string]any{}, path: "file", size: 3}, true},
		{"JSON", map[string]any{"id": 1}, true},
		{"fallback", map[string]any{"other": "value"}, false},
		{"issues fallback", map[string]any{"issues": "unexpected"}, false},
		{"files fallback", map[string]any{"files": "unexpected"}, false},
	}
	want := errors.New("output unavailable")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := printResult(failedOutputWriter{want}, tc.result, config.Config{JSONOutput: tc.json})
			if !errors.Is(err, want) {
				t.Fatalf("expected write error, got %v", err)
			}
		})
	}
}

func TestPrintResultPreservesOutput(t *testing.T) {
	issue := map[string]any{"id": float64(123), "summary": "Login fails", "status": map[string]any{"name": "new"}, "project": map[string]any{"name": "Web"}}
	cases := []struct {
		name   string
		result any
		json   bool
		want   string
	}{
		{"success", nil, false, "OK\n"},
		{"issue", map[string]any{"issue": issue}, false, "#123 [new] Web - Login fails\n"},
		{"issues", map[string]any{"issues": []any{issue}}, false, "#123 [new] Web - Login fails\n"},
		{"empty issues", map[string]any{"issues": []any{}}, false, ""},
		{"user", map[string]any{"user": map[string]any{"id": float64(1), "real_name": "Alice"}}, false, "Alice (1)\n"},
		{"note", map[string]any{"note": map[string]any{"id": float64(2), "text": "done", "reporter": map[string]any{"name": "Alice"}}}, false, "note #2 by Alice: done\n"},
		{"download", downloaded{path: "file.txt", size: 3}, false, "wrote file.txt (3 bytes)\n"},
		{"raw JSON", map[string]any{"text": "<tag>&"}, true, "{\n  \"text\": \"<tag>&\"\n}\n"},
		{"download JSON", downloaded{raw: map[string]any{"files": []any{}}, path: "file.txt", size: 3}, true, "{\n  \"files\": []\n}\n"},
		{"null JSON", nil, true, "null\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := printResult(&output, tc.result, config.Config{JSONOutput: tc.json}); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != tc.want {
				t.Fatalf("output=%q, want %q", got, tc.want)
			}
		})
	}
}
