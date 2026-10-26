package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/go-pay/gopay"
	"github.com/go-pay/gopay/alipay"
	"github.com/shopspring/decimal"
)

// Alipay direct adapter.
//
// It speaks the classic open-platform gateway with RSA2 signing:
//
//	POST /api/user/alipay/amount  price preview
//	POST /api/user/alipay/pay     create the order, return the signed checkout URL
//	POST /api/alipay/notify       asynchronous result (the only thing that credits)
//
// The synchronous return URL only sends the browser back to the wallet page; it
// carries no authority.
//
// Note on the gopay package: the classic `alipay` package is used rather than
// `alipay/v3` because 电脑网站支付, 手机网站支付 and 交易查询 all live in
// gateway.do, while alipay/v3's TradeQuery targets the newer OpenAPI v3 REST
// endpoint — a different protocol with a separate account enablement. Keeping
// one API family means a failure is attributable to the credentials.

type AlipayPayRequest struct {
	Amount int64 `json:"amount"`
	// PaymentMethod is validated against model.PaymentMethodAlipayDirect so the
	// front end can keep one request shape across gateways.
	PaymentMethod string `json:"payment_method"`
	// Device is an optional hint: "mobile" forces 手机网站支付. Empty means the
	// user agent decides.
	Device string `json:"device,omitempty"`
}

// getAlipayClient builds a client per request. Two reasons: notify_url and
// return_url are read from the current callback address, and gopay's client is
// cheap (one RSA key decode), while the endpoint is rate limited anyway.
func getAlipayClient() (*alipay.Client, error) {
	if !isAlipayWebhookConfigured() {
		return nil, errors.New("payment information has not been configured by administrator")
	}

	client, err := alipay.NewClient(normalizeAlipayKey(setting.AlipayAppId), normalizeAlipayKey(setting.AlipayPrivateKey), !setting.AlipaySandbox)
	if err != nil {
		return nil, fmt.Errorf("initialize Alipay client: %s", redactKeyMaterial(err.Error()))
	}
	client.SetCharset("utf-8").
		SetSignType(alipay.RSA2).
		SetNotifyUrl(service.GetCallbackAddress() + "/api/alipay/notify").
		SetReturnUrl(paymentReturnPath("/usage-logs"))
	return client, nil
}

// normalizeAlipayKey turns any accepted shape of a key into bare base64.
// gopay's FormatAlipayPrivateKey / FormatAlipayPublicKey always re-wrap their
// input as PEM, so a pasted PEM block breaks: PKCS#1 fails outright and PKCS#8
// only survives because the PEM decoder happens to find the inner block.
func normalizeAlipayKey(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(raw), `\n`, "\n"))
	if !strings.Contains(raw, "-----BEGIN") {
		return strings.Join(strings.Fields(raw), "")
	}
	var b strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

// getAlipayMoney returns the amount to charge, rounded to fen, and its decimal
// string form for the gateway.
func getAlipayMoney(amount int64, group string) (float64, string) {
	payMoney := getPayMoney(amount, group)
	d := decimal.NewFromFloat(payMoney).Round(2)
	return d.InexactFloat64(), d.StringFixed(2)
}

// getAlipayMinTopup is the minimum this gateway accepts, in the same unit the
// wallet sends (quota units). The wallet displays setting.AlipayMinTopUp, so the
// handler must enforce the same number instead of the global MinTopUp.
func getAlipayMinTopup() int64 {
	minTopUp := int64(setting.AlipayMinTopUp)
	if minTopUp <= 0 {
		minTopUp = getMinTopup()
	}
	return minTopUp
}

// requireCNYPricing refuses requests when the console quotes another currency.
// Alipay charges CNY, while getPayMoney multiplies by Price in the display
// currency: a hidden button is not enough, a direct API call must not create a
// mispriced order either.
func requireCNYPricing(c *gin.Context) bool {
	if operation_setting.IsCNYDisplay() {
		return true
	}
	logger.LogWarn(c.Request.Context(), common.LogText("Alipay request rejected reason=display_currency_is_not_cny path=%q user_id=%d",
		c.Request.RequestURI, c.GetInt("id")))
	paymentError(c, common.NewMessage("This gateway requires the console currency to be CNY"))
	return false
}

// isMobileDevice decides between 手机网站支付 and 电脑网站支付.
func isMobileDevice(ua string, hint string) bool {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "mobile":
		return true
	case "desktop", "pc":
		return false
	}
	ua = strings.ToLower(ua)
	for _, needle := range []string{"android", "iphone", "ipad", "ipod", "mobile", "harmonyos", "micromessenger"} {
		if strings.Contains(ua, needle) {
			return true
		}
	}
	return false
}

