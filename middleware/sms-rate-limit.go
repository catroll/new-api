package middleware

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	smsIPRateLimitMark          = "SMSIP"
	smsIPRateLimitWindowSeconds = int64(3600)
)

// SMSIPRateLimit caps how many SMS requests one client IP may make per hour.
// Per-number and site-wide limits live with the code flow, because they only
// count sends that actually reach the provider.
//
// A Redis failure fails closed: a rate limit that silently disappears during an
// outage turns a paid endpoint into an open one.
func SMSIPRateLimit() gin.HandlerFunc {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		settings := system_setting.SMSSettingsSnapshot()
		if !settings.Enabled {
			// The handler answers "SMS is not enabled"; no limit needed.
			c.Next()
			return
		}
		limit := settings.IPHourlyLimit
		if limit <= 0 {
			c.Next()
			return
		}

		ip := c.ClientIP()
		if common.RedisEnabled && common.RDB != nil {
			allowed, _, ttlSeconds, err := redisFixedWindowTake(
				c.Request.Context(),
				redisIPRateLimitKey(smsIPRateLimitMark, ip),
				limit,
				smsIPRateLimitWindowSeconds,
			)
			if err != nil {
				logger.LogError(c.Request.Context(), common.LogText("sms rate limit check failed: %v", err))
				writeSMSTemporarilyUnavailable(c)
				return
			}
			if !allowed {
				writeSMSRateLimited(c, ttlSeconds)
				return
			}
			c.Next()
			return
		}

		if !inMemoryRateLimiter.Request(smsIPRateLimitMark+":"+ip, limit, smsIPRateLimitWindowSeconds) {
			writeSMSRateLimited(c, smsIPRateLimitWindowSeconds)
			return
		}
		c.Next()
	}
}

func writeSMSRateLimited(c *gin.Context, retryAfterSeconds int64) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = smsIPRateLimitWindowSeconds
	}
	c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage(
		"Please wait {{seconds}} seconds before requesting another verification code.",
		map[string]any{"seconds": retryAfterSeconds}))
	c.Abort()
}

func writeSMSTemporarilyUnavailable(c *gin.Context) {
	common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("SMS verification is temporarily unavailable"))
	c.Abort()
}
