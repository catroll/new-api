package sms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const (
	sendCloudTimeout   = 5 * time.Second
	sendCloudMaxBody   = 1 << 20
	sendCloudTimePath  = "/timestamp/get"
	secondsBeforeMilli = 1_000_000_000_000 // a millisecond timestamp is larger than this
)

// sendCloudProvider talks to the SendCloud SMS API. The official Go SDK is not
// used on purpose: its HTTP client is http.DefaultClient with no timeout and
// its base URL is a private constant, which is unacceptable on a login path.
type sendCloudProvider struct {
	cfg    Config
	client *http.Client
}

func newSendCloud(cfg Config, client *http.Client) *sendCloudProvider {
	if client == nil {
		client = &http.Client{Timeout: sendCloudTimeout}
	}
	return &sendCloudProvider{cfg: cfg, client: client}
}

func (p *sendCloudProvider) Name() string { return system_setting.SMSProviderSendCloud }

// SendCode sends one verification code and maps SendCloud status codes onto the
// package sentinels. A nil error only means "accepted": delivery is reported
// asynchronously through SMSHook, which this integration does not consume.
func (p *sendCloudProvider) SendCode(ctx context.Context, msg Message) (Result, error) {
	if p.cfg.SMSUser == "" || p.cfg.SMSKey == "" || (p.cfg.SignID <= 0 && p.cfg.SignName == "") {
		return Result{}, ErrNotConfigured
	}
	if strings.TrimSpace(msg.Phone) == "" || strings.TrimSpace(msg.Code) == "" {
		return Result{}, ErrInvalidPhone
	}

	result, err := p.send(ctx, msg, time.Now().UnixMilli())
	if errors.Is(err, errClockSkew) {
		// The docs disagree on the tolerance (60s vs 6000ms), so ask the server
		// instead of trusting the local clock, then retry exactly once.
		if serverMillis, clockErr := p.serverTime(ctx); clockErr == nil {
			logger.LogWarn(ctx, common.LogText("sms timestamp rejected, retrying with server time offset=%dms", serverMillis-time.Now().UnixMilli()))
			result, err = p.send(ctx, msg, serverMillis)
		}
	}
	return result, err
}

func (p *sendCloudProvider) send(ctx context.Context, msg Message, timestampMillis int64) (Result, error) {
	endpoint := strings.TrimRight(p.cfg.APIBase, "/")

	params := map[string]string{
		"smsUser":   p.cfg.SMSUser,
		"msgType":   strconv.Itoa(p.cfg.MsgType),
		"phone":     p.providerPhone(msg.Phone),
		"timestamp": strconv.FormatInt(timestampMillis, 10),
	}
	if p.cfg.SignID > 0 {
		params["signId"] = strconv.Itoa(p.cfg.SignID)
	} else {
		params["signName"] = p.cfg.SignName
	}

	if p.cfg.API == system_setting.SMSAPISend {
		if p.cfg.TemplateID <= 0 {
			return Result{}, ErrNotConfigured
		}
		vars, err := json.Marshal(map[string]string{p.cfg.TemplateParamKey: msg.Code})
		if err != nil {
			return Result{}, ErrProvider
		}
		params["templateId"] = strconv.Itoa(p.cfg.TemplateID)
		params["vars"] = string(vars)
		endpoint += "/send"
	} else {
		params["code"] = msg.Code
		endpoint += "/sendCode"
	}

	params["signature"] = signParams(params, p.cfg.SMSKey)

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		logger.LogWarn(ctx, common.LogText("sms provider request failed: %v", err))
		return Result{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, sendCloudMaxBody))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	if resp.StatusCode != http.StatusOK {
		logger.LogWarn(ctx, common.LogText("sms provider http %d phone=%s", resp.StatusCode, common.MaskPhone(msg.Phone)))
		return Result{}, fmt.Errorf("%w: http %d", ErrProvider, resp.StatusCode)
	}

	var envelope sendCloudResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("%w: invalid response", ErrProvider)
	}

	if err := classify(envelope); err != nil {
		logger.LogWarn(ctx, common.LogText(
			"sms provider rejected phone=%s status=%d message=%q", common.MaskPhone(msg.Phone), envelope.StatusCode, envelope.Message))
		return Result{StatusCode: envelope.StatusCode}, err
	}

	return Result{SMSIDs: envelope.Info.SMSIDs, StatusCode: envelope.StatusCode}, nil
}

// signParams implements the documented signature:
// sha256(smsKey + "&" + "k=v&k=v" (keys A-Z, no urlencode) + "&" + smsKey).
func signParams(params map[string]string, smsKey string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "smsKey" || key == "signature" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	sum := sha256.Sum256([]byte(smsKey + "&" + strings.Join(parts, "&") + "&" + smsKey))
	return hex.EncodeToString(sum[:])
}

// providerPhone converts the canonical "+8613800138000" into what SendCloud
// expects: bare mainland digits for msgType=0, country code without "+" for
// international messages.
func (p *sendCloudProvider) providerPhone(phone string) string {
	digits := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if p.cfg.MsgType == system_setting.SMSMsgTypeDomestic && strings.HasPrefix(digits, "86") && len(digits) == 13 {
		return digits[2:]
	}
	return digits
}

func (p *sendCloudProvider) serverTime(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.cfg.APIBase, "/")+sendCloudTimePath, nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, sendCloudMaxBody))
	if err != nil {
		return 0, err
	}
	var envelope sendCloudResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, err
	}
	value, err := envelope.timestampValue()
	if err != nil {
		return 0, err
	}
	if value < secondsBeforeMilli {
		value *= 1000
	}
	return value, nil
}

type sendCloudResponse struct {
	Result     bool        `json:"result"`
	StatusCode int         `json:"statusCode"`
	Message    string      `json:"message"`
	Timestamp  json.Number `json:"timestamp"`
	Info       struct {
		SuccessCount int      `json:"successCount"`
		SMSIDs       []string `json:"smsIds"`
		Items        []struct {
			Phone   string `json:"phone"`
			Message string `json:"message"`
		} `json:"items"`
		Timestamp json.Number `json:"timestamp"`
	} `json:"info"`
}

func (r sendCloudResponse) timestampValue() (int64, error) {
	for _, raw := range []json.Number{r.Info.Timestamp, r.Timestamp} {
		if raw == "" {
			continue
		}
		value, err := raw.Int64()
		if err != nil {
			return 0, err
		}
		return value, nil
	}
	return 0, errors.New("timestamp missing")
}

func classify(envelope sendCloudResponse) error {
	if envelope.Result && (envelope.StatusCode == 200 || (envelope.StatusCode == 311 && envelope.Info.SuccessCount > 0)) {
		return nil
	}
	switch envelope.StatusCode {
	case 412, 413:
		return ErrInvalidPhone
	case 421, 422, 471, 472, 4721, 4722, 601, 602:
		return ErrNotConfigured
	case 461:
		return errClockSkew
	case 496, 497, 498:
		return ErrThrottled
	case 499:
		return ErrQuota
	default:
		return ErrProvider
	}
}
