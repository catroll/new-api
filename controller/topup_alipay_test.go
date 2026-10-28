package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-pay/gopay"
	"github.com/go-pay/gopay/alipay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// --- harness ---------------------------------------------------------------

type alipayTestKeys struct {
	priv    *rsa.PrivateKey
	privPEM string
	pubB64  string
}

func newAlipayTestKeys(t *testing.T) alipayTestKeys {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	return alipayTestKeys{
		priv:    priv,
		privPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
		pubB64:  base64.StdEncoding.EncodeToString(pubDER),
	}
}

// setupAlipayTestDB installs an in-memory database with the tables the credit
// path touches: users (quota), top_ups (the order), logs (the top-up record).
func setupAlipayTestDB(t *testing.T) *model.User {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled

	// model.InitDB() initialises the dialect-specific column names
	// (`group` vs "group") that GetUserGroup relies on. It opens a throwaway
	// SQLite database; the in-memory one below replaces it, but without this call
	// the column names stay empty and every GROUP query fails with a syntax error
	// — which only shows up when this test runs alone.
	require.NoError(t, model.InitDB())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Option{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	user := &model.User{
		Username: "alipay-tester",
		Password: "placeholder",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(user).Error)

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
	})
	return user
}

func enableAlipayForTest(t *testing.T, keys alipayTestKeys) {
	t.Helper()
	previousCompliance := operation_setting.GetPaymentSetting()
	previousAppID, previousPriv, previousPub := setting.AlipayAppId, setting.AlipayPrivateKey, setting.AlipayPublicKey
	previousSandbox := setting.AlipaySandbox
	previousMinTopUp := setting.AlipayMinTopUp
	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousPrice := operation_setting.Price
	previousMin := operation_setting.MinTopUp
	previousPaymentCurrency := setting.PaymentCurrency

	confirmPaymentComplianceForTest(t)
	setting.AlipayAppId = "2021000000000001"
	setting.AlipayPrivateKey = keys.privPEM
	setting.AlipayPublicKey = keys.pubB64
	setting.AlipaySandbox = true
	setting.AlipayMinTopUp = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.Price = 7.3
	operation_setting.MinTopUp = 1

	t.Cleanup(func() {
		setting.AlipayAppId, setting.AlipayPrivateKey, setting.AlipayPublicKey = previousAppID, previousPriv, previousPub
		setting.AlipaySandbox = previousSandbox
		setting.AlipayMinTopUp = previousMinTopUp
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		operation_setting.Price = previousPrice
		operation_setting.MinTopUp = previousMin
		setting.PaymentCurrency = previousPaymentCurrency
		_ = previousCompliance
	})
}

// signedAlipayNotify builds the form Alipay would POST. Alipay signs every
// parameter except sign and sign_type, so those are added after signing.
func signedAlipayNotify(t *testing.T, keys alipayTestKeys, tradeNo, totalAmount, status string) url.Values {
	t.Helper()
	now := time.Now().Format("2006-01-02 15:04:05")
	bm := gopay.BodyMap{
		"notify_id":      "2026102400000000000000000001",
		"notify_type":    "trade_status_sync",
		"notify_time":    now,
		"app_id":         setting.AlipayAppId,
		"charset":        "utf-8",
		"version":        "1.0",
		"trade_no":       "2026102422001400000000000001",
		"out_trade_no":   tradeNo,
		"trade_status":   status,
		"total_amount":   totalAmount,
		"receipt_amount": totalAmount,
		"seller_id":      "2088000000000001",
		"buyer_logon_id": "tester@example.com",
		"gmt_payment":    now,
	}
	sign, err := alipay.GetRsaSign(bm, alipay.RSA2, keys.priv)
	require.NoError(t, err)

	form := url.Values{}
	for k, v := range bm {
		if s, ok := v.(string); ok {
			form.Set(k, s)
		}
	}
	form.Set("sign", sign)
	form.Set("sign_type", alipay.RSA2)
	return form
}

func newTestTopUp(t *testing.T, userID int, tradeNo string, amount int64, money float64) *model.TopUp {
	t.Helper()
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipayDirect,
		PaymentProvider: model.PaymentProviderAlipay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	return topUp
}

