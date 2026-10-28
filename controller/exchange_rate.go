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
func RefreshExchangeRate(c *gin.Context) {
	rate, err := service.RefreshExchangeRate(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"rate":                   rate,
			"price":                  operation_setting.Price,
			"price_follows_exchange": setting.PriceFollowsExchangeRate,
		},
	})
}
