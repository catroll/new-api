package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	smsprovider "github.com/QuantumNous/new-api/service/sms"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

type smsPhoneRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

// SendSMSVerification issues a code for login, registration or password reset.
//
//   - login / reset: the number must belong to an account, but the response is
//     uniform either way, so the endpoint cannot be used to enumerate accounts.
//   - register: the number must still be free, and answering "already in use"
//     leaks nothing that Register does not already say.
func SendSMSVerification(c *gin.Context) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled {
		common.ApiErrorT(c, "SMS is not enabled")
		return
	}

	purpose := strings.TrimSpace(c.Query("purpose"))
	if purpose == "" {
		purpose = service.SMSCodePurposeLogin
	}
	switch purpose {
	case service.SMSCodePurposeLogin, service.SMSCodePurposeRegister, service.SMSCodePurposeReset:
	default:
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	phone, err := service.NormalizePhoneInput(c.Query("phone"))
	if err != nil {
		writeSMSError(c, err)
		return
	}

	switch purpose {
	case service.SMSCodePurposeLogin, service.SMSCodePurposeReset:
		if _, err := model.GetUniqueUserByPhone(phone); err != nil {
			if errors.Is(err, model.ErrPhoneNotFound) {
				common.ApiSuccess(c, nil)
				return
			}
			writeSMSError(c, err)
			return
		}
	case service.SMSCodePurposeRegister:
		if err := model.EnsurePhoneAvailable(phone, 0); err != nil {
			writeSMSError(c, err)
			return
		}
	}

	if _, err := service.SendSMSCode(c.Request.Context(), purpose, phone, phone, settings); err != nil {
		writeSMSError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// LoginWithSMS verifies a code and then walks the normal login policy, so 2FA
// and passkey users still get their challenge.
func LoginWithSMS(c *gin.Context) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled || !settings.LoginEnabled {
		common.ApiErrorT(c, "SMS login is not enabled")
		return
	}

	var request smsPhoneRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	phone, err := service.NormalizePhoneInput(request.Phone)
	if err != nil {
		writeSMSError(c, err)
		return
	}
	user, err := model.GetUniqueUserByPhone(phone)
	if err != nil {
		// Same text for "no such number" and "wrong code".
		common.ApiErrorT(c, "Verification code is incorrect or has expired")
		return
	}
	if err := service.VerifySMSCode(c.Request.Context(), service.SMSCodePurposeLogin, phone, request.Code, settings.MaxAttempts); err != nil {
		writeSMSError(c, err)
		return
	}
	setupLogin(user, nil, c)
}

// SendPhoneBindCode sends a code to the number the account wants to bind.
func SendPhoneBindCode(c *gin.Context) {
	var request smsPhoneRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SendPhoneBindCode(c.Request.Context(), c.GetInt("id"), request.Phone); err != nil {
		writeSMSError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// BindPhone attaches the number after the code and the step-up proof for
// account.binding.bind have been verified.
func BindPhone(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}

	var request smsPhoneRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeSecurityOperationError(c, service.ErrVerificationContextInvalid)
		return
	}
	phone, err := service.NormalizePhoneInput(request.Phone)
	if err != nil {
		writeSMSError(c, err)
		return
	}

	proofContext, err := common.Marshal(service.AccountBindingContext{Provider: "phone", Phone: phone})
	if err != nil {
		writeSMSError(c, err)
		return
	}
	authorization := middleware.RequireSecurityProof(c, service.VerificationOperation{
		Scope:   service.VerificationScopeAccountBind,
		Context: proofContext,
	})
	if authorization == nil {
		return
	}

	succeeded := false
	defer func() {
		recordUserSecurityAudit(c, identity.UserID, "user.phone_bind", map[string]any{"success": succeeded})
	}()
	if _, err := service.FinishPhoneBind(c.Request.Context(), identity.UserID, phone, request.Code); err != nil {
		writeSMSError(c, err)
		return
	}
	succeeded = true

	user, err := model.GetSelfUserById(identity.UserID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildSelfUserData(user))
}

// SendTestSMS lets an administrator verify the provider configuration from the
// console. It sends a real message but stores nothing, so it cannot be used to
// log in or to reset a password.
func SendTestSMS(c *gin.Context) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.ProviderConfigured() {
		common.ApiErrorT(c, "SMS credentials and signature must be configured first")
		return
	}

	var request smsPhoneRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	phone, err := service.NormalizePhoneInput(request.Phone)
	if err != nil {
		writeSMSError(c, err)
		return
	}

	provider, err := smsprovider.New(smsprovider.ConfigFromSettings(settings))
	if err != nil {
		writeSMSError(c, err)
		return
	}
	code, err := service.GenerateSMSCode()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := provider.SendCode(c.Request.Context(), smsprovider.Message{Phone: phone, Code: code})
	if err != nil {
		writeSMSError(c, err)
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText(
		"sms test send phone=%s provider=%s status=%d sms_ids=%v", common.MaskPhone(phone), provider.Name(), result.StatusCode, result.SMSIDs))
	common.ApiSuccess(c, gin.H{"sms_ids": result.SMSIDs, "status_code": result.StatusCode})
}

