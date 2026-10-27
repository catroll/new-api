package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callSendTestEmail(t *testing.T, body string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/option/email/test", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	SendTestEmail(context)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func TestParseTestEmailReceiverKeepsTheAddressOnly(t *testing.T) {
	receiver, err := parseTestEmailReceiver("  New API <admin@example.com>  ")
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", receiver)

	for _, invalid := range []string{
		"",
		"not-an-address",
		"a@example.com;b@example.com",
		"a@example.com\r\nBcc: other@example.com",
		strings.Repeat("a", 250) + "@example.com",
	} {
		_, err := parseTestEmailReceiver(invalid)
		assert.Error(t, err, "expected %q to be rejected", invalid)
	}
}

func TestSendTestEmailRejectsMalformedAddress(t *testing.T) {
	configureEmailDelivery(t, map[string]string{"EmailProvider": "smtp"})

	payload := callSendTestEmail(t, `{"email":"not-an-address"}`)

	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "Invalid email address", payload["message"])
}

func TestSendTestEmailRequiresConfiguredProvider(t *testing.T) {
	configureEmailDelivery(t, map[string]string{"EmailProvider": "smtp"})

	payload := callSendTestEmail(t, `{"email":"admin@example.com"}`)

	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "SMTP server is not configured", payload["message"])
}

func TestSendTestEmailDeliversThroughSelectedProvider(t *testing.T) {
	configureEmailDelivery(t, map[string]string{
		"EmailProvider": "resend",
		"ResendApiKey":  "test-key",
		"ResendFrom":    "New API <sender@example.com>",
	})
	var sent struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}
	http.DefaultTransport = emailHTTPTransport(func(request *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(request.Body).Decode(&sent))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"test-email-id"}`)),
			Request:    request,
		}, nil
	})

	payload := callSendTestEmail(t, `{"email":"admin@example.com"}`)

	assert.Equal(t, true, payload["success"], payload["message"])
	assert.Equal(t, []string{"admin@example.com"}, sent.To)
	assert.Contains(t, sent.Subject, "Test email")
	assert.Contains(t, sent.HTML, "test email")
}

func TestSendTestEmailReportsProviderRejection(t *testing.T) {
	configureEmailDelivery(t, map[string]string{
		"EmailProvider": "resend",
		"ResendApiKey":  "test-key",
		"ResendFrom":    "New API <sender@example.com>",
	})
	http.DefaultTransport = emailHTTPTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnprocessableEntity,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"name":"validation_error","message":"domain is not verified"}`)),
			Request:    request,
		}, nil
	})

	payload := callSendTestEmail(t, `{"email":"admin@example.com"}`)

	assert.Equal(t, false, payload["success"])
	assert.Equal(t, "Resend email delivery failed (HTTP 422)", payload["message"])
}
