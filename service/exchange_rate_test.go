package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

// resetExchangeRateState clears the in-memory bookkeeping the job keeps between
// ticks, so one test cannot hand another a cache-busting decision or a next
// update time it never fetched.
func resetExchangeRateState(t *testing.T) {
	t.Helper()
	exchangeRateMu.Lock()
	exchangeRateNextUpdateAt = 0
	exchangeRateLastBustAt = 0
	exchangeRateLastCheckedAt = 0
	exchangeRateMu.Unlock()
	t.Cleanup(func() {
		exchangeRateMu.Lock()
		exchangeRateNextUpdateAt = 0
		exchangeRateLastBustAt = 0
		exchangeRateLastCheckedAt = 0
		exchangeRateMu.Unlock()
	})
}

// seedExchangeRateState puts the loop in the position it would be in after a
// fetch that advertised nextUpdateAt and, if it was a cache-busting fetch, ran
// at lastBustAt.
func seedExchangeRateState(t *testing.T, nextUpdateAt, lastBustAt int64) {
	t.Helper()
	exchangeRateMu.Lock()
	exchangeRateNextUpdateAt = nextUpdateAt
	exchangeRateLastBustAt = lastBustAt
	exchangeRateMu.Unlock()
}

// withRateSettings saves and restores everything the refresh touches, so one
// test cannot leave the suite priced at a different rate.
func withRateSettings(t *testing.T, url string, followsPrice bool) {
	t.Helper()
	previousURL, previousFollow := setting.ExchangeRateApiUrl, setting.PriceFollowsExchangeRate
	previousRate, previousPrice := operation_setting.USDExchangeRate, operation_setting.Price
	previousUpdatedAt := setting.ExchangeRateUpdatedAt
	setting.ExchangeRateApiUrl = url
	setting.PriceFollowsExchangeRate = followsPrice
	resetExchangeRateState(t)
	t.Cleanup(func() {
		setting.ExchangeRateApiUrl = previousURL
		setting.PriceFollowsExchangeRate = previousFollow
		operation_setting.USDExchangeRate = previousRate
		operation_setting.Price = previousPrice
		setting.ExchangeRateUpdatedAt = previousUpdatedAt
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
	rate, _, err := fetchUSDCNY(context.Background(), server.URL, false)
	require.NoError(t, err)
	assert.InDelta(t, 7.2543, rate, 1e-9)
}

func TestFetchUSDCNYReadsNextUpdate(t *testing.T) {
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.2543},"time_next_update_unix":1794240000}`)
	_, nextUpdate, err := fetchUSDCNY(context.Background(), server.URL, false)
	require.NoError(t, err)
	assert.EqualValues(t, 1794240000, nextUpdate)
}

// TestFetchUSDCNYCacheBusterOnlyWhenAsked covers the part the CDN actually
// respects: a plain poll stays cacheable, while a call made after the source said
// it changed must reach the origin.
func TestFetchUSDCNYCacheBusterOnlyWhenAsked(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"rates":{"CNY":7.25}}`))
	}))
	t.Cleanup(server.Close)

	_, _, err := fetchUSDCNY(context.Background(), server.URL, false)
	require.NoError(t, err)
	_, _, err = fetchUSDCNY(context.Background(), server.URL, true)
	require.NoError(t, err)

	require.Len(t, queries, 2)
	assert.Empty(t, queries[0], "a normal poll should stay cacheable")
	assert.Contains(t, queries[1], "t=", "a confirmed update must bypass the cache")
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
			_, _, err := fetchUSDCNY(context.Background(), server.URL, false)
			assert.Error(t, err, "an unusable response must not become a price")
		})
	}
}

func TestFetchUSDCNYRejectsEmptyEndpoint(t *testing.T) {
	_, _, err := fetchUSDCNY(context.Background(), "  ", false)
	assert.Error(t, err)
}

