package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	// exchangeRateBustMinGap bounds how often a refresh may bypass an
	// intermediary cache. The free open.er-api.com endpoint publishes once a day
	// and asks clients to cache for that long; when it does not advertise its
	// next update time, or that time is already past (late publish, clock skew),
	// one origin request per hour stays well inside its rate limit and still
	// finds a new value within an hour.
	exchangeRateBustMinGap = time.Hour
)

type exchangeRatePayload struct {
	Rates map[string]float64 `json:"rates"`
	// TimeNextUpdateUnix is the only reliable signal that the source has a new
	// value: the free plan refreshes once a day, so the payload says when the
	// next refresh is due. Without it the 5-minute loop would keep reading a
	// cached copy for up to an hour after the source changed.
	TimeNextUpdateUnix int64 `json:"time_next_update_unix"`
}

var (
	exchangeRateMu sync.Mutex
	// exchangeRateNextUpdateAt is the next change time advertised by the source,
	// unix seconds. Zero when the endpoint does not report one.
	exchangeRateNextUpdateAt int64
	// exchangeRateLastBustAt is when the loop last bypassed an intermediary cache.
	exchangeRateLastBustAt int64
	// exchangeRateLastCheckedAt is when a fetch last completed successfully.
	exchangeRateLastCheckedAt int64
)

// appendCacheBuster makes a URL unique so an intermediary cache (Cloudflare in
// front of open.er-api.com keeps responses for an hour) has to forward the
// request to the origin. A Cache-Control: no-cache request header is not enough:
// the CDN ignores it, so the query parameter is the part that actually works.
func appendCacheBuster(url string, token int64) string {
	separator := "?"
	if strings.Contains(url, "?") {
		separator = "&"
	}
	return url + separator + "t=" + strconv.FormatInt(token, 10)
}

// fetchUSDCNY asks the configured endpoint for the dollar price in renminbi. The
// shape is the open.er-api.com one: {"result":"success","rates":{"CNY":7.25}}.
// When bustCache is set it also returns the next update time the source
// advertises (0 when the endpoint does not send one).
func fetchUSDCNY(ctx context.Context, url string, bustCache bool) (float64, int64, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return 0, 0, fmt.Errorf("exchange rate endpoint is not configured")
	}
	if bustCache {
		url = appendCacheBuster(url, time.Now().UnixNano())
	}

	reqCtx, cancel := context.WithTimeout(ctx, exchangeRateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "new-api/exchange-rate")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("exchange rate endpoint returned %d", resp.StatusCode)
	}

	var payload exchangeRatePayload
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, 0, fmt.Errorf("decode exchange rate response: %w", err)
	}
	rate, ok := payload.Rates["CNY"]
	if !ok {
		return 0, 0, fmt.Errorf("exchange rate response has no CNY rate")
	}
	if rate < exchangeRateMin || rate > exchangeRateMax {
		return 0, 0, fmt.Errorf("exchange rate %.4f is outside the accepted range %.1f-%.1f", rate, exchangeRateMin, exchangeRateMax)
	}
	return rate, payload.TimeNextUpdateUnix, nil
}

// shouldBustExchangeRateCacheLocked decides whether this tick must go past any
// response cache. exchangeRateMu must be held by the caller.
func shouldBustExchangeRateCacheLocked(now int64) bool {
	if exchangeRateNextUpdateAt == 0 {
		// The endpoint does not say when it updates. One origin request per hour
		// still surfaces a daily update within an hour without hammering it, and
		// the very first tick forces one so a fresh process is never stuck on a
		// stale cached copy.
		return exchangeRateLastBustAt == 0 || now-exchangeRateLastBustAt >= int64(exchangeRateBustMinGap/time.Second)
	}
	if now < exchangeRateNextUpdateAt {
		return false
	}
	// The advertised update time has passed: the cached copy is older than the
	// source may allow, so ask the origin. The gap keeps a late or misbehaving
	// source from turning every tick into an origin request.
	return exchangeRateLastBustAt == 0 || now-exchangeRateLastBustAt >= int64(exchangeRateBustMinGap/time.Second)
}

