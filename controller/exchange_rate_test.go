package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestRefreshExchangeRateEndpoint covers the button the administrator presses
// after wiring the source: it must fetch, persist, and report the number it used,
// because "the endpoint is down" and "the job is disabled" otherwise look the same.
func TestRefreshExchangeRateEndpoint(t *testing.T) {
	previousDB := model.DB
	previousOptionMap := common.OptionMap
	previousURL := setting.ExchangeRateApiUrl
	previousFollow := setting.PriceFollowsExchangeRate
	previousRate, previousPrice := operation_setting.USDExchangeRate, operation_setting.Price

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	model.DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptionMap
		setting.ExchangeRateApiUrl = previousURL
		setting.PriceFollowsExchangeRate = previousFollow
		operation_setting.USDExchangeRate = previousRate
		operation_setting.Price = previousPrice
	})

	rates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rates":{"CNY":7.2543}}`))
	}))
	defer rates.Close()

	setting.ExchangeRateApiUrl = rates.URL
	setting.PriceFollowsExchangeRate = true

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/option/exchange_rate/refresh", RefreshExchangeRate)

	req := httptest.NewRequest(http.MethodPost, "/api/option/exchange_rate/refresh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Rate  float64 `json:"rate"`
			Price float64 `json:"price"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.InDelta(t, 7.2543, body.Data.Rate, 1e-9)
	assert.InDelta(t, 7.2543, body.Data.Price, 1e-9, "Price follows the rate when enabled")

	var stored model.Option
	require.NoError(t, model.DB.Where("key = ?", "USDExchangeRate").First(&stored).Error)
	assert.Equal(t, "7.2543", stored.Value)
}

// TestRefreshExchangeRateEndpointReportsFailure keeps the administrator from
// believing a stale rate is current: a broken source answers 500 with the reason.
func TestRefreshExchangeRateEndpointReportsFailure(t *testing.T) {
	previousURL := setting.ExchangeRateApiUrl
	previousRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		setting.ExchangeRateApiUrl = previousURL
		operation_setting.USDExchangeRate = previousRate
	})

	rates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer rates.Close()
	setting.ExchangeRateApiUrl = rates.URL
	operation_setting.USDExchangeRate = 7.3

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.POST("/api/option/exchange_rate/refresh", RefreshExchangeRate)

	req := httptest.NewRequest(http.MethodPost, "/api/option/exchange_rate/refresh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.InDelta(t, 7.3, operation_setting.USDExchangeRate, 1e-9, "a failure must not move the rate")
}