// SendPasswordResetSMS mails a reset code by SMS. Like the email flow it always
// reports success, even for an unknown number.
func SendPasswordResetSMS(c *gin.Context) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled {
		common.ApiErrorT(c, "SMS is not enabled")
		return
	}
	phone, err := service.NormalizePhoneInput(c.Query("phone"))
	if err != nil {
		writeSMSError(c, err)
		return
	}
	if _, err := model.GetUniqueUserByPhone(phone); err != nil {
		if errors.Is(err, model.ErrPhoneNotFound) {
			common.ApiSuccess(c, nil)
			return
		}
		writeSMSError(c, err)
		return
	}
	if _, err := service.SendSMSCode(c.Request.Context(), service.SMSCodePurposeReset, phone, phone, settings); err != nil {
		writeSMSError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ResetPasswordBySMS mirrors ResetPassword: it returns a freshly generated
// password, because that is how the existing reset page presents it.
func ResetPasswordBySMS(c *gin.Context) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled {
		common.ApiErrorT(c, "SMS is not enabled")
		return
	}

	var request smsPhoneRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	phone, err := service.NormalizePhoneInput(request.Phone)
	if err != nil {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}
	if err := service.VerifySMSCode(c.Request.Context(), service.SMSCodePurposeReset, phone, request.Code, settings.MaxAttempts); err != nil {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}

	password := common.GenerateVerificationCode(12)
	if err := model.ResetUserPasswordByPhone(phone, password); err != nil {
		if errors.Is(err, model.ErrPhoneNotFound) || errors.Is(err, model.ErrPhoneAmbiguous) {
			common.ApiErrorT(c, "Password reset link is invalid or has expired")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, password)
}

// writeSMSError maps the sentinels onto user-facing messages. Internal provider
// details are logged, never returned.
func writeSMSError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSMSNotEnabled):
		common.ApiErrorT(c, "SMS is not enabled")
	case errors.Is(err, service.ErrPhoneNotAllowed):
		common.ApiErrorT(c, "This phone number is not supported")
	case errors.Is(err, model.ErrPhoneInvalid):
		common.ApiErrorT(c, "Please enter a valid phone number")
	case errors.Is(err, model.ErrPhoneAlreadyTaken):
		common.ApiErrorT(c, "This phone number is already in use")
	case errors.Is(err, model.ErrPhoneNotFound), errors.Is(err, model.ErrPhoneAmbiguous):
		common.ApiErrorT(c, "This phone number is not linked to an account")
	case errors.Is(err, service.ErrSMSSendTooSoon):
		common.ApiErrorT(c, "Please wait before requesting another verification code")
	case errors.Is(err, service.ErrSMSPhoneDailyLimit):
		common.ApiErrorT(c, "Daily SMS limit for this number reached")
	case errors.Is(err, service.ErrSMSDailyQuota):
		common.ApiErrorT(c, "Daily SMS quota reached")
	case errors.Is(err, service.ErrSMSCodeInvalid):
		common.ApiErrorT(c, "Verification code is incorrect or has expired")
	case errors.Is(err, service.ErrSMSCodeTooMany):
		common.ApiErrorT(c, "Too many verification attempts, request a new code")
	case errors.Is(err, service.ErrSMSStoreUnavailable):
		common.ApiErrorT(c, "SMS verification is temporarily unavailable")
	case errors.Is(err, smsprovider.ErrInvalidPhone):
		common.ApiErrorT(c, "Please enter a valid phone number")
	case errors.Is(err, smsprovider.ErrThrottled):
		common.ApiErrorT(c, "SMS sent too frequently to this number, try again later")
	case errors.Is(err, smsprovider.ErrQuota):
		common.ApiErrorT(c, "SMS quota exhausted")
	case errors.Is(err, smsprovider.ErrNotConfigured), errors.Is(err, smsprovider.ErrProvider):
		logger.LogError(c.Request.Context(), common.LogText("sms provider failure: %v", err))
		common.ApiErrorT(c, "SMS verification is temporarily unavailable")
	default:
		common.ApiError(c, err)
	}
}
