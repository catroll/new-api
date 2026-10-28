package setting

// Currency and exchange-rate settings shared by every payment gateway.
//
// The display currency and the payment currency are two different questions. A
// catalogue can be priced in dollars while the buyer still pays renminbi: the
// quota unit stays a dollar of quota, quota_display_type stays "USD", and every
// order is quoted in CNY by multiplying the unit count with Price. That is also
// how a storefront shows "you pay ¥73" next to "$10 of credit".

// PaymentCurrency is the currency the gateways themselves charge. Domestic
// Alipay only accepts CNY, so any other value is refused there until cross-border
// acquiring is implemented; the value exists so the refusal names the real cause.
var PaymentCurrency = "CNY"

// PriceFollowsExchangeRate makes the scheduled rate job overwrite Price together
// with USDExchangeRate. Turn it on for a site that prices in dollars and charges
// in renminbi; a CNY-native site keeps Price fixed (one unit is one yuan) and
// leaves this off, or the job would silently reprice the catalogue every hour.
var PriceFollowsExchangeRate = false

// ExchangeRateRefreshMinutes is the refresh interval of the rate job. Zero
// disables the job; the value is re-read on every tick so an administrator does
// not have to restart the process to change it.
var ExchangeRateRefreshMinutes = 0

// ExchangeRateApiUrl must answer with {"rates":{"CNY":<number>}}. It stays
// configurable because the deployments sit in different regions and a source that
// is reachable from one is not necessarily reachable from the other.
var ExchangeRateApiUrl = "https://open.er-api.com/v6/latest/USD"

// ExchangeRateUpdatedAt is the unix time of the last successful refresh. It is
// written by the job, never by an administrator, and only exists so the console
// can say how old the number it is showing is.
var ExchangeRateUpdatedAt int64 = 0
