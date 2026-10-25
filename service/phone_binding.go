package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	smsprovider "github.com/QuantumNous/new-api/service/sms"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

var (
	ErrSMSNotEnabled    = errors.New("SMS is not enabled")
	ErrPhoneNotAllowed  = errors.New("This phone number is not supported")
	mainlandPhoneFormat = regexp.MustCompile(`^\+861[3-9]\d{9}$`)
)

// NormalizePhoneInput validates the raw input against the configured country
// list and returns the canonical form used everywhere else.
func NormalizePhoneInput(raw string) (string, error) {
	return normalizePhoneWithSettings(raw, system_setting.SMSSettingsSnapshot())
}

func normalizePhoneWithSettings(raw string, settings system_setting.SMSSettings) (string, error) {
	settings = settings.WithDefaults()
	phone := model.NormalizePhone(raw)
	if err := model.ValidateNormalizedPhone(phone); err != nil {
		return "", err
	}
	for _, countryCode := range strings.Split(settings.AllowedCountryCodes, ",") {
		countryCode = strings.TrimSpace(countryCode)
		if countryCode == "" {
			continue
		}
		if !strings.HasPrefix(countryCode, "+") {
			countryCode = "+" + countryCode
		}
		if !strings.HasPrefix(phone, countryCode) {
			continue
		}
		if countryCode == "+86" && !mainlandPhoneFormat.MatchString(phone) {
			return "", model.ErrPhoneInvalid
		}
		return phone, nil
	}
	return "", ErrPhoneNotAllowed
}

// SendSMSCode applies the resend cooldown, the per-number and site-wide daily
// budgets, stores the code and hands it to the provider.
//
// Callers must check whether a code is actually needed (an unknown number for
// login, a taken number for registration) before calling this, so limits and
// money are only spent on real sends.
func SendSMSCode(ctx context.Context, purpose, scope, phone string, settings system_setting.SMSSettings) (string, error) {
	settings = settings.WithDefaults()

	allowed, err := TakeSMSCooldown(ctx, purpose, scope, time.Duration(settings.ResendSeconds)*time.Second)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", ErrSMSSendTooSoon
	}
	if err := TakeSMSPhoneDailyBudget(ctx, phone, settings.PhoneDailyLimit); err != nil {
		return "", err
	}
	if err := TakeSMSGlobalDailyBudget(ctx, settings.DailyQuota); err != nil {
		return "", err
	}

	code, err := IssueSMSCode(ctx, purpose, scope, time.Duration(settings.CodeTTLSeconds)*time.Second)
	if err != nil {
		return "", err
	}

	provider, err := smsprovider.New(smsprovider.ConfigFromSettings(settings))
	if err != nil {
		return "", err
	}
	result, err := provider.SendCode(ctx, smsprovider.Message{Phone: phone, Code: code})
	if err != nil {
		logger.LogWarn(ctx, common.LogText("sms send failed purpose=%s phone=%s provider=%s: %v", purpose, common.MaskPhone(phone), provider.Name(), err))
		return "", err
	}
	logger.LogInfo(ctx, common.LogText("sms sent purpose=%s phone=%s provider=%s status=%d sms_ids=%v", purpose, common.MaskPhone(phone), provider.Name(), result.StatusCode, result.SMSIDs))
	return code, nil
}

// SendPhoneBindCode sends a bind code for an authenticated account.
func SendPhoneBindCode(ctx context.Context, userID int, rawPhone string) error {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled {
		return ErrSMSNotEnabled
	}
	phone, err := normalizePhoneWithSettings(rawPhone, settings)
	if err != nil {
		return err
	}
	if err := model.EnsurePhoneAvailable(phone, userID); err != nil {
		return err
	}
	_, err = SendSMSCode(ctx, SMSCodePurposeBind, SMSCodeScopeForBind(userID, phone), phone, settings)
	return err
}

// FinishPhoneBind verifies the bind code and attaches the number in one step.
func FinishPhoneBind(ctx context.Context, userID int, rawPhone, code string) (string, error) {
	settings := system_setting.SMSSettingsSnapshot()
	if !settings.Enabled {
		return "", ErrSMSNotEnabled
	}
	phone, err := normalizePhoneWithSettings(rawPhone, settings)
	if err != nil {
		return "", err
	}
	if err := VerifySMSCode(ctx, SMSCodePurposeBind, SMSCodeScopeForBind(userID, phone), code, settings.MaxAttempts); err != nil {
		return "", err
	}
	if err := model.BindUserPhone(userID, phone); err != nil {
		return "", err
	}
	return phone, nil
}