func postAlipayNotify(engine http.Handler, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/alipay/notify",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.9:12345"
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func alipayTestRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST("/api/alipay/notify", AlipayNotify)
	return r
}

// --- gate ------------------------------------------------------------------

func TestAlipayTopUpEnabledRequiresComplianceCNYAndCredentials(t *testing.T) {
	keys := newAlipayTestKeys(t)
	previousAppID, previousPriv, previousPub := setting.AlipayAppId, setting.AlipayPrivateKey, setting.AlipayPublicKey
	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		setting.AlipayAppId, setting.AlipayPrivateKey, setting.AlipayPublicKey = previousAppID, previousPriv, previousPub
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
	})

	setting.AlipayAppId = "2021000000000001"
	setting.AlipayPrivateKey = keys.privPEM
	setting.AlipayPublicKey = keys.pubB64
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY

	// Compliance is not confirmed: hidden regardless of credentials.
	require.False(t, isAlipayTopUpEnabled())

	confirmPaymentComplianceForTest(t)
	require.True(t, isAlipayTopUpEnabled())

	// A dollar-priced catalogue still charges renminbi: the display currency must
	// not disable the gateway, but a non-CNY payment currency must.
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.True(t, isAlipayTopUpEnabled(), "USD display with a CNY charge is a supported configuration")
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	setting.PaymentCurrency = "USD"
	require.False(t, isAlipayTopUpEnabled(), "only CNY can be charged today")
	setting.PaymentCurrency = "CNY"

	for _, empty := range []*string{&setting.AlipayAppId, &setting.AlipayPrivateKey, &setting.AlipayPublicKey} {
		original := *empty
		*empty = ""
		require.False(t, isAlipayTopUpEnabled(), "a missing credential must disable the gateway")
		*empty = original
	}
	require.True(t, isAlipayTopUpEnabled())
}

func TestReplaceOrAppendPayMethodKeepsOrder(t *testing.T) {
	existing := []map[string]string{
		{"type": "alipay", "name": "支付宝"},
		{"type": "wxpay", "name": "微信"},
	}
	replacement := map[string]string{"type": model.PaymentMethodAlipayDirect, "name": "支付宝"}
	out := replaceOrAppendPayMethod(existing, "alipay", replacement)
	require.Len(t, out, 2)
	assert.Equal(t, model.PaymentMethodAlipayDirect, out[0]["type"], "replaced in place")
	assert.Equal(t, "wxpay", out[1]["type"])

	appended := replaceOrAppendPayMethod([]map[string]string{{"type": "wxpay"}}, "alipay", replacement)
	require.Len(t, appended, 2)
	assert.Equal(t, "wxpay", appended[0]["type"])
	assert.Equal(t, model.PaymentMethodAlipayDirect, appended[1]["type"])
}

// --- notify ----------------------------------------------------------------

func TestAlipayNotifyCreditsExactlyOnce(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	const tradeNo = "USR1NOCREDITS001"
	topUp := newTestTopUp(t, user.Id, tradeNo, 10, 0.01) // 10 quota units, ¥0.01

	engine := alipayTestRouter()
	form := signedAlipayNotify(t, keys, tradeNo, "0.01", "TRADE_SUCCESS")

	rec := postAlipayNotify(engine, form)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())

	var stored model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.NotZero(t, stored.CompleteTime)

	var refreshed model.User
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	expectedQuota := int(float64(topUp.Amount) * common.QuotaPerUnit)
	assert.Equal(t, expectedQuota, refreshed.Quota)

	var logCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeTopup).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)

	completeTime := stored.CompleteTime
	quotaAfterFirst := refreshed.Quota

	// The platform retries for up to 25 hours; the second delivery must not credit.
	rec = postAlipayNotify(engine, form)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())

	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
	assert.Equal(t, completeTime, stored.CompleteTime, "a duplicate must not touch the order")
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	assert.Equal(t, quotaAfterFirst, refreshed.Quota, "a duplicate must not add quota")
}

