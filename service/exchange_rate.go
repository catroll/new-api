package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// The rate endpoint is third-party data on the path that decides how much money
// an order costs, so a response outside this window is treated as garbage rather
// than a bargain. A USD/CNY rate has not left this range in twenty years.
const (
	exchangeRateMin     = 3.0
	exchangeRateMax     = 15.0
	exchangeRateTimeout = 10 * time.Second
	// exchangeRateIdlePoll is how often the loop re-reads the interval while the
	// job is disabled, so enabling it in the console takes effect without a
	// restart.
	exchangeRateIdlePoll = 30 * time.Second
)

type exchangeRatePayload struct {
	Rates map[string]float64 `json:"rates"`
}

// fetchUSDCNY asks the configured endpoint for the dollar price in renminbi. The
// shape is the open.er-api.com one: {"result":"success","rates":{"CNY":7.25}}.
func fetchUSDCNY(ctx context.Context, url string) (float64, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return 0, fmt.Errorf("exchange rate endpoint is not configured")
	}

	reqCtx, cancel := context.WithTimeout(ctx, exchangeRateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "new-api/exchange-rate")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("exchange rate endpoint returned %d", resp.StatusCode)
	}

	var payload exchangeRatePayload
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode exchange rate response: %w", err)
	}
	rate, ok := payload.Rates["CNY"]
	if !ok {
		return 0, fmt.Errorf("exchange rate response has no CNY rate")
	}
	if rate < exchangeRateMin || rate > exchangeRateMax {
		return 0, fmt.Errorf("exchange rate %.4f is outside the accepted range %.1f-%.1f", rate, exchangeRateMin, exchangeRateMax)
	}
	return rate, nil
}

// RefreshExchangeRate fetches the current rate and persists it. USDExchangeRate
// always follows the market; Price follows it only when the site explicitly asked
// for that, because on a CNY-native site Price is a fixed unit price and moving it
// would reprice the catalogue.
//
// Both writes go through UpdateOption, which refreshes the in-memory value, so the
// next order is quoted with the new rate without a restart.
func RefreshExchangeRate(ctx context.Context) (float64, error) {
	rate, err := fetchUSDCNY(ctx, setting.ExchangeRateApiUrl)
	if err != nil {
		return 0, err
	}

	rateText := strconv.FormatFloat(rate, 'f', -1, 64)
	if err = model.UpdateOption("USDExchangeRate", rateText); err != nil {
		return 0, fmt.Errorf("persist USDExchangeRate: %w", err)
	}

	if setting.PriceFollowsExchangeRate {
		if err = model.UpdateOption("Price", rateText); err != nil {
			return 0, fmt.Errorf("persist Price: %w", err)
		}
	}

	// The timestamp is metadata: losing it must not fail a refresh whose rate is
	// already written, so it is reported and otherwise ignored.
	if err = model.UpdateOption("ExchangeRateUpdatedAt", strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
		logger.LogWarn(ctx, common.LogText("persist ExchangeRateUpdatedAt failed error=%q", err.Error()))
	}

	logger.LogInfo(ctx, common.LogText("exchange rate refreshed usd_cny=%s price=%s price_follows=%t",
		rateText, strconv.FormatFloat(operation_setting.Price, 'f', -1, 64), setting.PriceFollowsExchangeRate))
	return rate, nil
}

// StartExchangeRateJob refreshes the rate on the configured interval. It runs for
// the lifetime of the process; the interval is re-read every tick so the console
// switch takes effect without a restart, and 0 pauses instead of exiting.
func StartExchangeRateJob() {
	go func() {
		for {
			minutes := setting.ExchangeRateRefreshMinutes
			if minutes <= 0 {
				time.Sleep(exchangeRateIdlePoll)
				continue
			}
			if _, err := RefreshExchangeRate(context.Background()); err != nil {
				logger.LogWarn(context.Background(), common.LogText("exchange rate refresh failed error=%q", err.Error()))
			}
			time.Sleep(time.Duration(minutes) * time.Minute)
		}
	}()
}
