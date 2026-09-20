package mantis

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type countedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *countedBody) Close() error { b.closed = true; return nil }

func TestResponseSizeLimit(t *testing.T) {
	const payload = `{"message":"hello"}`
	for _, knownLength := range []bool{false, true} {
		for _, limit := range []int64{int64(len(payload) - 1), int64(len(payload)), int64(len(payload) + 1)} {
			body := &countedBody{Reader: strings.NewReader(payload)}
			client := NewClient("https://mantis.example", "test-token")
			client.MaxResponseBytes = limit
			client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				length := int64(-1)
				if knownLength {
					length = int64(len(payload))
				}
				return &http.Response{StatusCode: 200, ContentLength: length, Body: body, Request: r}, nil
			})
			_, err := client.Get("/api/rest/issues", nil)
			if limit < int64(len(payload)) {
				if err == nil || !strings.Contains(err.Error(), "--max-response-size") {
					t.Fatalf("expected size error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !body.closed || int64(body.read) > limit+1 {
				t.Fatalf("response not bounded/closed: read=%d closed=%t", body.read, body.closed)
			}
		}
	}
}

func TestHTTPErrorBodyIsBounded(t *testing.T) {
	body := &countedBody{Reader: strings.NewReader(strings.Repeat("x", int(maxErrorBodyBytes)+1000))}
	client := NewClient("https://mantis.example", "test-token")
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: body, Request: r}, nil
	})
	_, err := client.Get("/api/rest/issues", nil)
	if err == nil || !strings.Contains(err.Error(), "500 Internal Server Error") || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected truncated HTTP error, got %v", err)
	}
	if !body.closed || int64(body.read) != maxErrorBodyBytes+1 {
		t.Fatalf("read=%d closed=%t", body.read, body.closed)
	}
}