func TestAlipayNotifyRejectsAmountMismatch(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	const tradeNo = "USR1NOMISMATCH01"
	newTestTopUp(t, user.Id, tradeNo, 100, 1.00) // order is ¥1.00

	engine := alipayTestRouter()
	// Signed correctly, but for a different amount: the signature passes and the
	// amount check is what must reject it.
	form := signedAlipayNotify(t, keys, tradeNo, "0.01", "TRADE_SUCCESS")
	rec := postAlipayNotify(engine, form)
	require.NotEqual(t, http.StatusOK, rec.Code)
	assert.Equal(t, "failure", rec.Body.String())

	var stored model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	assert.Zero(t, stored.CompleteTime)

	var refreshed model.User
	require.NoError(t, model.DB.First(&refreshed, user.Id).Error)
	assert.Zero(t, refreshed.Quota)
}

func TestAlipayNotifyRejectsBadSignature(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	const tradeNo = "USR1NOBADSIG001"
	newTestTopUp(t, user.Id, tradeNo, 10, 0.01)

	attacker := newAlipayTestKeys(t)
	engine := alipayTestRouter()
	form := signedAlipayNotify(t, attacker, tradeNo, "0.01", "TRADE_SUCCESS")
	rec := postAlipayNotify(engine, form)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "failure", rec.Body.String())

	var stored model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
}

func TestAlipayNotifyRejectsForeignAppID(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	const tradeNo = "USR1NOAPPID001"
	newTestTopUp(t, user.Id, tradeNo, 10, 0.01)

	engine := alipayTestRouter()
	form := signedAlipayNotify(t, keys, tradeNo, "0.01", "TRADE_SUCCESS")
	form.Set("app_id", "2021999999999999")

	rec := postAlipayNotify(engine, form)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "failure", rec.Body.String())
}

func TestAlipayNotifyNonSuccessStatusIsAcknowledgedButNotCredited(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	const tradeNo = "USR1NOWAITPAY001"
	newTestTopUp(t, user.Id, tradeNo, 10, 0.01)

	engine := alipayTestRouter()
	form := signedAlipayNotify(t, keys, tradeNo, "0.01", "WAIT_BUYER_PAY")
	rec := postAlipayNotify(engine, form)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String(), "acknowledge so Alipay stops retrying")

	var stored model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", tradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
}

func TestAlipayNotifyRejectsWhenGatewayDisabled(t *testing.T) {
	engine := alipayTestRouter()
	form := url.Values{"out_trade_no": {"USR1NOANYTHING"}}
	rec := postAlipayNotify(engine, form)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "failure", rec.Body.String())
}

// --- pay -------------------------------------------------------------------

