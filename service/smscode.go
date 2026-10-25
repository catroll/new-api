package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/QuantumNous/new-api/common"
)

const (
	SMSCodePurposeLogin    = "login"
	SMSCodePurposeRegister = "register"
	SMSCodePurposeBind     = "bind"
	SMSCodePurposeReset    = "reset"

	smsKeyPrefix        = "sms"
	smsAttemptsTTL      = 10 * time.Minute
	smsCodeLength       = 6
	smsDailyBudgetGrace = time.Hour
)

var (
	ErrSMSCodeInvalid      = errors.New("Verification code is incorrect or has expired")
	ErrSMSCodeTooMany      = errors.New("Too many verification attempts, request a new code")
	ErrSMSSendTooSoon      = errors.New("Please wait before requesting another verification code")
	ErrSMSPhoneDailyLimit  = errors.New("Daily SMS limit for this number reached")
	ErrSMSDailyQuota       = errors.New("Daily SMS quota reached")
	ErrSMSStoreUnavailable = errors.New("SMS verification is temporarily unavailable")
)

// smsCodeStore is the small slice of key/value behaviour the code flow needs.
// It is an interface so the whole flow can be tested without Redis.
type smsCodeStore interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, bool, error)
	Del(ctx context.Context, key string) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
}

type redisSMSCodeStore struct{}

func (redisSMSCodeStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return common.RDB.Set(ctx, key, value, ttl).Err()
}

func (redisSMSCodeStore) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := common.RDB.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (redisSMSCodeStore) Del(ctx context.Context, key string) error {
	return common.RDB.Del(ctx, key).Err()
}

func (redisSMSCodeStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := common.RDB.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return count.Val(), nil
}

func (redisSMSCodeStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return common.RDB.SetNX(ctx, key, value, ttl).Result()
}

type memorySMSEntry struct {
	value     string
	expiresAt time.Time
}

type memorySMSCodeStore struct {
	mutex  sync.Mutex
	values map[string]memorySMSEntry
}

func (m *memorySMSCodeStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.values == nil {
		m.values = make(map[string]memorySMSEntry)
	}
	m.values[key] = memorySMSEntry{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (m *memorySMSCodeStore) Get(_ context.Context, key string) (string, bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	entry, ok := m.values[key]
	if !ok {
		return "", false, nil
	}
	if time.Now().After(entry.expiresAt) {
		delete(m.values, key)
		return "", false, nil
	}
	return entry.value, true, nil
}

func (m *memorySMSCodeStore) Del(_ context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.values, key)
	return nil
}

func (m *memorySMSCodeStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.values == nil {
		m.values = make(map[string]memorySMSEntry)
	}
	entry, ok := m.values[key]
	if !ok || time.Now().After(entry.expiresAt) {
		entry = memorySMSEntry{value: "0", expiresAt: time.Now().Add(ttl)}
	}
	var current int64
	_, _ = fmt.Sscanf(entry.value, "%d", &current)
	current++
	entry.value = fmt.Sprintf("%d", current)
	m.values[key] = entry
	return current, nil
}

func (m *memorySMSCodeStore) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.values == nil {
		m.values = make(map[string]memorySMSEntry)
	}
	if entry, ok := m.values[key]; ok && time.Now().Before(entry.expiresAt) {
		return false, nil
	}
	m.values[key] = memorySMSEntry{value: value, expiresAt: time.Now().Add(ttl)}
	return true, nil
}

var smsMemoryStore = &memorySMSCodeStore{values: make(map[string]memorySMSEntry)}

func smsCodeStoreForUse() smsCodeStore {
	if common.RedisEnabled && common.RDB != nil {
		return redisSMSCodeStore{}
	}
	return smsMemoryStore
}

func smsCodeKey(purpose, scope string) string {
	return fmt.Sprintf("%s:code:%s:%s", smsKeyPrefix, purpose, scope)
}

func smsAttemptKey(purpose, scope string) string {
	return fmt.Sprintf("%s:att:%s:%s", smsKeyPrefix, purpose, scope)
}

func smsCooldownKey(purpose, scope string) string {
	return fmt.Sprintf("%s:cd:%s:%s", smsKeyPrefix, purpose, scope)
}

func smsPhoneDailyKey(phone string) string {
	return fmt.Sprintf("%s:phone:%s:%s", smsKeyPrefix, time.Now().Format("20060102"), phone)
}

