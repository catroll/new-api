package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type topUpInfoPayload struct {
	Success bool `json:"success"`
	Data    struct {
		EnableOnlineTopup bool                `json:"enable_online_topup"`
		EnableAlipay      bool                `json:"enable_alipay_topup"`
		PayMethods        []map[string]string `json:"pay_methods"`
	} `json:"data"`
}

func renderTopUpInfo(t *testing.T) topUpInfoPayload {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/api/user/topup/info", GetTopUpInfo)

	req := httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body topUpInfoPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Success)
	return body
}

// TestTopUpInfoOnlyAdvertisesConfiguredGateways covers the wallet showing three
// buttons on a site that had wired exactly one: the aggregator's channel list is
// seeded with 支付宝/微信/自定义1 and used to be returned even with no merchant
// account behind it.
func TestTopUpInfoOnlyAdvertisesConfiguredGateways(t *testing.T) {
	keys := newAlipayTestKeys(t)
	enableAlipayForTest(t, keys)
	previousAddress, previousID, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousID, previousKey
	})
	operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = "", "", ""

	body := renderTopUpInfo(t)
	require.True(t, body.Data.EnableAlipay)
	assert.False(t, body.Data.EnableOnlineTopup)
	assert.Len(t, body.Data.PayMethods, 1, "only the gateway that can actually charge")
	assert.Equal(t, model.PaymentMethodAlipayDirect, body.Data.PayMethods[0]["type"])
}

// TestTopUpInfoKeepsAggregatorChannelsWhenConfigured is the other half: a site
// with a real aggregator account still gets the whole channel list.
func TestTopUpInfoKeepsAggregatorChannelsWhenConfigured(t *testing.T) {
	keys := newAlipayTestKeys(t)
	enableAlipayForTest(t, keys)
	previousAddress, previousID, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousID, previousKey
	})
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId, operation_setting.EpayKey = "1000", "secret"

	body := renderTopUpInfo(t)
	require.True(t, body.Data.EnableOnlineTopup)
	assert.Len(t, body.Data.PayMethods, 3, "alipay is replaced in place, 微信 and 自定义1 remain")
	assert.Equal(t, model.PaymentMethodAlipayDirect, body.Data.PayMethods[0]["type"])
	assert.Equal(t, "wxpay", body.Data.PayMethods[1]["type"])
	assert.Equal(t, "custom1", body.Data.PayMethods[2]["type"])
}
