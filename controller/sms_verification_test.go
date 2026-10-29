package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

const stepUpTestPhone = "+8613800138000"

// enableStepUpSMS turns on the console provider for the duration of one test.
// The console provider requires debug mode and never touches the network.
func enableStepUpSMS(t *testing.T) {
	t.Helper()
	previousSettings := *system_setting.GetSMSSettings()
	previousDebug := common.DebugEnabled
	t.Cleanup(func() {
		*system_setting.GetSMSSettings() = previousSettings
		common.DebugEnabled = previousDebug
	})
	*system_setting.GetSMSSettings() = (system_setting.SMSSettings{
		Enabled:  true,
		Provider: system_setting.SMSProviderConsole,
	}).WithDefaults()
	common.DebugEnabled = true
}

func disablePasswordLogin(t *testing.T) {
	t.Helper()
	previous := common.PasswordLoginEnabled
	t.Cleanup(func() { common.PasswordLoginEnabled = previous })
	common.PasswordLoginEnabled = false
}

// A bound phone makes SMS a fallback method exactly where the password was the
// only option, and an enrolled factor still wins: a SIM swap must not downgrade
// an account that has 2FA.
func TestStepUpSMSIsOfferedOnlyWithoutAStrongerFactor(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	enableStepUpSMS(t)
	disablePasswordLogin(t)
	require.NoError(t, model.BindUserPhone(user.Id, stepUpTestPhone))

	requirements, err := service.GetVerificationRequirements(identity, service.VerificationScopePasswordChange)
	require.NoError(t, err)
	require.Len(t, requirements.Methods, 2)
	require.Equal(t, service.VerificationMethodPassword, requirements.Methods[0].Method)
	require.False(t, requirements.Methods[0].Available, "a disabled password must stay unavailable")
	require.Equal(t, service.VerificationMethodSMS, requirements.Methods[1].Method)
	require.True(t, requirements.Methods[1].Available)
	require.Equal(t, "138****8000", requirements.PhoneMasked)
	require.Equal(t, 60, requirements.SMSResendSeconds)

	require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
	requirements, err = service.GetVerificationRequirements(identity, service.VerificationScopePasswordChange)
	require.NoError(t, err)
	require.Len(t, requirements.Methods, 1)
	require.Equal(t, service.VerificationMethodTwoFA, requirements.Methods[0].Method)
	require.Empty(t, requirements.PhoneMasked, "an enrolled factor must not advertise the SMS fallback")
}

// An account without a bound number keeps the old behaviour, and asking for a
// code is rejected instead of silently sending one somewhere.
func TestStepUpSMSRequiresABoundPhone(t *testing.T) {
	_, identity := setupSecurityEnrollmentTest(t)
	enableStepUpSMS(t)
	disablePasswordLogin(t)

	requirements, err := service.GetVerificationRequirements(identity, service.VerificationScopePasswordChange)
	require.NoError(t, err)
	require.Len(t, requirements.Methods, 1)
	require.Equal(t, service.VerificationMethodPassword, requirements.Methods[0].Method)

	_, err = service.SendSecurityVerificationSMS(context.Background(), identity, service.VerificationOperation{Scope: service.VerificationScopePasswordChange})
	require.ErrorIs(t, err, service.ErrVerificationUnavailable)
}

// A step-up code is bound to one account, one action and one context: it cannot
// authorize a different operation, and it is consumed on first use.
func TestStepUpSMSCodeAuthorizesOnlyItsOwnOperation(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	enableStepUpSMS(t)
	disablePasswordLogin(t)
	require.NoError(t, model.BindUserPhone(user.Id, stepUpTestPhone))

	ctx := context.Background()
	operation := service.VerificationOperation{
		Scope:   service.VerificationScopeAccountBind,
		Context: json.RawMessage(`{"provider":"email","email":"first@example.com"}`),
	}
	masked, err := service.SendSecurityVerificationSMS(ctx, identity, operation)
	require.NoError(t, err)
	require.Equal(t, "138****8000", masked)

	other := service.VerificationOperation{
		Scope:   service.VerificationScopeAccountBind,
		Context: json.RawMessage(`{"provider":"email","email":"second@example.com"}`),
	}
	binding, err := service.BindVerificationOperation(other)
	require.NoError(t, err)
	code, err := service.IssueSMSCode(ctx, service.SMSCodePurposeVerify, service.SMSCodeScopeForVerification(user.Id, binding), time.Minute)
	require.NoError(t, err)

	// The code issued for "second" cannot pay for "first".
	_, err = service.VerifySecurityInput(ctx, identity, service.VerificationInput{
		Scope:   operation.Scope,
		Context: operation.Context,
		Method:  service.VerificationMethodSMS,
		Code:    code,
	})
	require.ErrorIs(t, err, service.ErrSMSCodeInvalid)

	proof, err := service.VerifySecurityInput(ctx, identity, service.VerificationInput{
		Scope:   other.Scope,
		Context: other.Context,
		Method:  service.VerificationMethodSMS,
		Code:    code,
	})
	require.NoError(t, err)
	require.Equal(t, service.VerificationMethodSMS, proof.Method)
	require.Equal(t, other.Scope, proof.Scope)

	// Single use: the same code cannot mint a second proof.
	_, err = service.VerifySecurityInput(ctx, identity, service.VerificationInput{
		Scope:   other.Scope,
		Context: other.Context,
		Method:  service.VerificationMethodSMS,
		Code:    code,
	})
	require.ErrorIs(t, err, service.ErrSMSCodeInvalid)
}

// The endpoint reports where the code went so the console can say so, and a
// method that is not offered never sends a message.
func TestSendVerificationSMSHandler(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	enableStepUpSMS(t)
	disablePasswordLogin(t)

	body := `{"scope":"account.password.change"}`
	response := securityEnrollmentRequest("POST", "/api/verify/sms", body, "", identity, SendVerificationSMS)
	require.Contains(t, response.Body.String(), "SECURITY_METHOD_UNAVAILABLE", response.Body.String())

	require.NoError(t, model.BindUserPhone(user.Id, stepUpTestPhone))
	response = securityEnrollmentRequest("POST", "/api/verify/sms", body, "", identity, SendVerificationSMS)
	require.Contains(t, response.Body.String(), `"success":true`, response.Body.String())
	require.Contains(t, response.Body.String(), "138****8000", response.Body.String())
	require.Contains(t, response.Body.String(), `"resend_seconds":60`, response.Body.String())
}
