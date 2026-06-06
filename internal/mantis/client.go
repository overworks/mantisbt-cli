package mantis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Error is returned when a MantisBT API request fails.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

func newError(format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

// Client is a thin HTTP client for the MantisBT REST API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient builds a client with a sensible default timeout.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs a GET request and decodes the JSON response.
func (c *Client) Get(path string, params map[string]string) (any, error) {
	return c.request(http.MethodGet, path, params, nil)
}

// Post performs a POST request with a JSON body.
func (c *Client) Post(path string, payload any) (any, error) {
	return c.request(http.MethodPost, path, nil, payload)
}

// Patch performs a PATCH request with a JSON body.
func (c *Client) Patch(path string, payload any) (any, error) {
	return c.request(http.MethodPatch, path, nil, payload)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string) (any, error) {
	return c.request(http.MethodDelete, path, nil, nil)
}

func (c *Client) request(method, path string, params map[string]string, payload any) (any, error) {
	endpoint := c.buildURL(path, params)

	var body io.Reader
	var contentType string
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, newError("failed to encode request payload: %v", err)
		}
		body = bytes.NewReader(data)
		contentType = "application/json"
	}

	req, err := http.NewRequest(strings.ToUpper(method), endpoint, body)
	if err != nil {
		return nil, newError("%v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.Token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, newError("%v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newError("%v", err)
	}

	if resp.StatusCode >= 400 {
		return nil, newError("%d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
	}

	if len(respBody) == 0 {
		return nil, nil
	}

	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, newError("MantisBT returned a non-JSON response")
	}
	return result, nil
}

func (c *Client) buildURL(path string, params map[string]string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	route := path
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	if isRESTBaseURL(base) && strings.HasPrefix(route, "/api/rest/") {
		route = strings.TrimPrefix(route, "/api/rest")
	}
	full := base + route
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		full = full + "?" + q.Encode()
	}
	return full
}

func isRESTBaseURL(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	return strings.HasSuffix(path, "/api/rest") || strings.HasSuffix(path, "/api/rest/index.php")
}
