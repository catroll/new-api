package sms

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// consoleProvider logs the code instead of sending it. It only works with
// common.DebugEnabled, so it cannot be selected in production by accident.
type consoleProvider struct{}

func (p *consoleProvider) Name() string { return system_setting.SMSProviderConsole }

func (p *consoleProvider) SendCode(ctx context.Context, msg Message) (Result, error) {
	if !common.DebugEnabled {
		return Result{}, ErrNotConfigured
	}
	if msg.Phone == "" || msg.Code == "" {
		return Result{}, ErrInvalidPhone
	}
	logger.LogInfo(ctx, common.LogText("sms console provider: phone=%s code=%s", common.MaskPhone(msg.Phone), msg.Code))
	return Result{
		SMSIDs:     []string{fmt.Sprintf("console-%d", time.Now().UnixNano())},
		StatusCode: 200,
	}, nil
}
