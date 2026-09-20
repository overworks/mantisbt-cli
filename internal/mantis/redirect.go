package mantis

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Redirects may canonicalize an endpoint, but must not change the operation or
// send credentials to a different origin. Configure the final base URL when
// moving between origins (including an HTTP/HTTPS upgrade).
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	if req.URL.User != nil || !sameOrigin(req.URL, via[0].URL) {
		return fmt.Errorf("redirect to a different origin is not allowed; configure the final MantisBT URL")
	}
	if req.Method != via[0].Method {
		return fmt.Errorf("redirect would change request method from %s to %s", via[0].Method, req.Method)
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
