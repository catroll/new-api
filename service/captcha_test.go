package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestGenerateCaptchaStoresAnswerAndVerifiesOnce(t *testing.T) {
	id, imageDataURL, err := GenerateCaptcha()
	if err != nil {
		t.Fatalf("GenerateCaptcha: %v", err)
	}
	if len(id) < 16 {
		t.Fatalf("challenge id is too short: %q", id)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(imageDataURL, prefix) {
		t.Fatalf("unexpected image data URL: %q", imageDataURL)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageDataURL, prefix))
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("image is not a valid PNG: %v", err)
	}

	answer, ok, err := smsCodeStoreForUse().Get(context.Background(), captchaKey(id))
	if err != nil || !ok {
		t.Fatalf("stored answer missing: ok=%v err=%v", ok, err)
	}
	if len(answer) != captchaCodeLength {
		t.Fatalf("unexpected answer length: %d", len(answer))
	}
	if !VerifyCaptcha(context.Background(), id+":"+strings.ToUpper(answer)) {
		t.Fatal("correct answer was rejected")
	}
	if VerifyCaptcha(context.Background(), id+":"+answer) {
		t.Fatal("challenge was accepted twice")
	}
}

func TestVerifyCaptchaRejectsWrongAndMalformedTokens(t *testing.T) {
	id, _, err := GenerateCaptcha()
	if err != nil {
		t.Fatalf("GenerateCaptcha: %v", err)
	}
	if VerifyCaptcha(context.Background(), "") {
		t.Fatal("empty token accepted")
	}
	if VerifyCaptcha(context.Background(), "not-a-token") {
		t.Fatal("token without separator accepted")
	}
	if VerifyCaptcha(context.Background(), id+":wrong") {
		t.Fatal("wrong answer accepted")
	}
	// The wrong answer consumed the challenge, so a later correct answer must
	// also fail. This is the one-guess bound.
	if VerifyCaptcha(context.Background(), id+":") {
		t.Fatal("empty answer accepted")
	}
}

func TestVerifyCaptchaRejectsExpiredChallenge(t *testing.T) {
	challengeID := "expired-test"
	if err := smsCodeStoreForUse().Set(context.Background(), captchaKey(challengeID), "abcde", -time.Second); err != nil {
		t.Fatalf("seed challenge: %v", err)
	}
	if VerifyCaptcha(context.Background(), challengeID+":abcde") {
		t.Fatal("expired challenge accepted")
	}
}