// yuanToFen converts a gateway amount string to fen without going through
// float64: 14.30 * 100 is 1429.9999… and a one-fen error fails every check.
func yuanToFen(raw string) (int64, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	if d.IsNegative() {
		return 0, fmt.Errorf("negative amount %q", raw)
	}
	return d.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
}

func RequestAlipayAmount(c *gin.Context) {
	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	if !requireCNYPricing(c) {
		return
	}
	if req.Amount < getAlipayMinTopup() {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": getAlipayMinTopup()}))
		return
	}
	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		paymentError(c, common.NewMessage("Failed to get the user group"))
		return
	}
	payMoney, moneyText := getAlipayMoney(req.Amount, group)
	if payMoney < 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": moneyText})
}

func RequestAlipayPay(c *gin.Context) {
	ctx := c.Request.Context()
	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		paymentError(c, common.NewMessage("Invalid parameters"))
		return
	}
	if req.PaymentMethod != "" && req.PaymentMethod != model.PaymentMethodAlipayDirect {
		paymentError(c, common.NewMessage("Unsupported payment channel"))
		return
	}
	if !requireCNYPricing(c) {
		return
	}
	if req.Amount < getAlipayMinTopup() {
		paymentError(c, common.NewMessage("Top-up amount cannot be less than {{min}}", map[string]any{"min": getAlipayMinTopup()}))
		return
	}

	id := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, id, req.Amount) {
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		paymentError(c, common.NewMessage("Failed to get the user group"))
		return
	}
	payMoney, moneyText := getAlipayMoney(req.Amount, group)
	if payMoney < 0.01 {
		paymentError(c, common.NewMessage("Payment amount is too low"))
		return
	}

	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(ctx, common.LogText("Alipay failed to initialize client user_id=%d error=%q", id, err.Error()))
		paymentError(c, common.NewMessage("Payment information has not been configured by administrator"))
		return
	}

	tradeNo := newTopUpTradeNo(id)
	// Same convention as the aggregator adapter: the stored amount is quota units,
	// money is what the gateway charges.
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          req.Amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipayDirect,
		PaymentProvider: model.PaymentProviderAlipay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	// Insert before calling the gateway: a notification can arrive within
	// seconds and must find the order.
	if err = topUp.Insert(); err != nil {
		logger.LogError(ctx, common.LogText("Alipay failed to create top-up order user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		paymentError(c, common.NewMessage("Failed to create order"))
		return
	}

	bm := make(gopay.BodyMap)
	bm.Set("subject", fmt.Sprintf("TUC%d", req.Amount)).
		Set("out_trade_no", tradeNo).
		Set("total_amount", moneyText).
		Set("timeout_express", "15m")

	var payURL string
	if isMobileDevice(c.Request.UserAgent(), req.Device) {
		payURL, err = client.TradeWapPay(ctx, bm)
	} else {
		payURL, err = client.TradePagePay(ctx, bm)
	}
	if err != nil {
		// The order stays in the table as failed: a late notification is still
		// allowed to flip it to paid (see RechargeDirect), because the gateway
		// may have accepted a request whose HTTP response we lost.
		if updateErr := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderAlipay, common.TopUpStatusFailed); updateErr != nil {
			logger.LogError(ctx, common.LogText("Alipay failed to mark order failed trade_no=%s error=%q", tradeNo, updateErr.Error()))
		}
		safe := redactKeyMaterial(err.Error())
		logger.LogError(ctx, common.LogText("Alipay failed to start payment user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, safe))
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}
	if payURL == "" || payURL == gopay.NULL {
		logger.LogError(ctx, common.LogText("Alipay returned an empty pay url user_id=%d trade_no=%s", id, tradeNo))
		paymentError(c, common.NewMessage("Failed to start payment"))
		return
	}

	logger.LogInfo(ctx, common.LogText("Alipay top-up order created user_id=%d trade_no=%s amount=%d money=%s mobile=%v",
		id, tradeNo, req.Amount, moneyText, isMobileDevice(c.Request.UserAgent(), req.Device)))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_url":  payURL,
			"trade_no": tradeNo,
		},
	})
}

