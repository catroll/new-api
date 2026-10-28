package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupExchangeRateTestDB gives RefreshExchangeRate somewhere to persist. It
// keeps the options table only: the refresh path writes nothing else.
func setupExchangeRateTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousOptionMap := common.OptionMap
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	model.DB = db
	// updateOptionMap writes into the process-wide option map, which is nil until
	// the server initialises it.
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptionMap
	})
}

// withRateSettings saves and restores everything the refresh touches, so one
// test cannot leave the suite priced at a different rate.
func withRateSettings(t *testing.T, url string, followsPrice bool) {
	t.Helper()
	previousURL, previousFollow := setting.ExchangeRateApiUrl, setting.PriceFollowsExchangeRate
	previousRate, previousPrice := operation_setting.USDExchangeRate, operation_setting.Price
	setting.ExchangeRateApiUrl = url
	setting.PriceFollowsExchangeRate = followsPrice
	t.Cleanup(func() {
		setting.ExchangeRateApiUrl = previousURL
		setting.PriceFollowsExchangeRate = previousFollow
		operation_setting.USDExchangeRate = previousRate
		operation_setting.Price = previousPrice
	})
}

func rateServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchUSDCNYParsesTheCNYRate(t *testing.T) {
	server := rateServer(t, http.StatusOK, `{"result":"success","rates":{"USD":1,"CNY":7.2543}}`)
	rate, err := fetchUSDCNY(context.Background(), server.URL)
	require.NoError(t, err)
	assert.InDelta(t, 7.2543, rate, 1e-9)
}

func TestFetchUSDCNYRejectsUnusableResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"http error", http.StatusBadGateway, `{"rates":{"CNY":7.25}}`},
		{"no cny rate", http.StatusOK, `{"result":"success","rates":{"EUR":0.9}}`},
		{"not json", http.StatusOK, `<html>blocked</html>`},
		// A feed that answers 1 or 100 would otherwise reprice the catalogue.
		{"absurdly low", http.StatusOK, `{"rates":{"CNY":0.5}}`},
		{"absurdly high", http.StatusOK, `{"rates":{"CNY":420}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := rateServer(t, tc.status, tc.body)
			_, err := fetchUSDCNY(context.Background(), server.URL)
			assert.Error(t, err, "an unusable response must not become a price")
		})
	}
}

func TestFetchUSDCNYRejectsEmptyEndpoint(t *testing.T) {
	_, err := fetchUSDCNY(context.Background(), "  ")
	assert.Error(t, err)
}

// TestRefreshExchangeRateWritesBothSettings is the USD-priced deployment: the
// rate updates the display conversion and, because the site asked for it, the
// price it charges in renminbi.
func TestRefreshExchangeRateWritesBothSettings(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31}}`)
	withRateSettings(t, server.URL, true)

	rate, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 7.31, rate, 1e-9)

	assert.InDelta(t, 7.31, operation_setting.USDExchangeRate, 1e-9)
	assert.InDelta(t, 7.31, operation_setting.Price, 1e-9, "Price follows the rate when asked")

	var stored model.Option
	require.NoError(t, model.DB.Where("key = ?", "Price").First(&stored).Error)
	assert.Equal(t, "7.31", stored.Value, "the new price must survive a restart")
}

// TestRefreshExchangeRateKeepsFixedPrice is the CNY-native deployment: one unit
// stays one yuan and only the display conversion moves.
func TestRefreshExchangeRateKeepsFixedPrice(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31}}`)
	withRateSettings(t, server.URL, false)
	operation_setting.Price = 1

	_, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)

	assert.InDelta(t, 7.31, operation_setting.USDExchangeRate, 1e-9)
	assert.InDelta(t, 1.0, operation_setting.Price, 1e-9, "a fixed unit price must not move")
}

func TestRefreshExchangeRateLeavesSettingsOnFailure(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":0.1}}`)
	withRateSettings(t, server.URL, true)
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.3

	_, err := RefreshExchangeRate(context.Background())
	require.Error(t, err)
	assert.InDelta(t, 7.3, operation_setting.USDExchangeRate, 1e-9, "a bad reading must not overwrite the good one")
	assert.InDelta(t, 7.3, operation_setting.Price, 1e-9)
}
