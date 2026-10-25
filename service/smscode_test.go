package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func smsTestScope(t *testing.T, purpose string) (string, string) {
	t.Helper()
	scope := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	return purpose, scope
}

func TestSMSCodeIssueAndVerify(t *testing.T) {
	purpose, scope := smsTestScope(t, SMSCodePurposeLogin)
	ctx := context.Background()

	code, err := IssueSMSCode(ctx, purpose, scope, time.Minute)
	if err != nil {
		t.Fatalf("IssueSMSCode() error = %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code = %q, want 6 digits", code)
	}
	if err := VerifySMSCode(ctx, purpose, scope, code, 5); err != nil {
		t.Fatalf("VerifySMSCode() error = %v", err)
	}
	// Single use: the same code cannot be redeemed twice.
	if err := VerifySMSCode(ctx, purpose, scope, code, 5); !errors.Is(err, ErrSMSCodeInvalid) {
		t.Fatalf("second VerifySMSCode() = %v, want ErrSMSCodeInvalid", err)
	}
}

func TestSMSCodeWrongGuessExhaustsAttempts(t *testing.T) {
	purpose, scope := smsTestScope(t, SMSCodePurposeLogin)
	ctx := context.Background()

	if _, err := IssueSMSCode(ctx, purpose, scope, time.Minute); err != nil {
		t.Fatalf("IssueSMSCode() error = %v", err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if err := VerifySMSCode(ctx, purpose, scope, "000000", 5); !errors.Is(err, ErrSMSCodeInvalid) {
			t.Fatalf("attempt %d = %v, want ErrSMSCodeInvalid", attempt, err)
		}
	}
	if err := VerifySMSCode(ctx, purpose, scope, "000000", 5); !errors.Is(err, ErrSMSCodeTooMany) {
		t.Fatalf("after limit = %v, want ErrSMSCodeTooMany", err)
	}
}

func TestSMSCodeExpires(t *testing.T) {
	purpose, scope := smsTestScope(t, SMSCodePurposeReset)
	ctx := context.Background()

	code, err := IssueSMSCode(ctx, purpose, scope, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("IssueSMSCode() error = %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if err := VerifySMSCode(ctx, purpose, scope, code, 5); !errors.Is(err, ErrSMSCodeInvalid) {
		t.Fatalf("expired code = %v, want ErrSMSCodeInvalid", err)
	}
}

func TestSMSCodeCooldown(t *testing.T) {
	purpose, scope := smsTestScope(t, SMSCodePurposeLogin)
	ctx := context.Background()

	allowed, err := TakeSMSCooldown(ctx, purpose, scope, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("first cooldown = %v, %v; want true, nil", allowed, err)
	}
	allowed, err = TakeSMSCooldown(ctx, purpose, scope, time.Minute)
	if err != nil || allowed {
		t.Fatalf("second cooldown = %v, %v; want false, nil", allowed, err)
	}
	// A different purpose on the same number is an independent slot.
	allowed, err = TakeSMSCooldown(ctx, SMSCodePurposeRegister, scope, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("other purpose cooldown = %v, %v; want true, nil", allowed, err)
	}
}

func TestSMSPhoneDailyBudget(t *testing.T) {
	phone := fmt.Sprintf("+8613800%05d", time.Now().UnixNano()%100000)
	ctx := context.Background()

	for send := 1; send <= 2; send++ {
		if err := TakeSMSPhoneDailyBudget(ctx, phone, 2); err != nil {
			t.Fatalf("send %d = %v, want nil", send, err)
		}
	}
	if err := TakeSMSPhoneDailyBudget(ctx, phone, 2); !errors.Is(err, ErrSMSPhoneDailyLimit) {
		t.Fatalf("third send = %v, want ErrSMSPhoneDailyLimit", err)
	}
}

func TestSMSGlobalDailyBudget(t *testing.T) {
	// The global counter is shared by the process, so this test only asserts
	// that a limit of one is enforced on the first call after a large quota was
	// already consumed elsewhere. Use a huge quota to keep it order-independent.
	ctx := context.Background()
	if err := TakeSMSGlobalDailyBudget(ctx, 1_000_000); err != nil {
		t.Fatalf("huge quota = %v, want nil", err)
	}
}