// AlipayNotify handles the asynchronous notification. The signature is the only
// proof the callback came from Alipay; the amount is checked against the local
// order inside the credit path.
func AlipayNotify(c *gin.Context) {
	ctx := c.Request.Context()
	if !isAlipayWebhookEnabled() {
		logger.LogWarn(ctx, common.LogText("Alipay webhook rejected reason=webhook_disabled path=%q client_ip=%s",
			c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusServiceUnavailable, "failure")
		return
	}

	publicKey := normalizeAlipayKey(setting.AlipayPublicKey)
	params, err := alipay.ParseNotifyToBodyMap(c.Request)
	if err != nil {
		logger.LogWarn(ctx, common.LogText("Alipay webhook failed to parse body path=%q client_ip=%s error=%q",
			c.Request.RequestURI, c.ClientIP(), redactKeyMaterial(err.Error())))
		c.String(http.StatusBadRequest, "failure")
		return
	}
	if len(params) == 0 {
		logger.LogWarn(ctx, common.LogText("Alipay webhook params are empty path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusBadRequest, "failure")
		return
	}

	ok, err := alipay.VerifySign(publicKey, params)
	if err != nil || !ok {
		detail := "verify_status=false"
		if err != nil {
			detail = redactKeyMaterial(err.Error())
		}
		logger.LogWarn(ctx, common.LogText("Alipay webhook signature verification failed path=%q client_ip=%s verify_error=%q",
			c.Request.RequestURI, c.ClientIP(), detail))
		c.String(http.StatusBadRequest, "failure")
		return
	}

	if appID := params.GetString("app_id"); appID != "" && appID != setting.AlipayAppId {
		logger.LogWarn(ctx, common.LogText("Alipay webhook app_id mismatch got=%s want=%s client_ip=%s",
			appID, setting.AlipayAppId, c.ClientIP()))
		c.String(http.StatusBadRequest, "failure")
		return
	}

	tradeNo := params.GetString("out_trade_no")
	tradeStatus := params.GetString("trade_status")
	amountFen, amountErr := yuanToFen(params.GetString("total_amount"))
	logger.LogInfo(ctx, common.LogText("Alipay webhook signature verified trade_no=%s trade_status=%s amount_fen=%d provider_trade_no=%s client_ip=%s",
		tradeNo, tradeStatus, amountFen, params.GetString("trade_no"), c.ClientIP()))

	if tradeStatus != "TRADE_SUCCESS" && tradeStatus != "TRADE_FINISHED" {
		// A valid notification that is not a completed payment: acknowledge it so
		// Alipay stops retrying, credit nothing.
		c.String(http.StatusOK, "success")
		return
	}
	if amountErr != nil {
		logger.LogError(ctx, common.LogText("Alipay webhook has an unusable amount trade_no=%s total_amount=%q error=%q",
			tradeNo, params.GetString("total_amount"), amountErr.Error()))
		c.String(http.StatusBadRequest, "failure")
		return
	}

	// The in-process lock only avoids duplicate work; RechargeDirect's row lock
	// and status check are what make repeated callbacks safe (also across
	// instances).
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	alreadyDone, err := model.RechargeDirect(tradeNo, model.PaymentProviderAlipay, amountFen, c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, model.ErrTopUpNotFound):
			logger.LogWarn(ctx, common.LogText("Alipay callback order not found trade_no=%s client_ip=%s", tradeNo, c.ClientIP()))
		case errors.Is(err, model.ErrPaymentMethodMismatch):
			logger.LogWarn(ctx, common.LogText("Alipay callback order payment gateway mismatch trade_no=%s client_ip=%s", tradeNo, c.ClientIP()))
		case errors.Is(err, model.ErrTopUpAmountMismatch):
			logger.LogError(ctx, common.LogText("Alipay callback amount mismatch, credit rejected trade_no=%s notified_fen=%d client_ip=%s",
				tradeNo, amountFen, c.ClientIP()))
		case errors.Is(err, model.ErrTopUpStatusInvalid):
			logger.LogWarn(ctx, common.LogText("Alipay callback order status invalid trade_no=%s client_ip=%s", tradeNo, c.ClientIP()))
		default:
			logger.LogError(ctx, common.LogText("Alipay top-up processing failed trade_no=%s client_ip=%s error=%q",
				tradeNo, c.ClientIP(), err.Error()))
		}
		c.String(http.StatusInternalServerError, "failure")
		return
	}
	if alreadyDone {
		logger.LogInfo(ctx, common.LogText("Alipay duplicate callback ignored trade_no=%s client_ip=%s", tradeNo, c.ClientIP()))
	} else {
		logger.LogInfo(ctx, common.LogText("Alipay top-up succeeded trade_no=%s amount_fen=%d client_ip=%s", tradeNo, amountFen, c.ClientIP()))
	}
	// The literal seven characters Alipay documents; anything else is retried for
	// about 25 hours.
	c.String(http.StatusOK, "success")
}
