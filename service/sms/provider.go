// Package sms sends verification codes through a configurable provider.
//
// SendCloud is the only production provider for now. Adding another one means
// one more file plus a registry entry: callers only see the Provider interface
// and the Config struct, both of which are built from the "sms" settings group.
package sms

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/setting/system_setting"
)

var (
	// ErrNotConfigured means the provider is not usable as configured: missing
	// credentials, rejected signature, IP not allowlisted or API permission.
	ErrNotConfigured = errors.New("SMS is not configured or the account rejected the request")
	// ErrInvalidPhone means the provider rejected the number format.
	ErrInvalidPhone = errors.New("Invalid phone number")
	// ErrThrottled means the provider's own per-number rate limit kicked in.
	ErrThrottled = errors.New("SMS sent too frequently to this number, try again later")
	// ErrQuota means the account balance or daily quota is exhausted.
	ErrQuota = errors.New("SMS quota exhausted")
	// ErrProvider is any other provider-side failure.
	ErrProvider = errors.New("SMS provider error")

	errClockSkew = errors.New("SMS provider rejected the request timestamp")
)

// Message is one verification code to deliver. The text itself is composed by
// the provider (sendCode) or by an approved template (send).
type Message struct {
	Phone string
	Code  string
}

// Result is what the provider returned for an accepted request. Delivery is
// asynchronous: an accepted request does not prove the handset received it.
type Result struct {
	SMSIDs     []string
	StatusCode int
}

// Config is the provider-independent view of the settings group.
type Config struct {
	Provider         string
	API              string
	APIBase          string
	SMSUser          string
	SMSKey           string
	SignID           int
	SignName         string
	TemplateID       int
	TemplateParamKey string
	MsgType          int
}

// ConfigFromSettings maps the settings group onto a provider config.
func ConfigFromSettings(settings system_setting.SMSSettings) Config {
	settings = settings.WithDefaults()
	return Config{
		Provider:         settings.Provider,
		API:              settings.API,
		APIBase:          settings.APIBase,
		SMSUser:          settings.SMSUser,
		SMSKey:           settings.SMSKey,
		SignID:           settings.SignID,
		SignName:         settings.SignName,
		TemplateID:       settings.TemplateID,
		TemplateParamKey: settings.TemplateParamKey,
		MsgType:          settings.MsgType,
	}
}

// Provider sends one verification code.
type Provider interface {
	Name() string
	SendCode(ctx context.Context, msg Message) (Result, error)
}

// New builds the provider named by the settings.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case system_setting.SMSProviderSendCloud:
		return newSendCloud(cfg, nil), nil
	case system_setting.SMSProviderConsole:
		return &consoleProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown SMS provider %q", cfg.Provider)
	}
}
