package system_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
)

// TestSMSSettingsDefaults pins the documented built-in defaults: an admin who
// never opens the SMS settings page still gets a bounded, working policy.
func TestSMSSettingsDefaults(t *testing.T) {
	settings := SMSSettings{}.WithDefaults()
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"code ttl", settings.CodeTTLSeconds, 300},
		{"resend cooldown", settings.ResendSeconds, 60},
		{"max attempts", settings.MaxAttempts, 5},
		{"phone daily limit", settings.PhoneDailyLimit, 10},
		{"ip hourly limit", settings.IPHourlyLimit, 10},
		{"daily quota", settings.DailyQuota, 3000},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	if settings.API != SMSAPISendCode {
		t.Errorf("api = %q, want %q", settings.API, SMSAPISendCode)
	}
	if settings.Provider != SMSProviderSendCloud {
		t.Errorf("provider = %q, want %q", settings.Provider, SMSProviderSendCloud)
	}
	if settings.MsgType != SMSMsgTypeDomestic {
		t.Errorf("msg type = %d, want domestic", settings.MsgType)
	}
	if settings.TemplateParamKey != "code" {
		t.Errorf("template param key = %q, want code", settings.TemplateParamKey)
	}
	if settings.AllowedCountryCodes != "+86" {
		t.Errorf("allowed country codes = %q, want +86", settings.AllowedCountryCodes)
	}
}

// TestSMSOptionDefaultsAreExported guards the registration path: the admin
// console reads defaults from the options API, which InitOptionMap builds from
// ExportAllConfigs. If the registered struct stays zero-valued, every numeric
// field shows 0 on a fresh installation even though WithDefaults is correct.
func TestSMSOptionDefaultsAreExported(t *testing.T) {
	exported := config.GlobalConfig.ExportAllConfigs()
	wants := map[string]string{
		"sms.api":               SMSAPISendCode,
		"sms.provider":          SMSProviderSendCloud,
		"sms.code_ttl_seconds":  "300",
		"sms.resend_seconds":    "60",
		"sms.max_attempts":      "5",
		"sms.phone_daily_limit": "10",
		"sms.ip_hourly_limit":   "10",
		"sms.daily_quota":       "3000",
		"sms.msg_type":          "0",
	}
	for key, want := range wants {
		if got := exported[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
