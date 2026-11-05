package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// SendEndpointOriginGuard rejects browser requests that a third-party page could
// trigger against endpoints which send an email or a paid SMS. Those routes are
// GET, so a cross-site <img> or <form> can reach them without a CORS preflight;
// checking Sec-Fetch-Site and the Origin/Referer host closes that path.
//
// A missing browser header is allowed: command-line clients and native apps do
// not send them, and they are not the cross-site threat this guard addresses.
func SendEndpointOriginGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isSameOriginBrowserRequest(c.Request) {
			common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Request origin is not allowed"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func isSameOriginBrowserRequest(r *http.Request) bool {
	if values := r.Header.Values("Sec-Fetch-Site"); len(values) > 1 {
		return false
	} else if len(values) == 1 {
		switch strings.ToLower(strings.TrimSpace(values[0])) {
		case "same-origin", "none":
		default:
			return false
		}
	}

	// Code sending is a background fetch, never a navigation or an <img>. A
	// link in an email or a no-cors subresource is not a legitimate caller, and
	// rejecting those modes closes the cases where Sec-Fetch-Site is missing.
	if values := r.Header.Values("Sec-Fetch-Mode"); len(values) > 1 {
		return false
	} else if len(values) == 1 {
		switch strings.ToLower(strings.TrimSpace(values[0])) {
		case "cors", "same-origin":
		default:
			return false
		}
	}

	for _, header := range []string{"Origin", "Referer"} {
		values := r.Header.Values(header)
		if len(values) == 0 {
			continue
		}
		if len(values) != 1 || !sameRequestHost(r, values[0]) {
			return false
		}
	}
	return true
}

// sameRequestHost compares only the host. The console is served over HTTPS while
// the Go process usually sees plain HTTP behind the proxy, so comparing full
// origins would reject every legitimate request.
func sameRequestHost(r *http.Request, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, ",") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	// The rsbuild dev server proxies /api to another localhost port with
	// changeOrigin, so the page origin and the backend Host differ by port.
	// Loopback is not a cross-site boundary worth enforcing here.
	return isLoopbackHost(parsed.Hostname()) && isLoopbackHost(requestHostname(r.Host))
}

func requestHostname(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
