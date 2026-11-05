package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Send endpoints are GET requests that cost money per call. A third-party page
// can hit them with an <img> tag, so browser cross-site requests must be
// rejected before the handler runs. Native clients that send no browser headers
// stay usable.
func TestSendEndpointOriginGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/send", SendEndpointOriginGuard(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{name: "native client without browser headers", want: http.StatusNoContent},
		{name: "same-origin fetch", headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors"}, want: http.StatusNoContent},
		{name: "site none without mode", headers: map[string]string{"Sec-Fetch-Site": "none"}, want: http.StatusNoContent},
		{name: "navigated link", headers: map[string]string{"Sec-Fetch-Mode": "navigate"}, want: http.StatusForbidden},
		{name: "no-cors image", headers: map[string]string{"Sec-Fetch-Mode": "no-cors"}, want: http.StatusForbidden},
		{name: "plain fetch mode", headers: map[string]string{"Sec-Fetch-Mode": "cors"}, want: http.StatusNoContent},
		{name: "same-site subdomain", headers: map[string]string{"Sec-Fetch-Site": "same-site"}, want: http.StatusForbidden},
		{name: "cross-site fetch", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, want: http.StatusForbidden},
		{name: "matching origin", headers: map[string]string{"Origin": "https://token.shanedit.com"}, want: http.StatusNoContent},
		{name: "foreign origin", headers: map[string]string{"Origin": "https://evil.example"}, want: http.StatusForbidden},
		{name: "null origin", headers: map[string]string{"Origin": "null"}, want: http.StatusForbidden},
		{name: "matching referer", headers: map[string]string{"Referer": "https://token.shanedit.com/console"}, want: http.StatusNoContent},
		{name: "foreign referer", headers: map[string]string{"Referer": "https://evil.example/console"}, want: http.StatusForbidden},
		{name: "cross-site wins over matching origin", headers: map[string]string{
			"Sec-Fetch-Site": "cross-site",
			"Origin":         "https://token.shanedit.com",
		}, want: http.StatusForbidden},
		{name: "comma separated origin", headers: map[string]string{"Origin": "https://token.shanedit.com, https://evil.example"}, want: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://token.shanedit.com/send", nil)
			for key, value := range tc.headers {
				request.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code)
		})
	}
}

func TestSendEndpointOriginGuardAllowsLocalDevPorts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/send", SendEndpointOriginGuard(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "http://localhost:3000/send", nil)
	request.Header.Set("Origin", "http://localhost:3001")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	request = httptest.NewRequest(http.MethodGet, "https://token.shanedit.com/send", nil)
	request.Header.Set("Origin", "https://token.shanedit.com:8443")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}
