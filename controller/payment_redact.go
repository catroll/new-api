package controller

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// gopay embeds the whole PEM blob into some of its error strings — xpem's
// "私钥解析出错 [-----BEGIN RSA PRIVATE KEY-----…" is the one that bites. Those
// strings reach the application log, which is shipped off the box, so a bad
// paste would print the merchant private key. Everything user- or log-facing
// goes through here first.

var (
	pemBlockRedactRe  = regexp.MustCompile(`(?s)-----BEGIN [^-]+-----.*?-----END [^-]+-----`)
	base64RunRedactRe = regexp.MustCompile(`[A-Za-z0-9+/]{48,}={0,2}`)
	spaceRunRedactRe  = regexp.MustCompile(`\s+`)
)

// redactKeyMaterial removes anything that could be key material and caps the
// remaining text so one error cannot flood the log.
func redactKeyMaterial(msg string) string {
	msg = pemBlockRedactRe.ReplaceAllString(msg, "<key material redacted>")
	msg = base64RunRedactRe.ReplaceAllString(msg, "<base64 redacted>")
	msg = spaceRunRedactRe.ReplaceAllString(msg, " ")
	msg = strings.TrimSpace(msg)
	const limit = 240
	if utf8.RuneCountInString(msg) <= limit {
		return msg
	}
	return string([]rune(msg)[:limit]) + "…"
}
