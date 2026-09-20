package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseURL validates a MantisBT base URL without echoing credentials or query
// values in errors. The URL may include the installation or REST root path.
func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(raw) != raw || u == nil ||
		(u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return nil, fmt.Errorf("invalid MantisBT URL: use an absolute http:// or https:// URL with a host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("invalid MantisBT URL: embedded credentials are not allowed; use --token or MANTISBT_TOKEN")
	}
	if u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return nil, fmt.Errorf("invalid MantisBT URL: query strings and fragments are not allowed; use the web or REST API root")
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, fmt.Errorf("invalid MantisBT URL: port must be between 1 and 65535")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("invalid MantisBT URL: port must not be empty")
	}
	return u, nil
}