// TestRefreshExchangeRateWritesBothSettings is the USD-priced deployment: the
// rate updates the display conversion and, because the site asked for it, the
// price it charges in renminbi.
func TestRefreshExchangeRateWritesBothSettings(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31}}`)
	withRateSettings(t, server.URL, true)

	rate, changed, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.True(t, changed)
	assert.InDelta(t, 7.31, rate, 1e-9)

	assert.InDelta(t, 7.31, operation_setting.USDExchangeRate, 1e-9)
	assert.InDelta(t, 7.31, operation_setting.Price, 1e-9, "Price follows the rate when asked")

	var stored model.Option
	require.NoError(t, model.DB.Where("key = ?", "Price").First(&stored).Error)
	assert.Equal(t, "7.31", stored.Value, "the new price must survive a restart")
	assert.Greater(t, setting.ExchangeRateUpdatedAt, time.Now().Add(-time.Minute).Unix())
}

// TestRefreshExchangeRateKeepsFixedPrice is the CNY-native deployment: one unit
// stays one yuan and only the display conversion moves.
func TestRefreshExchangeRateKeepsFixedPrice(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31}}`)
	withRateSettings(t, server.URL, false)
	operation_setting.Price = 1

	_, _, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)

	assert.InDelta(t, 7.31, operation_setting.USDExchangeRate, 1e-9)
	assert.InDelta(t, 1.0, operation_setting.Price, 1e-9, "a fixed unit price must not move")
}

// TestRefreshExchangeRateSkipsUnchangedWrites is the reason the job no longer
// produces 864 UPDATEs a day: a source that answers the value we already hold
// must not touch the options table at all.
func TestRefreshExchangeRateSkipsUnchangedWrites(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31},"time_next_update_unix":1794240000}`)
	withRateSettings(t, server.URL, true)
	operation_setting.USDExchangeRate = 7.31
	operation_setting.Price = 7.31
	setting.ExchangeRateUpdatedAt = 111

	rate, changed, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.False(t, changed, "an unchanged rate must not be reported as persisted")
	assert.InDelta(t, 7.31, rate, 1e-9)

	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count, "no option row should be written when nothing changed")
	assert.EqualValues(t, 111, setting.ExchangeRateUpdatedAt, "the last-change time must stay put")
}

// TestRefreshExchangeRatePersistsOnlyOnChange keeps the two behaviours apart:
// the first differing value is written, the identical second poll is not.
func TestRefreshExchangeRatePersistsOnlyOnChange(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":7.31}}`)
	withRateSettings(t, server.URL, true)
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.3

	_, changed, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Greater(t, setting.ExchangeRateUpdatedAt, time.Now().Add(-time.Minute).Unix())
	changedAt := setting.ExchangeRateUpdatedAt

	_, changed, err = RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, changedAt, setting.ExchangeRateUpdatedAt, "a no-op poll must not rewrite the timestamp")
}

// TestRefreshExchangeRateBustsCacheOnlyWhenUpdateIsDue covers the second half of
// the fix: with the source's next-update time known, the loop reads the cacheable
// copy until that moment, then asks the origin once.
func TestRefreshExchangeRateBustsCacheOnlyWhenUpdateIsDue(t *testing.T) {
	setupExchangeRateTestDB(t)
	var busted int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "" {
			atomic.StoreInt32(&busted, 1)
		}
		_, _ = w.Write([]byte(`{"rates":{"CNY":7.31},"time_next_update_unix":1794240000}`))
	}))
	t.Cleanup(server.Close)
	withRateSettings(t, server.URL, true)
	operation_setting.USDExchangeRate = 7.31
	operation_setting.Price = 7.31

	now := time.Now().Unix()
	seedExchangeRateState(t, now+3600, now)
	_, _, err := RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.Zero(t, atomic.LoadInt32(&busted), "the source has not said it changed yet")
	assert.EqualValues(t, 1794240000, ExchangeRateNextUpdateAt())

	seedExchangeRateState(t, now-60, now-2*int64(exchangeRateBustMinGap/time.Second))
	_, _, err = RefreshExchangeRate(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt32(&busted), "past the advertised update the origin must be asked")
}

func TestRefreshExchangeRateLeavesSettingsOnFailure(t *testing.T) {
	setupExchangeRateTestDB(t)
	server := rateServer(t, http.StatusOK, `{"rates":{"CNY":0.1}}`)
	withRateSettings(t, server.URL, true)
	operation_setting.USDExchangeRate = 7.3
	operation_setting.Price = 7.3

	_, _, err := RefreshExchangeRate(context.Background())
	require.Error(t, err)
	assert.InDelta(t, 7.3, operation_setting.USDExchangeRate, 1e-9, "a bad reading must not overwrite the good one")
	assert.InDelta(t, 7.3, operation_setting.Price, 1e-9)
}
