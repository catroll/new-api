package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Enabling the phone policy must not depend on password registration: sites
// that only offer OAuth/WeChat signup still want the post-login reminder, and
// blocking the switch there was a leftover from the abandoned "block OAuth
// signup" design.
func TestValidateSMSOptionAllowsRequirePhoneWithoutPasswordRegistration(t *testing.T) {
	settings := system_setting.GetSMSSettings()
	previousSettings := *settings
	previousEmailVerification := common.EmailVerificationEnabled
	previousPasswordRegister := common.PasswordRegisterEnabled
	t.Cleanup(func() {
		*settings = previousSettings
		common.EmailVerificationEnabled = previousEmailVerification
		common.PasswordRegisterEnabled = previousPasswordRegister
	})

	*settings = system_setting.SMSSettings{
		Enabled:  true,
		Provider: system_setting.SMSProviderSendCloud,
		SMSUser:  "user",
		SMSKey:   "key",
		SignName: "sign",
	}
	common.EmailVerificationEnabled = false
	common.PasswordRegisterEnabled = false

	if err := validateSMSOption("sms.require_phone", "true"); err != nil {
		t.Fatalf("requiring a phone without password registration = %v, want nil", err)
	}

	common.EmailVerificationEnabled = true
	if err := validateSMSOption("sms.require_phone", "true"); err == nil {
		t.Fatal("requiring a phone while email verification is on must be refused")
	}
}