// sameRate compares two rates at the precision the provider reports them, so a
// value that merely carries float noise is not persisted again.
func sameRate(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// RefreshExchangeRate fetches the current rate and persists it when it changed.
// USDExchangeRate always follows the market; Price follows it only when the site
// explicitly asked for that, because on a CNY-native site Price is a fixed unit
// price and moving it would reprice the catalogue.
//
// A no-op tick writes nothing: every write goes through UpdateOption, which saves
// the row and refreshes the in-memory value, so persisting an unchanged rate
// would only produce 864 pointless UPDATEs a day. The returned bool reports
// whether anything was persisted, which lets the console distinguish "the source
// moved" from "the source is alive and agrees with us".
func RefreshExchangeRate(ctx context.Context) (float64, bool, error) {
	exchangeRateMu.Lock()
	defer exchangeRateMu.Unlock()

	now := time.Now().Unix()
	bustCache := shouldBustExchangeRateCacheLocked(now)
	if bustCache {
		// Record the attempt before the request: a source that is down must not
		// make every following tick bypass the cache again.
		exchangeRateLastBustAt = now
	}

	rate, nextUpdateUnix, err := fetchUSDCNY(ctx, setting.ExchangeRateApiUrl, bustCache)
	if err != nil {
		return 0, false, err
	}
	exchangeRateLastCheckedAt = now
	if nextUpdateUnix > 0 {
		exchangeRateNextUpdateAt = nextUpdateUnix
	}

	rateText := strconv.FormatFloat(rate, 'f', -1, 64)
	priceText := strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	rateChanged := !sameRate(operation_setting.USDExchangeRate, rate)
	priceChanged := setting.PriceFollowsExchangeRate && !sameRate(operation_setting.Price, rate)
	if !rateChanged && !priceChanged {
		logger.LogInfo(ctx, common.LogText("exchange rate unchanged usd_cny=%s price=%s price_follows=%t next_update=%d",
			rateText, priceText, setting.PriceFollowsExchangeRate, exchangeRateNextUpdateAt))
		return rate, false, nil
	}

	if err = model.UpdateOption("USDExchangeRate", rateText); err != nil {
		return 0, false, fmt.Errorf("persist USDExchangeRate: %w", err)
	}
	if setting.PriceFollowsExchangeRate {
		if err = model.UpdateOption("Price", rateText); err != nil {
			return 0, false, fmt.Errorf("persist Price: %w", err)
		}
	}

	// The timestamp is metadata: losing it must not fail a refresh whose rate is
	// already written, so it is reported and otherwise ignored. It means "the
	// persisted value last changed", not "the job last ran" — the loop logs a
	// heartbeat every tick instead of writing a row just to say it is alive.
	if err = model.UpdateOption("ExchangeRateUpdatedAt", strconv.FormatInt(now, 10)); err != nil {
		logger.LogWarn(ctx, common.LogText("persist ExchangeRateUpdatedAt failed error=%q", err.Error()))
	}

	logger.LogInfo(ctx, common.LogText("exchange rate refreshed usd_cny=%s price=%s price_follows=%t next_update=%d",
		rateText, strconv.FormatFloat(operation_setting.Price, 'f', -1, 64), setting.PriceFollowsExchangeRate, exchangeRateNextUpdateAt))
	return rate, true, nil
}

// ExchangeRateLastCheckedAt reports when the last fetch succeeded, unix seconds;
// zero before the first success. It lives in memory so a healthy job does not
// write a database row on every unchanged tick.
func ExchangeRateLastCheckedAt() int64 {
	exchangeRateMu.Lock()
	defer exchangeRateMu.Unlock()
	return exchangeRateLastCheckedAt
}

// ExchangeRateNextUpdateAt reports the next update time advertised by the
// source, unix seconds; zero when the source does not report one.
func ExchangeRateNextUpdateAt() int64 {
	exchangeRateMu.Lock()
	defer exchangeRateMu.Unlock()
	return exchangeRateNextUpdateAt
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
			if _, _, err := RefreshExchangeRate(context.Background()); err != nil {
				logger.LogWarn(context.Background(), common.LogText("exchange rate refresh failed error=%q", err.Error()))
			}
			time.Sleep(time.Duration(minutes) * time.Minute)
		}
	}()
}
