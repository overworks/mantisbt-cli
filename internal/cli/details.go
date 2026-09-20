package cli

import (
	"fmt"
	"io"
	"strings"
)

// issueDetails selects detailed human output for issue get while retaining the
// original response for --json. Create/update responses remain compact.
type issueDetails struct{ raw any }

func printIssueDetails(w io.Writer, result any) error {
	m, ok := result.(map[string]any)
	if !ok {
		return printJSON(w, result)
	}
	if issue, ok := m["issue"].(map[string]any); ok {
		return printDetailedIssue(w, issue)
	}
	if issues, ok := m["issues"].([]any); ok {
		for i, item := range issues {
			issue, ok := item.(map[string]any)
			if !ok {
				return printJSON(w, item)
			}
			if i > 0 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
			if err := printDetailedIssue(w, issue); err != nil {
				return err
			}
		}
		return nil
	}
	return printJSON(w, result)
}

func printDetailedIssue(w io.Writer, issue map[string]any) error {
	if err := printIssue(w, issue); err != nil {
		return err
	}
	for _, field := range []struct{ key, label string }{
		{"reporter", "Reporter"}, {"handler", "Assignee"}, {"category", "Category"},
		{"priority", "Priority"}, {"severity", "Severity"}, {"resolution", "Resolution"},
		{"reproducibility", "Reproducibility"}, {"view_state", "Visibility"},
	} {
		if value := ValueName(issue[field.key]); value != "" {
			if _, err := fmt.Fprintf(w, "%s: %s\n", field.label, value); err != nil {
				return err
			}
		}
	}
	for _, field := range []struct{ label, value string }{
		{"Created", firstNonEmpty(issue, "created_at", "date_submitted")},
		{"Updated", firstNonEmpty(issue, "updated_at", "date_updated")},
	} {
		if field.value != "" {
			if _, err := fmt.Fprintf(w, "%s: %s\n", field.label, field.value); err != nil {
				return err
			}
		}
	}
	for _, field := range []struct{ key, label string }{
		{"description", "Description"}, {"steps_to_reproduce", "Steps to reproduce"},
		{"additional_information", "Additional information"},
	} {
		if text := valueToString(issue[field.key]); text != "" {
			if _, err := fmt.Fprintf(w, "\n%s:\n%s\n", field.label, strings.TrimRight(text, "\r\n")); err != nil {
				return err
			}
		}
	}
	if notes, ok := issue["notes"].([]any); ok && len(notes) > 0 {
		if _, err := fmt.Fprintf(w, "\nNotes (%d):\n", len(notes)); err != nil {
			return err
		}
		for _, item := range notes {
			note, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := printDetailedNote(w, note); err != nil {
				return err
			}
		}
	}
	files, ok := issue["attachments"].([]any)
	if !ok {
		files, _ = issue["files"].([]any)
	}
	if len(files) > 0 {
		if _, err := fmt.Fprintf(w, "\nAttachments (%d):\n", len(files)); err != nil {
			return err
		}
		return printFiles(w, files)
	}
	return nil
}

func printDetailedNote(w io.Writer, note map[string]any) error {
	id := firstNonEmpty(note, "id")
	if id == "" {
		id = "unknown"
	}
	header := fmt.Sprintf("note #%s by %s", id, ValueName(note["reporter"]))
	if visibility := ValueName(note["view_state"]); visibility != "" {
		header += " [" + visibility + "]"
	}
	if created := firstNonEmpty(note, "created_at", "date_submitted"); created != "" {
		header += " (" + created + ")"
	}
	text := strings.TrimRight(valueToString(note["text"]), "\r\n")
	_, err := fmt.Fprintf(w, "%s:\n  %s\n", header, strings.ReplaceAll(text, "\n", "\n  "))
	return err
}
