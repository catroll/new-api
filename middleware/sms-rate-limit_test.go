package middleware

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func withSMSSettings(t *testing.T, mutate func(settings *system_setting.SMSSettings)) {
	t.Helper()
	settings := system_setting.GetSMSSettings()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	mutate(settings)
}

func TestSMSIPRateLimitPassesThroughWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSMSSettings(t, func(settings *system_setting.SMSSettings) {
		settings.Enabled = false
		settings.IPHourlyLimit = 1
	})

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/sms", SMSIPRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for attempt := 0; attempt < 3; attempt++ {
		response := performRateLimitRequest(router, "/sms", "203.0.113.10:1234")
		if response.Code != http.StatusNoContent {
			t.Fatalf("attempt %d: status = %d, want %d (a disabled SMS policy must not rate limit)", attempt, response.Code, http.StatusNoContent)
		}
	}
}

func TestSMSIPRateLimitEnforcesConfiguredLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	withSMSSettings(t, func(settings *system_setting.SMSSettings) {
		settings.Enabled = true
		settings.IPHourlyLimit = 2
	})

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/sms", SMSIPRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for attempt := 0; attempt < 2; attempt++ {
		response := performRateLimitRequest(router, "/sms", "203.0.113.20:1234")
		if response.Code != http.StatusNoContent {
			t.Fatalf("allowed attempt %d: status = %d, want %d", attempt, response.Code, http.StatusNoContent)
		}
	}

	response := performRateLimitRequest(router, "/sms", "203.0.113.20:1234")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is missing")
	}

	// A different client IP gets its own window.
	response = performRateLimitRequest(router, "/sms", "203.0.113.21:1234")
	if response.Code != http.StatusNoContent {
		t.Fatalf("other ip status = %d, want %d", response.Code, http.StatusNoContent)
	}
}