func smsGlobalDailyKey() string {
	return fmt.Sprintf("%s:day:%s", smsKeyPrefix, time.Now().Format("20060102"))
}

// SMSCodeScopeForBind binds a code to one account and one number, so a code
// issued for account A cannot be redeemed by account B.
func SMSCodeScopeForBind(userID int, phone string) string {
	return fmt.Sprintf("user:%d:%s", userID, phone)
}

// IssueSMSCode generates a numeric code, stores it for the given TTL and resets
// the attempt counter. The code itself is returned so the caller can hand it to
// the provider.
func IssueSMSCode(ctx context.Context, purpose, scope string, ttl time.Duration) (string, error) {
	code, err := generateSMSCode(smsCodeLength)
	if err != nil {
		return "", err
	}
	store := smsCodeStoreForUse()
	if err := store.Set(ctx, smsCodeKey(purpose, scope), code, ttl); err != nil {
		return "", ErrSMSStoreUnavailable
	}
	if err := store.Del(ctx, smsAttemptKey(purpose, scope)); err != nil {
		return "", ErrSMSStoreUnavailable
	}
	return code, nil
}

// VerifySMSCode consumes one attempt and, on success, the code itself.
func VerifySMSCode(ctx context.Context, purpose, scope, code string, maxAttempts int) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrSMSCodeInvalid
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	store := smsCodeStoreForUse()

	attempts, err := store.Incr(ctx, smsAttemptKey(purpose, scope), smsAttemptsTTL)
	if err != nil {
		return ErrSMSStoreUnavailable
	}
	if int(attempts) > maxAttempts {
		return ErrSMSCodeTooMany
	}

	stored, ok, err := store.Get(ctx, smsCodeKey(purpose, scope))
	if err != nil {
		return ErrSMSStoreUnavailable
	}
	if !ok {
		return ErrSMSCodeInvalid
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(code)) != 1 {
		return ErrSMSCodeInvalid
	}

	if err := store.Del(ctx, smsCodeKey(purpose, scope)); err != nil {
		return ErrSMSStoreUnavailable
	}
	_ = store.Del(ctx, smsAttemptKey(purpose, scope))
	return nil
}

// TakeSMSCooldown reserves the next send slot for this purpose/number. It
// returns false when the cooldown is still running.
func TakeSMSCooldown(ctx context.Context, purpose, scope string, cooldown time.Duration) (bool, error) {
	if cooldown <= 0 {
		return true, nil
	}
	allowed, err := smsCodeStoreForUse().SetNX(ctx, smsCooldownKey(purpose, scope), "1", cooldown)
	if err != nil {
		return false, ErrSMSStoreUnavailable
	}
	return allowed, nil
}

// TakeSMSPhoneDailyBudget counts one send against the number's daily allowance.
func TakeSMSPhoneDailyBudget(ctx context.Context, phone string, limit int) error {
	if limit <= 0 {
		return nil
	}
	store := smsCodeStoreForUse()
	count, err := store.Incr(ctx, smsPhoneDailyKey(phone), smsDailyTTL())
	if err != nil {
		return ErrSMSStoreUnavailable
	}
	if int(count) > limit {
		return ErrSMSPhoneDailyLimit
	}
	return nil
}

// TakeSMSGlobalDailyBudget counts one send against the site-wide daily quota.
// A zero limit means "no cap".
func TakeSMSGlobalDailyBudget(ctx context.Context, limit int) error {
	if limit <= 0 {
		return nil
	}
	store := smsCodeStoreForUse()
	count, err := store.Incr(ctx, smsGlobalDailyKey(), smsDailyTTL())
	if err != nil {
		return ErrSMSStoreUnavailable
	}
	if int(count) > limit {
		return ErrSMSDailyQuota
	}
	return nil
}

func smsDailyTTL() time.Duration {
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	return time.Until(endOfDay) + smsDailyBudgetGrace
}

// GenerateSMSCode returns a random numeric code of the documented length.
func GenerateSMSCode() (string, error) {
	return generateSMSCode(smsCodeLength)
}

func generateSMSCode(length int) (string, error) {
	limit := big.NewInt(1)
	for range length {
		limit.Mul(limit, big.NewInt(10))
	}
	number, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", length, number), nil
}
