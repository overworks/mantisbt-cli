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

func TestBuildURLWithRESTBase(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "rest root",
			base: "https://mantis.example.com/api/rest",
			want: "https://mantis.example.com/api/rest/users/me",
		},
		{
			name: "rest index",
			base: "https://mantis.example.com/api/rest/index.php",
			want: "https://mantis.example.com/api/rest/index.php/users/me",
		},
		{
			name: "rest index under subdirectory",
			base: "https://mantis.example.com/mantisbt/api/rest/index.php/",
			want: "https://mantis.example.com/mantisbt/api/rest/index.php/users/me",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.base, "token")
			if got := c.buildURL("/api/rest/users/me", nil); got != tc.want {
				t.Fatalf("unexpected url: %s", got)
			}
		})
	}
}
