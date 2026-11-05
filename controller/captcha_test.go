package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type captchaResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	} `json:"data"`
}

func TestGetCaptchaReturnsChallengeWhenEnabled(t *testing.T) {
	previousCaptcha := common.CaptchaEnabled
	previousTurnstile := common.TurnstileCheckEnabled
	common.CaptchaEnabled = true
	common.TurnstileCheckEnabled = false
	defer func() {
		common.CaptchaEnabled = previousCaptcha
		common.TurnstileCheckEnabled = previousTurnstile
	}()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/captcha", nil)

	GetCaptcha(c)

	var body captchaResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || body.Data.ID == "" || body.Data.Image == "" {
		t.Fatalf("unexpected challenge response: %+v", body)
	}
}

func TestGetCaptchaRejectsWhenDisabled(t *testing.T) {
	previousCaptcha := common.CaptchaEnabled
	previousTurnstile := common.TurnstileCheckEnabled
	common.CaptchaEnabled = false
	common.TurnstileCheckEnabled = false
	defer func() {
		common.CaptchaEnabled = previousCaptcha
		common.TurnstileCheckEnabled = previousTurnstile
	}()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/api/captcha", nil)

	GetCaptcha(c)

	var body captchaResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Success || body.Data.ID != "" {
		t.Fatalf("disabled CAPTCHA returned a challenge: %+v", body)
	}
}
