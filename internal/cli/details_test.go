package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/config"
)

const detailedIssueJSON = `{
  "id":123,"summary":"Login fails","status":{"name":"new"},"project":{"name":"Web"},
  "reporter":{"name":"alice"},"handler":{"name":"bob"},"category":{"name":"General"},
  "priority":{"name":"high"},"severity":{"name":"major"},"resolution":{"name":"open"},
  "view_state":{"name":"private"},"created_at":"2026-09-01T12:00:00Z",
  "description":"First line\nSecond line","steps_to_reproduce":"Open <login>",
  "additional_information":"Only Safari",
  "notes":[{"id":9,"text":"Confirmed\nInvestigating","reporter":{"name":"alice"},"view_state":{"name":"private"}}],
  "attachments":[{"id":5,"filename":"fix.patch","size":3,"content_type":"text/plain","content":"YWJj"}]
}`

func TestIssueGetRendersDetailsAndPreservesJSON(t *testing.T) {
	for _, body := range []string{`{"issues":[` + detailedIssueJSON + `]}`, `{"issue":` + detailedIssueJSON + `}`} {
		client, req, _ := recordingServer(t, body)
		call, code := route([]string{"issue", "get", "123"})
		if call == nil {
			t.Fatalf("route failed: %d", code)
		}
		result, err := call(client)
		if err != nil || req.URL.Path != "/api/rest/issues/123" {
			t.Fatalf("request=%s err=%v", req.URL.Path, err)
		}
		var text bytes.Buffer
		if err := printResult(&text, result, config.Config{}); err != nil {
			t.Fatal(err)
		}
		want := "#123 [new] Web - Login fails\n" +
			"Reporter: alice\nAssignee: bob\nCategory: General\nPriority: high\nSeverity: major\nResolution: open\nVisibility: private\n" +
			"Created: 2026-09-01T12:00:00Z\n" +
			"\nDescription:\nFirst line\nSecond line\n" +
			"\nSteps to reproduce:\nOpen <login>\n" +
			"\nAdditional information:\nOnly Safari\n" +
			"\nNotes (1):\nnote #9 by alice [private]:\n  Confirmed\n  Investigating\n" +
			"\nAttachments (1):\nfile #5 fix.patch (3 bytes, text/plain)\n"
		if text.String() != want {
			t.Fatalf("unexpected details:\n%s\nwant:\n%s", text.String(), want)
		}
		text.Reset()
		if err := printResult(&text, result, config.Config{JSONOutput: true}); err != nil {
			t.Fatal(err)
		}
		var actual, original any
		if err := json.Unmarshal(text.Bytes(), &actual); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(body), &original); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, original) || strings.Contains(text.String(), `\u003c`) {
			t.Fatalf("JSON changed: %s", text.String())
		}
	}
}

func TestIssueDetailsOptionalFieldsAndCompactList(t *testing.T) {
	var issue map[string]any
	if err := json.Unmarshal([]byte(detailedIssueJSON), &issue); err != nil {
		t.Fatal(err)
	}
	for _, result := range []any{map[string]any{"issue": issue}, map[string]any{"issues": []any{issue}}} {
		var output bytes.Buffer
		if err := printResult(&output, result, config.Config{}); err != nil {
			t.Fatal(err)
		}
		if output.String() != "#123 [new] Web - Login fails\n" {
			t.Fatalf("list/write output should remain compact: %s", output.String())
		}
	}
	var output bytes.Buffer
	minimal := issueDetails{raw: map[string]any{"issues": []any{map[string]any{"id": 1, "summary": "Minimal"}}}}
	if err := printResult(&output, minimal, config.Config{}); err != nil || output.String() != "#1 []  - Minimal\n" {
		t.Fatalf("optional fields: output=%q err=%v", output.String(), err)
	}
}

type sectionFailureWriter struct {
	section string
	err     error
}

func (w sectionFailureWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), w.section) {
		return 0, w.err
	}
	return len(data), nil
}

func TestIssueDetailsPropagatesSectionWriteFailures(t *testing.T) {
	var issue map[string]any
	if err := json.Unmarshal([]byte(detailedIssueJSON), &issue); err != nil {
		t.Fatal(err)
	}
	want := errors.New("output failed")
	for _, section := range []string{"#123", "Assignee:", "Created:", "Description:", "Notes (", "note #", "Attachments (", "file #"} {
		result := issueDetails{raw: map[string]any{"issues": []any{issue}}}
		if err := printResult(sectionFailureWriter{section, want}, result, config.Config{}); !errors.Is(err, want) {
			t.Errorf("section %q: err=%v", section, err)
		}
	}
}
