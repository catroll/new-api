package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RefreshExchangeRate forces an immediate rate refresh instead of waiting for the
// scheduled tick. The administrator needs it once after wiring the endpoint, and
// it is the only way to tell "the source is unreachable" from "the job is off".
//
// updated_at is the last time the persisted value changed; checked_at is the
// last time the source answered, and next_update_at is when the source says its
// next change is due. All three together tell a healthy-but-flat rate apart from
// a stalled job.
func RefreshExchangeRate(c *gin.Context) {
	rate, changed, err := service.RefreshExchangeRate(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"rate":                   rate,
			"changed":                changed,
			"price":                  operation_setting.Price,
			"price_follows_exchange": setting.PriceFollowsExchangeRate,
			"updated_at":             setting.ExchangeRateUpdatedAt,
			"checked_at":             service.ExchangeRateLastCheckedAt(),
			"next_update_at":         service.ExchangeRateNextUpdateAt(),
		},
	})
}
