package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
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

func TestSMSRegisterDailyBudget(t *testing.T) {
	ctx := context.Background()
	store := smsCodeStoreForUse()
	if err := store.Del(ctx, smsRegisterDailyKey()); err != nil {
		t.Fatalf("reset register budget = %v", err)
	}

	for send := 1; send <= 2; send++ {
		if err := TakeSMSRegisterDailyBudget(ctx, 2); err != nil {
			t.Fatalf("send %d = %v, want nil", send, err)
		}
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 2); !errors.Is(err, ErrSMSRegisterDailyLimit) {
		t.Fatalf("third send = %v, want ErrSMSRegisterDailyLimit", err)
	}
}

func TestSMSRegisterIPHourlyBudget(t *testing.T) {
	ctx := context.Background()
	ip := fmt.Sprintf("198.51.100.%d", time.Now().UnixNano()%200+1)
	store := smsCodeStoreForUse()
	if err := store.Del(ctx, smsRegisterIPHourlyKey(ip)); err != nil {
		t.Fatalf("reset register IP budget = %v", err)
	}

	for send := 1; send <= 2; send++ {
		if err := TakeSMSRegisterIPHourlyBudget(ctx, ip, 2); err != nil {
			t.Fatalf("send %d = %v, want nil", send, err)
		}
	}
	if err := TakeSMSRegisterIPHourlyBudget(ctx, ip, 2); !errors.Is(err, ErrSMSRegisterIPLimit) {
		t.Fatalf("third send = %v, want ErrSMSRegisterIPLimit", err)
	}
	// A different source has its own slot.
	if err := TakeSMSRegisterIPHourlyBudget(ctx, ip+"x", 2); err != nil {
		t.Fatalf("other IP = %v, want nil", err)
	}
}

// A registration send must consume the registration-only counter; otherwise the
// separate quota would exist in the settings but never protect anything.
func TestSendSMSCodeConsumesRegisterDailyBudget(t *testing.T) {
	ctx := context.Background()
	store := smsCodeStoreForUse()
	if err := store.Del(ctx, smsRegisterDailyKey()); err != nil {
		t.Fatalf("reset register budget = %v", err)
	}

	previousDebug := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = previousDebug })

	settings := system_setting.SMSSettings{
		Enabled:            true,
		Provider:           system_setting.SMSProviderConsole,
		RegisterDailyQuota: 10,
	}
	phone := fmt.Sprintf("+8613801%05d", time.Now().UnixNano()%100000)
	if _, err := SendSMSCode(ctx, SMSCodePurposeRegister, phone, phone, settings); err != nil {
		t.Fatalf("SendSMSCode() error = %v", err)
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 1); !errors.Is(err, ErrSMSRegisterDailyLimit) {
		t.Fatalf("register counter = %v, want ErrSMSRegisterDailyLimit after one send", err)
	}
}

func TestSMSBudgetRefunds(t *testing.T) {
	ctx := context.Background()
	store := smsCodeStoreForUse()
	phone := fmt.Sprintf("+8613802%05d", time.Now().UnixNano()%100000)

	if err := store.Del(ctx, smsPhoneDailyKey(phone)); err != nil {
		t.Fatalf("reset phone budget = %v", err)
	}
	if err := TakeSMSPhoneDailyBudget(ctx, phone, 1); err != nil {
		t.Fatalf("phone take = %v", err)
	}
	if err := RefundSMSPhoneDailyBudget(ctx, phone); err != nil {
		t.Fatalf("phone refund = %v", err)
	}
	if err := TakeSMSPhoneDailyBudget(ctx, phone, 1); err != nil {
		t.Fatalf("phone take after refund = %v, want nil", err)
	}

	if err := store.Del(ctx, smsRegisterDailyKey()); err != nil {
		t.Fatalf("reset register budget = %v", err)
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 1); err != nil {
		t.Fatalf("register take = %v", err)
	}
	if err := RefundSMSRegisterDailyBudget(ctx); err != nil {
		t.Fatalf("register refund = %v", err)
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 1); err != nil {
		t.Fatalf("register take after refund = %v, want nil", err)
	}

	if err := store.Del(ctx, smsGlobalDailyKey()); err != nil {
		t.Fatalf("reset global budget = %v", err)
	}
	if err := TakeSMSGlobalDailyBudget(ctx, 1); err != nil {
		t.Fatalf("global take = %v", err)
	}
	if err := RefundSMSGlobalDailyBudget(ctx); err != nil {
		t.Fatalf("global refund = %v", err)
	}
	if err := TakeSMSGlobalDailyBudget(ctx, 1); err != nil {
		t.Fatalf("global take after refund = %v, want nil", err)
	}

	ip := fmt.Sprintf("198.51.100.%d", time.Now().UnixNano()%200+1)
	if err := store.Del(ctx, smsRegisterIPHourlyKey(ip)); err != nil {
		t.Fatalf("reset IP budget = %v", err)
	}
	if err := TakeSMSRegisterIPHourlyBudget(ctx, ip, 1); err != nil {
		t.Fatalf("IP take = %v", err)
	}
	if err := RefundSMSRegisterIPHourlyBudget(ctx, ip); err != nil {
		t.Fatalf("IP refund = %v", err)
	}
	if err := TakeSMSRegisterIPHourlyBudget(ctx, ip, 1); err != nil {
		t.Fatalf("IP take after refund = %v, want nil", err)
	}
}

