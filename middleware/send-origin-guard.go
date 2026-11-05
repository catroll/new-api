package middleware

import (
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
	return strings.EqualFold(parsed.Host, r.Host)
}