// TestRequestAlipayPayBuildsGatewayURL proves the request path without any
// network call: trading the URL is local signing, so a wrong key or a missing
// parameter shows up here rather than during a live payment.
func TestRequestAlipayPayBuildsGatewayURL(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/user/alipay/pay", func(c *gin.Context) {
		c.Set("id", user.Id)
		RequestAlipayPay(c)
	})

	body, err := json.Marshal(AlipayPayRequest{Amount: 1, PaymentMethod: model.PaymentMethodAlipayDirect})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var response struct {
		Message string `json:"message"`
		Data    struct {
			PayURL  string `json:"pay_url"`
			TradeNo string `json:"trade_no"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "success", response.Message)
	require.NotEmpty(t, response.Data.TradeNo)

	var stored model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", response.Data.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
	assert.Equal(t, model.PaymentProviderAlipay, stored.PaymentProvider)
	assert.Equal(t, model.PaymentMethodAlipayDirect, stored.PaymentMethod)

	assert.Contains(t, response.Data.PayURL, "alipaydev.com/gateway.do", "sandbox gateway for a sandbox config")
	assert.Contains(t, response.Data.PayURL, "out_trade_no")
	assert.Contains(t, response.Data.PayURL, "sign=")
	assert.Contains(t, response.Data.PayURL, "total_amount")

	// Money is what the gateway charges: 1 quota unit × Price (7.3 CNY).
	assert.InDelta(t, 7.3, stored.Money, 0.001)
	assert.Equal(t, int64(1), stored.Amount, "the stored amount stays in quota units")
}

func TestNormalizeAlipayKeyAcceptsEveryShape(t *testing.T) {
	keys := newAlipayTestKeys(t)
	bare := normalizeAlipayKey(keys.privPEM)
	require.NotEmpty(t, bare)
	assert.NotContains(t, bare, "BEGIN")
	assert.NotContains(t, bare, "\n")

	// PEM, PEM with literal \n escapes, and a bare body all reduce to the same
	// base64 string.
	assert.Equal(t, bare, normalizeAlipayKey(strings.ReplaceAll(keys.privPEM, "\n", `\n`)))
	assert.Equal(t, bare, normalizeAlipayKey("  "+bare+"  "))

	// The console's key field is a single-line input: the browser removes the
	// newlines when a PEM is pasted into it, so the value that reaches the server
	// looks like "-----BEGIN ...----- MIIE... -----END ...-----" on one line.
	// Stripping "lines that start with -----" used to erase the whole key and the
	// gateway then reported "missing alipay init parameter".
	collapsed := strings.Join(strings.Fields(keys.privPEM), " ")
	require.Contains(t, collapsed, "-----BEGIN")
	assert.Equal(t, bare, normalizeAlipayKey(collapsed))

	// And the collapsed form has to produce a working client, not just a string.
	client, err := alipay.NewClient("2021000000000001", normalizeAlipayKey(collapsed), true)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestYuanToFenAlipay(t *testing.T) {
	cases := map[string]int64{"0.01": 1, "1": 100, "1.5": 150, "14.30": 1430, " 7.30 ": 730}
	for in, want := range cases {
		got, err := yuanToFen(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "abc", "-1"} {
		_, err := yuanToFen(bad)
		assert.Error(t, err, bad)
	}
}

// The wallet displays setting.AlipayMinTopUp, so the handler has to enforce that
// number rather than the global MinTopUp.
func TestRequestAlipayAmountEnforcesGatewayMinimum(t *testing.T) {
	keys := newAlipayTestKeys(t)
	enableAlipayForTest(t, keys)
	previousMin := setting.AlipayMinTopUp
	setting.AlipayMinTopUp = 5
	t.Cleanup(func() { setting.AlipayMinTopUp = previousMin })

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/user/alipay/amount", RequestAlipayAmount)

	body, err := json.Marshal(AlipayPayRequest{Amount: 3, PaymentMethod: model.PaymentMethodAlipayDirect})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/alipay/amount", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// paymentError answers 200 with a message payload.
	var response struct {
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.NotEqual(t, "success", response.Message)
	assert.Contains(t, rec.Body.String(), "5", "the message must quote the Alipay minimum")
}

// A USD-quoted console would charge CNY for a USD number; refuse it server-side
// even though the wallet hides the button.
func TestRequestAlipayPayRejectsNonCNYPayment(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)
	setting.PaymentCurrency = "USD"

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/user/alipay/pay", func(c *gin.Context) {
		c.Set("id", user.Id)
		RequestAlipayPay(c)
	})

	body, err := json.Marshal(AlipayPayRequest{Amount: 1, PaymentMethod: model.PaymentMethodAlipayDirect})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotContains(t, rec.Body.String(), "pay_url")
	assert.Contains(t, rec.Body.String(), "CNY")

	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count, "a rejected request must not leave an order behind")
}

// TestRequestAlipayPayQuotesCNYForUSDDisplay is the hub platform's shape: the
// catalogue and the wallet speak dollars, the buyer pays renminbi, and Price is
// the exchange rate.
func TestRequestAlipayPayQuotesCNYForUSDDisplay(t *testing.T) {
	keys := newAlipayTestKeys(t)
	user := setupAlipayTestDB(t)
	enableAlipayForTest(t, keys)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.Price = 7.3

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/user/alipay/pay", func(c *gin.Context) {
		c.Set("id", user.Id)
		RequestAlipayPay(c)
	})

	body, err := json.Marshal(AlipayPayRequest{Amount: 10, PaymentMethod: model.PaymentMethodAlipayDirect})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/alipay/pay", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "pay_url")

	var stored model.TopUp
	require.NoError(t, model.DB.Order("id desc").First(&stored).Error)
	assert.Equal(t, int64(10), stored.Amount, "the order keeps the unit count")
	assert.InDelta(t, 73.0, stored.Money, 0.001, "ten dollars of credit costs 73 yuan")
}
