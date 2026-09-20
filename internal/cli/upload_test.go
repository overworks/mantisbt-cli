package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/overworks/mantisbt-cli/internal/mantis"
)

func TestUploadTotalSizeLimit(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, limit := range []int{2, 5, 6, 7} {
		call, code := route([]string{"issue", "file", "add", "123", "--max-upload-size", fmt.Sprint(limit), first, second})
		if call == nil {
			t.Fatalf("route failed: %d", code)
		}
		calls := 0
		client := mantis.NewClient("https://mantis.example", "test-token")
		client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			data, err := io.ReadAll(r.Body)
			if err != nil || strings.Count(string(data), `"content":"YWJj"`) != 2 {
				t.Fatalf("unexpected upload: %s err=%v", data, err)
			}
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		_, err := call(client)
		if limit < 6 {
			if err == nil || !strings.Contains(err.Error(), "--max-upload-size") || calls != 0 {
				t.Fatalf("limit=%d calls=%d err=%v", limit, calls, err)
			}
		} else if err != nil || calls != 1 {
			t.Fatalf("limit=%d calls=%d err=%v", limit, calls, err)
		}
	}
}

func TestUploadRejectsNonRegularFiles(t *testing.T) {
	for _, path := range []string{t.TempDir(), os.DevNull} {
		if _, err := readUpload(path, 10); err == nil || !strings.Contains(err.Error(), "regular files") {
			t.Errorf("%s: expected regular-file error, got %v", path, err)
		}
	}
}

func TestUploadAcceptsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := readUpload(path, 0)
	if err != nil || data == nil || len(data) != 0 {
		t.Fatalf("data=%v err=%v", data, err)
	}
}
