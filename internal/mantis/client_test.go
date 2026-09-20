package mantis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildURL(t *testing.T) {
	c := NewClient("https://mantis.example.com/", "token")

	if got, err := c.buildURL("/api/rest/users/me", nil); err != nil || got != "https://mantis.example.com/api/rest/users/me" {
		t.Fatalf("unexpected url: %s", got)
	}

	if got, err := c.buildURL("api/rest/issues", map[string]string{"page": "1"}); err != nil || got != "https://mantis.example.com/api/rest/issues?page=1" {
		t.Fatalf("unexpected url with leading-slash normalisation and query: %s", got)
	}
}

func TestBuildURLWithRESTBase(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		path   string
		params map[string]string
		want   string
	}{
		{
			name: "escaped installation path",
			base: "https://mantis.example.com/mantis%20bt/folder%2Fname/",
			path: "/api/rest/users/me",
			want: "https://mantis.example.com/mantis%20bt/folder%2Fname/api/rest/users/me",
		},
		{
			name: "rest root",
			base: "https://mantis.example.com/api/rest",
			path: "/api/rest/users/me",
			want: "https://mantis.example.com/api/rest/users/me",
		},
		{
			name: "rest index",
			base: "https://mantis.example.com/api/rest/index.php",
			path: "/api/rest/users/me",
			want: "https://mantis.example.com/api/rest/index.php/users/me",
		},
		{
			name: "rest index under subdirectory",
			base: "https://mantis.example.com/mantisbt/api/rest/index.php/",
			path: "/api/rest/users/me",
			want: "https://mantis.example.com/mantisbt/api/rest/index.php/users/me",
		},
		{
			name:   "rest root with relative path and query",
			base:   "https://mantis.example.com/api/rest",
			path:   "api/rest/issues",
			params: map[string]string{"page": "1"},
			want:   "https://mantis.example.com/api/rest/issues?page=1",
		},
		{
			name:   "rest index with relative path and query",
			base:   "https://mantis.example.com/api/rest/index.php",
			path:   "api/rest/issues",
			params: map[string]string{"page": "1"},
			want:   "https://mantis.example.com/api/rest/index.php/issues?page=1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.base, "token")
			if got, err := c.buildURL(tc.path, tc.params); err != nil || got != tc.want {
				t.Fatalf("unexpected url: %s", got)
			}
		})
	}
}

func TestRequestsSendExpectedHeadersAndPayload(t *testing.T) {
	payload := map[string]any{"summary": "<fix>", "status": map[string]any{"name": "resolved"}}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method || r.URL.Path != "/api/rest/index.php/issues/123" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "test-token" || r.Header.Get("Accept") != "application/json" {
					t.Error("request is missing authentication or JSON accept header")
				}
				if method == http.MethodPost || method == http.MethodPatch {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("Content-Type=%q", got)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, payload) {
						t.Errorf("unexpected JSON payload: %v, err=%v", body, err)
					}
				} else if body, err := io.ReadAll(r.Body); err != nil || len(body) != 0 {
					t.Errorf("unexpected request body: %q, err=%v", body, err)
				}
				if method == http.MethodGet && r.URL.Query().Get("query") != "a&b /한" {
					t.Errorf("query was not encoded correctly: %s", r.URL.RawQuery)
				}
				if method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"issues":[{"id":123}]}`)
			}))
			t.Cleanup(server.Close)
			client := NewClient(server.URL+"/api/rest/index.php", "test-token")
			var result any
			var err error
			switch method {
			case http.MethodGet:
				result, err = client.Get("/api/rest/issues/123", map[string]string{"query": "a&b /한"})
			case http.MethodPost:
				result, err = client.Post("/api/rest/issues/123", payload)
			case http.MethodPatch:
				result, err = client.Patch("/api/rest/issues/123", payload)
			case http.MethodDelete:
				result, err = client.Delete("/api/rest/issues/123")
			}
			if err != nil {
				t.Fatal(err)
			}
			if method == http.MethodDelete {
				if result != nil {
					t.Fatalf("204 response should return nil, got %v", result)
				}
			} else {
				want := map[string]any{"issues": []any{map[string]any{"id": float64(123)}}}
				if !reflect.DeepEqual(result, want) {
					t.Fatalf("decoded result=%v, want %v", result, want)
				}
			}
		})
	}
}

func TestRequestReportsHTTPFailures(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"request failed"}`)
			}))
			t.Cleanup(server.Close)
			result, err := NewClient(server.URL, "test-token").Get("/api/rest/issues", nil)
			var apiErr *Error
			if result != nil || !errors.As(err, &apiErr) {
				t.Fatalf("expected an API error, got result=%v, err=%v", result, err)
			}
			for _, part := range []string{fmt.Sprint(status), http.StatusText(status), "request failed"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q is missing %q", err, part)
				}
			}
		})
	}
}

func TestRequestHandlesEmptyAndInvalidJSONResponses(t *testing.T) {
	for _, body := range []string{"", "null", "<html>login</html>", "{", " "} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			}))
			t.Cleanup(server.Close)
			result, err := NewClient(server.URL, "test-token").Get("/api/rest/issues", nil)
			if body == "" || body == "null" {
				if err != nil || result != nil {
					t.Fatalf("empty result=%v, err=%v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "non-JSON response") {
				t.Fatalf("expected invalid JSON error, got %v", err)
			}
		})
	}
}

func TestRequestTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	client := NewClient(server.URL, "test-token")
	client.HTTP.Timeout = 50 * time.Millisecond
	result, err := client.Get("/api/rest/issues", nil)
	if result != nil || err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("expected timeout, got result=%v, err=%v", result, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInvalidRequestsNeverReachTransport(t *testing.T) {
	for _, invalidURL := range []bool{false, true} {
		client := NewClient("https://mantis.example.com", "test-token")
		if invalidURL {
			client = NewClient("http://[invalid", "test-token")
		}
		client.HTTP.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("invalid request reached the transport")
			return nil, errors.New("unexpected request")
		})
		var err error
		if invalidURL {
			_, err = client.Get("/api/rest/issues", nil)
		} else {
			_, err = client.Post("/api/rest/issues", map[string]any{"unsupported": make(chan int)})
		}
		if err == nil {
			t.Fatal("expected a request construction error")
		}
	}
}

type failedResponseBody struct{ closed bool }

func (*failedResponseBody) Read([]byte) (int, error) { return 0, errors.New("response read failed") }
func (b *failedResponseBody) Close() error           { b.closed = true; return nil }

func TestRequestClosesBodyOnReadFailure(t *testing.T) {
	body := &failedResponseBody{}
	client := NewClient("https://mantis.example.com", "test-token")
	client.HTTP.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})
	if _, err := client.Get("/api/rest/issues", nil); err == nil || !strings.Contains(err.Error(), "response read failed") {
		t.Fatalf("expected response read failure, got %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed after a read failure")
	}
}
