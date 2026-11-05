package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func smsSendTestSettings(t *testing.T) *system_setting.SMSSettings {
	t.Helper()
	settings := system_setting.GetSMSSettings()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	*settings = (system_setting.SMSSettings{
		Enabled:  true,
		Provider: system_setting.SMSProviderConsole,
	}).WithDefaults()
	return settings
}

func smsSendTestRequest(t *testing.T, query string) (bool, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/verification/sms?"+query, nil)
	SendSMSVerification(c)

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Success, body.Message
}

// The register purpose must not be an open SMS relay: a code is only sent while
// the phone policy is on and password registration is actually available.
func TestSendSMSVerificationRegisterRequiresThePhonePolicy(t *testing.T) {
	settings := smsSendTestSettings(t)
	previousRegister := common.RegisterEnabled
	previousPasswordRegister := common.PasswordRegisterEnabled
	previousEmailVerification := common.EmailVerificationEnabled
	t.Cleanup(func() {
		common.RegisterEnabled = previousRegister
		common.PasswordRegisterEnabled = previousPasswordRegister
		common.EmailVerificationEnabled = previousEmailVerification
	})
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false

	settings.RequirePhone = false
	success, message := smsSendTestRequest(t, "purpose=register&phone=13800138000")
	require.False(t, success)
	require.Equal(t, "SMS registration verification is not enabled", message)

	settings.RequirePhone = true
	common.PasswordRegisterEnabled = false
	success, message = smsSendTestRequest(t, "purpose=register&phone=13800138000")
	require.False(t, success)
	require.Equal(t, "SMS registration verification is not enabled", message)

	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = true
	success, message = smsSendTestRequest(t, "purpose=register&phone=13800138000")
	require.False(t, success)
	require.Equal(t, "SMS registration verification is not enabled", message)

	common.EmailVerificationEnabled = false
	common.RegisterEnabled = false
	success, message = smsSendTestRequest(t, "purpose=register&phone=13800138000")
	require.False(t, success)
	require.Equal(t, "New user registration has been disabled by administrator", message)
}

// Once the policy gate passes, the request reaches phone validation. This pins
// the order: the gate must not reject a legitimate registration attempt.
func TestSendSMSVerificationRegisterPassesThePolicyGate(t *testing.T) {
	settings := smsSendTestSettings(t)
	previousRegister := common.RegisterEnabled
	previousPasswordRegister := common.PasswordRegisterEnabled
	t.Cleanup(func() {
		common.RegisterEnabled = previousRegister
		common.PasswordRegisterEnabled = previousPasswordRegister
	})
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	settings.RequirePhone = true

	success, message := smsSendTestRequest(t, "purpose=register&phone=123")
	require.False(t, success)
	require.Equal(t, "Please enter a valid phone number", message)
}
