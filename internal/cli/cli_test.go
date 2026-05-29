package cli

import "testing"

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
