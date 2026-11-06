package middleware

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// CaptchaCheck validates the one-shot self-hosted image challenge. It is a
// no-op when the feature is disabled, and when Turnstile is enabled Turnstile
// remains the single human check so the two never demand two widgets.
func CaptchaCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.CaptchaEnabled || common.TurnstileCheckEnabled {
			c.Next()
			return
		}
		// A known device for this phone already passed the challenge once; the
		// marker is only ever set on the SMS send route.
		if c.GetBool(service.TrustedDeviceContextKey) {
			c.Next()
			return
		}
		token := strings.TrimSpace(c.GetHeader("X-Captcha"))
		if token == "" {
			token = strings.TrimSpace(c.Query("captcha"))
		}
		if !service.VerifyCaptcha(c.Request.Context(), token) {
			common.ApiErrorT(c, "Human verification failed, please refresh and try again")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CaptchaRateLimit bounds image generation. Each challenge allocates a small
// PNG and a Redis key, so an unauthenticated caller must not be able to mint
// them without limit.
func CaptchaRateLimit() gin.HandlerFunc {
	return rateLimitFactory(60, 60, "CP")
}
