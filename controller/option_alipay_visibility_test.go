package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The console reads every gateway back through GET /api/option/. The private key
// must stay write-only, while the platform public key is not a secret and has to
// be visible: an operator compares it with the value in Alipay's console.
func TestAlipayOptionVisibility(t *testing.T) {
	keys := map[string]string{
		"AlipayAppId":      "2021000000000001",
		"AlipayPrivateKey": "PRIVATE-KEY-MUST-NOT-LEAVE-THE-SERVER",
		"AlipayPublicKey":  "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A",
		"AlipaySandbox":    "true",
		"AlipayMinTopUp":   "5",
	}
	// OptionMap is nil unless a test set it up; own it for the duration.
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = keys
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	GetOptions(context)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()

	assert.NotContains(t, body, "PRIVATE-KEY-MUST-NOT-LEAVE-THE-SERVER", "the private key must never be echoed")
	assert.NotContains(t, body, `"key":"AlipayPrivateKey"`)
	assert.Contains(t, body, `"key":"AlipayAppId"`)
	assert.Contains(t, body, `"key":"AlipayPublicKey"`)
	assert.Contains(t, body, `"key":"AlipaySandbox"`)
	assert.Contains(t, body, `"key":"AlipayMinTopUp"`)
}
