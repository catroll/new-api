package middleware

import (
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TrustedDeviceCheck lets a previously verified device skip the human
// challenge when asking for a login code for the same phone number. It only
// marks the request; CaptchaCheck and TurnstileCheck read the marker, so a
// storage failure leaves both checks in place.
func TrustedDeviceCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		purpose := strings.TrimSpace(c.Query("purpose"))
		if purpose == "" {
			purpose = service.SMSCodePurposeLogin
		}
		if purpose != service.SMSCodePurposeLogin {
			c.Next()
			return
		}
		phone := strings.TrimSpace(c.Query("phone"))
		if phone == "" {
			c.Next()
			return
		}
		trusted, err := service.ValidateTrustedDevice(c, phone)
		if err != nil {
			// Fail closed: a broken store must not turn into a captcha bypass.
			c.Next()
			return
		}
		if trusted {
			c.Set(service.TrustedDeviceContextKey, true)
		}
		c.Next()
	}
}
