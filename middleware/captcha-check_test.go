package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestCaptchaCheckPassesThroughWhenDisabled(t *testing.T) {
	previous := common.CaptchaEnabled
	common.CaptchaEnabled = false
	defer func() { common.CaptchaEnabled = previous }()

	called := false
	router := gin.New()
	router.GET("/probe", CaptchaCheck(), func(c *gin.Context) {
		called = true
		c.Status(204)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/probe", nil))
	if !called || recorder.Code != 204 {
		t.Fatalf("disabled CAPTCHA blocked the request: called=%v status=%d", called, recorder.Code)
	}
}

func TestCaptchaCheckRejectsMissingToken(t *testing.T) {
	previous := common.CaptchaEnabled
	previousTurnstile := common.TurnstileCheckEnabled
	common.CaptchaEnabled = true
	common.TurnstileCheckEnabled = false
	defer func() {
		common.CaptchaEnabled = previous
		common.TurnstileCheckEnabled = previousTurnstile
	}()

	called := false
	router := gin.New()
	router.GET("/probe", CaptchaCheck(), func(c *gin.Context) {
		called = true
		c.Status(204)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/probe", nil))
	if called || recorder.Code != 200 {
		t.Fatalf("missing CAPTCHA was not rejected: called=%v status=%d", called, recorder.Code)
	}
}

func TestCaptchaCheckIsSkippedWhenTurnstileIsEnabled(t *testing.T) {
	previous := common.CaptchaEnabled
	previousTurnstile := common.TurnstileCheckEnabled
	common.CaptchaEnabled = true
	common.TurnstileCheckEnabled = true
	defer func() {
		common.CaptchaEnabled = previous
		common.TurnstileCheckEnabled = previousTurnstile
	}()

	called := false
	router := gin.New()
	router.GET("/probe", CaptchaCheck(), func(c *gin.Context) {
		called = true
		c.Status(204)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/probe", nil))
	if !called || recorder.Code != 204 {
		t.Fatalf("captcha ran while Turnstile was enabled: called=%v status=%d", called, recorder.Code)
	}
}
