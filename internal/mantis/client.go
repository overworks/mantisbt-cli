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

	"github.com/overworks/mantisbt-cli/internal/config"
)

// Error is returned when a MantisBT API request fails.
type Error struct{ msg string }

// DefaultMaxResponseBytes bounds the JSON returned by a single API request.
const DefaultMaxResponseBytes int64 = 64 << 20

const maxErrorBodyBytes int64 = 64 << 10

func (e *Error) Error() string { return e.msg }

func newError(format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

// Client is a thin HTTP client for the MantisBT REST API.
type Client struct {
	BaseURL          string
	Token            string
	HTTP             *http.Client
	MaxResponseBytes int64
}

// NewClient builds a client with a sensible default timeout.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:          baseURL,
		Token:            token,
		HTTP:             &http.Client{Timeout: 30 * time.Second, CheckRedirect: checkRedirect},
		MaxResponseBytes: DefaultMaxResponseBytes,
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
	endpoint, err := c.buildURL(path, params)
	if err != nil {
		return nil, newError("%v", err)
	}

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

	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	isError := resp.StatusCode < 200 || resp.StatusCode >= 300
	if isError && limit > maxErrorBodyBytes {
		limit = maxErrorBodyBytes
	}
	if !isError && resp.ContentLength > limit {
		return nil, newError("response exceeds --max-response-size limit of %d bytes", limit)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, newError("%v", err)
	}

	if isError {
		if int64(len(respBody)) > limit {
			respBody = append(respBody[:limit], []byte("\n[response body truncated]")...)
		}
		return nil, newError("%d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), string(respBody))
	}
	if int64(len(respBody)) > limit {
		return nil, newError("response exceeds --max-response-size limit of %d bytes", limit)
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

func (c *Client) buildURL(path string, params map[string]string) (string, error) {
	u, err := config.ParseURL(c.BaseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(u.Path, "/")
	route := strings.TrimLeft(path, "/")
	if strings.HasSuffix(basePath, "/api/rest") || strings.HasSuffix(basePath, "/api/rest/index.php") {
		route = strings.TrimPrefix(route, "api/rest/")
	}
	u = u.JoinPath(route)
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
