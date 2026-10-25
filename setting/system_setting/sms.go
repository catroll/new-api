package system_setting

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	SMSProviderSendCloud = "sendcloud"
	SMSProviderConsole   = "console"

	SMSAPISendCode = "sendCode"
	SMSAPISend     = "send"

	SMSMsgTypeDomestic      = 0
	SMSMsgTypeInternational = 2

	DefaultSMSAPIBase = "https://api.sendcloud.net/smsapi"
)

// SMSSettings is the "sms" configuration group. Every field becomes an option
// key "sms.<json tag>" in the options table, so the whole SMS integration can be
// configured from the admin console without a redeploy.
//
// Credentials and the signature id deliberately default to empty: they are
// installation-specific and must be filled in the console, never baked into the
// binary.
type SMSSettings struct {
	Enabled      bool `json:"enabled"`
	LoginEnabled bool `json:"login_enabled"`
	// RequirePhone makes the phone mandatory at registration and shows a
	// dismissible "bind your phone" reminder after login. It is mutually
	// exclusive with EmailVerificationEnabled.
	RequirePhone bool `json:"require_phone"`

	Provider string `json:"provider"`
	API      string `json:"api"`
	APIBase  string `json:"api_base"`

	SMSUser  string `json:"sms_user"`
	SMSKey   string `json:"sms_key"`
	SignID   int    `json:"sign_id"`
	SignName string `json:"sign_name"`

	TemplateID       int    `json:"template_id"`
	TemplateParamKey string `json:"template_param_key"`
	MsgType          int    `json:"msg_type"`

	CodeTTLSeconds  int `json:"code_ttl_seconds"`
	ResendSeconds   int `json:"resend_seconds"`
	MaxAttempts     int `json:"max_attempts"`
	PhoneDailyLimit int `json:"phone_daily_limit"`
	IPHourlyLimit   int `json:"ip_hourly_limit"`
	DailyQuota      int `json:"daily_quota"`

	AllowedCountryCodes string `json:"allowed_country_codes"`
}

var defaultSMSSettings = SMSSettings{}

func init() {
	config.GlobalConfig.Register("sms", &defaultSMSSettings)
}

func GetSMSSettings() *SMSSettings {
	return &defaultSMSSettings
}

// SMSSettingsSnapshot returns a copy with defaults applied, safe to use while
// an administrator edits the options.
func SMSSettingsSnapshot() SMSSettings {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return defaultSMSSettings.WithDefaults()
}

// WithDefaults fills the fields that have a meaningful built-in default. A zero
// value must not silently disable a limit, so every numeric field has one.
func (s SMSSettings) WithDefaults() SMSSettings {
	if strings.TrimSpace(s.Provider) == "" {
		s.Provider = SMSProviderSendCloud
	}
	if strings.TrimSpace(s.API) == "" {
		s.API = SMSAPISendCode
	}
	if strings.TrimSpace(s.APIBase) == "" {
		s.APIBase = DefaultSMSAPIBase
	}
	if s.CodeTTLSeconds <= 0 {
		s.CodeTTLSeconds = 300
	}
	if s.ResendSeconds <= 0 {
		s.ResendSeconds = 60
	}
	if s.MaxAttempts <= 0 {
		s.MaxAttempts = 5
	}
	if s.PhoneDailyLimit <= 0 {
		s.PhoneDailyLimit = 5
	}
	if s.IPHourlyLimit <= 0 {
		s.IPHourlyLimit = 10
	}
	if s.DailyQuota < 0 {
		s.DailyQuota = 0
	}
	if strings.TrimSpace(s.TemplateParamKey) == "" {
		s.TemplateParamKey = "code"
	}
	if strings.TrimSpace(s.AllowedCountryCodes) == "" {
		s.AllowedCountryCodes = "+86"
	}
	s.APIBase = strings.TrimRight(s.APIBase, "/")
	s.SMSUser = strings.TrimSpace(s.SMSUser)
	s.SMSKey = strings.TrimSpace(s.SMSKey)
	s.SignName = strings.TrimSpace(s.SignName)
	return s
}

// ProviderConfigured reports whether the credentials needed to send are present.
func (s SMSSettings) ProviderConfigured() bool {
	if s.Provider == SMSProviderConsole {
		return true
	}
	if s.SMSUser == "" || s.SMSKey == "" {
		return false
	}
	return s.SignID > 0 || s.SignName != ""
}

// Validate rejects a configuration that cannot work. It is called by the option
// update hook before a value is persisted.
func (s SMSSettings) Validate() error {
	s = s.WithDefaults()
	switch s.Provider {
	case SMSProviderSendCloud, SMSProviderConsole:
	default:
		return errors.New("SMS provider must be sendcloud or console")
	}
	switch s.API {
	case SMSAPISendCode, SMSAPISend:
	default:
		return errors.New("SMS API must be sendCode or send")
	}
	if s.API == SMSAPISend && s.TemplateID <= 0 {
		return errors.New("A template id is required when the SMS API is send")
	}
	if s.Enabled && !s.ProviderConfigured() {
		return errors.New("SMS credentials and signature must be configured before enabling SMS")
	}
	return nil
}
