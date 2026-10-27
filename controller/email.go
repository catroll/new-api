package controller

import (
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

// maxEmailAddressLength follows the practical RFC 5321 limit; it also keeps a
// mistyped paste from becoming a huge request body.
const maxEmailAddressLength = 254

type testEmailRequest struct {
	Email string `json:"email"`
}

// SendTestEmail lets a root administrator verify the saved email configuration
// from the console or a script. It sends one real message through the selected
// provider and stores nothing, so unlike the account flows it cannot hand out a
// login code or a reset link.
func SendTestEmail(c *gin.Context) {
	var request testEmailRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	receiver, err := parseTestEmailReceiver(request.Email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := requireEmailConfiguration(); err != nil {
		common.ApiError(c, err)
		return
	}

	subject, content := buildTestEmail()
	if err := common.SendEmail(subject, receiver, content); err != nil {
		// The provider's reason (timeout, rejected sender, HTTP status) is what
		// makes the test useful, and only a root administrator reaches here.
		logger.LogError(c.Request.Context(), common.LogText("test email failed email=%s error=%q", common.MaskEmail(receiver), err.Error()))
		common.ApiError(c, err)
		return
	}
	logger.LogInfo(c.Request.Context(), common.LogText("test email sent email=%s", common.MaskEmail(receiver)))
	common.ApiSuccess(c, nil)
}

// parseTestEmailReceiver accepts "a@b.c" as well as "Name <a@b.c>" and returns
// the bare address, because that is what the SMTP envelope and the Resend
// payload both take. Lists and control characters are rejected: this sends one
// test message, it is not a broadcast tool.
func parseTestEmailReceiver(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maxEmailAddressLength || strings.ContainsAny(trimmed, "\r\n\x00;,") {
		return "", common.NewMessage("Invalid email address")
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address == "" {
		return "", common.NewMessage("Invalid email address")
	}
	return parsed.Address, nil
}

// requireEmailConfiguration mirrors the checks SendEmail performs, so saving
// only half of the settings produces "not configured" instead of a dial error.
func requireEmailConfiguration() error {
	common.OptionMapRWMutex.RLock()
	provider := common.OptionMap["EmailProvider"]
	resendKey := common.OptionMap["ResendApiKey"]
	resendFrom := common.OptionMap["ResendFrom"]
	common.OptionMapRWMutex.RUnlock()

	switch provider {
	case "resend":
		return common.ValidateResendConfiguration(resendKey, resendFrom)
	case "", "smtp": // Preserve existing installations without a selector.
		if strings.TrimSpace(common.SMTPServer) == "" {
			return common.NewMessage("SMTP server is not configured")
		}
		if common.SMTPFrom == "" && common.SMTPAccount == "" {
			return common.NewMessage("SMTP sender address is not configured")
		}
		return nil
	default:
		return common.NewMessage("Email sending method must be SMTP or Resend")
	}
}

func buildTestEmail() (string, string) {
	subject := fmt.Sprintf("[%s] Test email", common.SystemName)
	content := fmt.Sprintf(`<div style="font-family:Arial,Helvetica,sans-serif;font-size:14px;line-height:1.7">
  <p>This is a test email sent from the %s console.</p>
  <p>Receiving it means the saved email configuration works.</p>
  <p style="color:#888888">Sent at %s.</p>
</div>`, html.EscapeString(common.SystemName), time.Now().Format("2006-01-02 15:04:05 MST"))
	return subject, content
}