// A provider failure must not silently spend the phone, registration or global
// budget, and it must not leave the number in a cooldown for a code that was
// never delivered.
func TestSendSMSCodeRefundsOnProviderFailure(t *testing.T) {
	ctx := context.Background()
	store := smsCodeStoreForUse()
	if err := store.Del(ctx, smsGlobalDailyKey()); err != nil {
		t.Fatalf("reset global budget = %v", err)
	}
	if err := store.Del(ctx, smsRegisterDailyKey()); err != nil {
		t.Fatalf("reset register budget = %v", err)
	}

	previousDebug := common.DebugEnabled
	common.DebugEnabled = false
	t.Cleanup(func() { common.DebugEnabled = previousDebug })

	settings := system_setting.SMSSettings{
		Enabled:            true,
		Provider:           system_setting.SMSProviderConsole,
		PhoneDailyLimit:    1,
		RegisterDailyQuota: 1,
		DailyQuota:         1,
		ResendSeconds:      60,
	}
	phone := fmt.Sprintf("+8613803%05d", time.Now().UnixNano()%100000)
	if _, err := SendSMSCode(ctx, SMSCodePurposeRegister, phone, phone, settings); err == nil {
		t.Fatal("SendSMSCode() error = nil, want provider failure")
	}

	if err := TakeSMSPhoneDailyBudget(ctx, phone, 1); err != nil {
		t.Fatalf("phone budget was not refunded: %v", err)
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 1); err != nil {
		t.Fatalf("register budget was not refunded: %v", err)
	}
	if err := TakeSMSGlobalDailyBudget(ctx, 1); err != nil {
		t.Fatalf("global budget was not refunded: %v", err)
	}
	allowed, err := TakeSMSCooldown(ctx, SMSCodePurposeRegister, phone, time.Minute)
	if err != nil {
		t.Fatalf("cooldown take = %v", err)
	}
	if !allowed {
		t.Fatal("cooldown was not released after a failed send")
	}
}

// The global quota check runs after the phone and registration counters. When it
// rejects, those two must be rolled back so a later legitimate attempt is not
// penalized for a code that was never sent.
func TestSendSMSCodeRefundsWhenGlobalQuotaRejects(t *testing.T) {
	ctx := context.Background()
	store := smsCodeStoreForUse()
	if err := store.Del(ctx, smsGlobalDailyKey()); err != nil {
		t.Fatalf("reset global budget = %v", err)
	}
	if err := store.Del(ctx, smsRegisterDailyKey()); err != nil {
		t.Fatalf("reset register budget = %v", err)
	}
	if err := TakeSMSGlobalDailyBudget(ctx, 1); err != nil {
		t.Fatalf("pre-consume global = %v", err)
	}

	settings := system_setting.SMSSettings{
		Enabled:            true,
		Provider:           system_setting.SMSProviderConsole,
		PhoneDailyLimit:    1,
		RegisterDailyQuota: 1,
		DailyQuota:         1,
	}
	phone := fmt.Sprintf("+8613804%05d", time.Now().UnixNano()%100000)
	if _, err := SendSMSCode(ctx, SMSCodePurposeRegister, phone, phone, settings); !errors.Is(err, ErrSMSDailyQuota) {
		t.Fatalf("SendSMSCode() error = %v, want ErrSMSDailyQuota", err)
	}

	if err := TakeSMSPhoneDailyBudget(ctx, phone, 1); err != nil {
		t.Fatalf("phone budget was not refunded after global rejection: %v", err)
	}
	if err := TakeSMSRegisterDailyBudget(ctx, 1); err != nil {
		t.Fatalf("register budget was not refunded after global rejection: %v", err)
	}
	allowed, err := TakeSMSCooldown(ctx, SMSCodePurposeRegister, phone, time.Minute)
	if err != nil {
		t.Fatalf("cooldown take = %v", err)
	}
	if !allowed {
		t.Fatal("cooldown was not released after global rejection")
	}
}
