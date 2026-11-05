package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetCaptcha issues a one-shot self-hosted image challenge. It is only served
// when the operator enabled the self-hosted CAPTCHA and Turnstile is off.
func GetCaptcha(c *gin.Context) {
	if !common.CaptchaEnabled || common.TurnstileCheckEnabled {
		common.ApiErrorT(c, "Self-hosted CAPTCHA is not enabled")
		return
	}
	id, image, err := service.GenerateCaptcha()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":    id,
			"image": image,
		},
	})
}
