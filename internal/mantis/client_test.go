package mantis

import "testing"

func TestBuildURL(t *testing.T) {
	c := NewClient("https://mantis.example.com/", "token")

	if got := c.buildURL("/api/rest/users/me", nil); got != "https://mantis.example.com/api/rest/users/me" {
		t.Fatalf("unexpected url: %s", got)
	}

	if got := c.buildURL("api/rest/issues", map[string]string{"page": "1"}); got != "https://mantis.example.com/api/rest/issues?page=1" {
		t.Fatalf("unexpected url with leading-slash normalisation and query: %s", got)
	}
}
