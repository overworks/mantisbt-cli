package mantis

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRedirectsPreserveMethodBodyAndCredentials(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
			t.Run(fmt.Sprintf("%d/%s", status, method), func(t *testing.T) {
				calls := 0
				client := NewClient("https://mantis.example", "test-token")
				client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Method != method || r.Header.Get("Authorization") != "test-token" {
						t.Fatalf("redirect changed method or credentials: %s", r.Method)
					}
					if method == http.MethodPost || method == http.MethodPatch {
						body, err := io.ReadAll(r.Body)
						if err != nil || string(body) != `{"summary":"new"}` {
							t.Fatalf("request body=%q, err=%v", body, err)
						}
					}
					resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}
					if calls == 1 {
						resp.StatusCode = status
						resp.Header.Set("Location", "/canonical/issues/123")
					}
					return resp, nil
				})
				var payload any
				if method == http.MethodPost || method == http.MethodPatch {
					payload = map[string]any{"summary": "new"}
				}
				_, err := client.request(method, "/api/rest/issues/123", nil, payload)
				if status < 307 && method != http.MethodGet {
					if err == nil || !strings.Contains(err.Error(), "change request method") || calls != 1 {
						t.Fatalf("unsafe redirect: calls=%d, err=%v", calls, err)
					}
				} else if err != nil || calls != 2 {
					t.Fatalf("safe redirect: calls=%d, err=%v", calls, err)
				}
			})
		}
	}
}

func TestRedirectsNeverSendCredentialsToAnotherOrigin(t *testing.T) {
	for _, destination := range []string{
		"http://mantis.example/api/rest/issues", "https://other.example/api/rest/issues",
		"https://sub.mantis.example/api/rest/issues", "https://mantis.example:444/api/rest/issues",
		"https://user:password@mantis.example/api/rest/issues",
	} {
		t.Run(destination, func(t *testing.T) {
			calls := 0
			client := NewClient("https://mantis.example", "test-token")
			client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls > 1 {
					t.Fatal("unsafe redirect reached transport")
				}
				return &http.Response{StatusCode: 307, Header: http.Header{"Location": {destination}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})
			if _, err := client.Get("/api/rest/issues", nil); err == nil || !strings.Contains(err.Error(), "different origin") {
				t.Fatalf("expected redirect refusal, got %v", err)
			}
		})
	}
}

func TestRedirectLoopStops(t *testing.T) {
	calls := 0
	client := NewClient("https://mantis.example", "test-token")
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"/loop"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	if _, err := client.Get("/api/rest/issues", nil); err == nil || calls != 10 {
		t.Fatalf("calls=%d, err=%v", calls, err)
	}
}

func TestUnfollowedRedirectIsNotSuccess(t *testing.T) {
	for _, status := range []int{300, 302, 304} {
		client := NewClient("https://mantis.example", "test-token")
		client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})
		if _, err := client.Get("/api/rest/issues", nil); err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) {
			t.Errorf("status=%d, err=%v", status, err)
		}
	}
}
