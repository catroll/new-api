package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// TrustedDeviceCookieName is the HttpOnly cookie that remembers a device on
// which a phone number already completed SMS login. It only ever makes the
// image challenge optional for that number; it never authenticates a request.
const TrustedDeviceCookieName = "new_api_trusted_device"

// TrustedDeviceContextKey is set by the middleware when the current request
// carries a valid trusted-device cookie for the requested phone.
const TrustedDeviceContextKey = "trusted_device_ok"

const (
	// TrustedDeviceTTL bounds how long one device may skip the challenge. It is
	// deliberately shorter than a year so a forgotten device eventually falls
	// back to the normal path.
	TrustedDeviceTTL = 90 * 24 * time.Hour
	// The per-user index outlives its members so a device record can never be
	// reachable through a live index after the device itself expired.
	trustedDeviceIndexTTL   = TrustedDeviceTTL + 24*time.Hour
	trustedDeviceMaxPerUser = 5
	trustedDeviceKeyPrefix  = "trusted"
	trustedDeviceSecretLen  = 64
)

var (
	// ErrTrustedDeviceLimit means the account already trusts the maximum number
	// of devices; issuing more would make the list unusable.
	ErrTrustedDeviceLimit = errors.New("too many trusted devices")
	// ErrTrustedDeviceNotFound covers both "no such device" and "device belongs
	// to another account" so callers cannot probe other users' device IDs.
	ErrTrustedDeviceNotFound = errors.New("trusted device not found")
	// ErrTrustedDeviceTTL guards the invariant that this feature never writes a
	// key without an expiry.
	ErrTrustedDeviceTTL = errors.New("trusted device TTL must be positive")
)

// TrustedDeviceView is the shape returned to the settings page.
type TrustedDeviceView struct {
	ID        string `json:"id"`
	Current   bool   `json:"current"`
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type trustedDeviceRecord struct {
	UserID      int    `json:"uid"`
	PhoneHash   string `json:"phone"`
	SecretHash  string `json:"secret"`
	AuthVersion int64  `json:"auth_version"`
	UserAgent   string `json:"user_agent,omitempty"`
	IP          string `json:"ip,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// trustedDeviceStore is the key/value surface the feature needs. Both keys it
// touches must always be written with a TTL; the implementations enforce that
// instead of trusting every caller.
type trustedDeviceStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	SetAdd(ctx context.Context, key, member string, ttl time.Duration) error
	SetRemove(ctx context.Context, key, member string) error
	SetMembers(ctx context.Context, key string) ([]string, error)
}

type redisTrustedDeviceStore struct{}

func (redisTrustedDeviceStore) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := common.RDB.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (redisTrustedDeviceStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrTrustedDeviceTTL
	}
	return common.RDB.Set(ctx, key, value, ttl).Err()
}

func (redisTrustedDeviceStore) Del(ctx context.Context, key string) error {
	return common.RDB.Del(ctx, key).Err()
}

func (redisTrustedDeviceStore) SetAdd(ctx context.Context, key, member string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrTrustedDeviceTTL
	}
	// SADD and EXPIRE travel in one transaction so the index never exists
	// without an expiry, even for a moment.
	pipe := common.RDB.TxPipeline()
	pipe.SAdd(ctx, key, member)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (redisTrustedDeviceStore) SetRemove(ctx context.Context, key, member string) error {
	return common.RDB.SRem(ctx, key, member).Err()
}

func (redisTrustedDeviceStore) SetMembers(ctx context.Context, key string) ([]string, error) {
	members, err := common.RDB.SMembers(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return members, err
}

// trustedDeviceClock is replaceable so tests can expire entries without sleeping.
var trustedDeviceClock = time.Now

type trustedDeviceValue struct {
	value     string
	expiresAt time.Time
}

type trustedDeviceSet struct {
	members   map[string]struct{}
	expiresAt time.Time
}

type memoryTrustedDeviceStore struct {
	mu     sync.Mutex
	values map[string]trustedDeviceValue
	sets   map[string]trustedDeviceSet
}

func newMemoryTrustedDeviceStore() *memoryTrustedDeviceStore {
	return &memoryTrustedDeviceStore{
		values: make(map[string]trustedDeviceValue),
		sets:   make(map[string]trustedDeviceSet),
	}
}

func (s *memoryTrustedDeviceStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", false, nil
	}
	if !value.expiresAt.After(trustedDeviceClock()) {
		delete(s.values, key)
		return "", false, nil
	}
	return value.value, true, nil
}

func (s *memoryTrustedDeviceStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrTrustedDeviceTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = trustedDeviceValue{value: value, expiresAt: trustedDeviceClock().Add(ttl)}
	return nil
}

func (s *memoryTrustedDeviceStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	delete(s.sets, key)
	return nil
}

func (s *memoryTrustedDeviceStore) SetAdd(_ context.Context, key, member string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrTrustedDeviceTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.sets[key]
	if !ok || !set.expiresAt.After(trustedDeviceClock()) {
		set = trustedDeviceSet{members: make(map[string]struct{})}
	}
	set.members[member] = struct{}{}
	set.expiresAt = trustedDeviceClock().Add(ttl)
	s.sets[key] = set
	return nil
}

func (s *memoryTrustedDeviceStore) SetRemove(_ context.Context, key, member string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.sets[key]
	if !ok {
		return nil
	}
	delete(set.members, member)
	if len(set.members) == 0 {
		delete(s.sets, key)
		return nil
	}
	s.sets[key] = set
	return nil
}

func (s *memoryTrustedDeviceStore) SetMembers(_ context.Context, key string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.sets[key]
	if !ok {
		return nil, nil
	}
	if !set.expiresAt.After(trustedDeviceClock()) {
		delete(s.sets, key)
		return nil, nil
	}
	members := make([]string, 0, len(set.members))
	for member := range set.members {
		members = append(members, member)
	}
	return members, nil
}

var (
	trustedDeviceMemoryStore = newMemoryTrustedDeviceStore()
	// trustedDeviceStoreOverride lets tests swap the backing store.
	trustedDeviceStoreOverride trustedDeviceStore
	// trustedDeviceUserLookup is replaceable so validation tests do not need a database.
	trustedDeviceUserLookup = model.GetUserById
)

func trustedDeviceStoreInstance() trustedDeviceStore {
	if trustedDeviceStoreOverride != nil {
		return trustedDeviceStoreOverride
	}
	if common.RedisEnabled && common.RDB != nil {
		return redisTrustedDeviceStore{}
	}
	return trustedDeviceMemoryStore
}

func trustedDeviceKey(deviceID string) string {
	return trustedDeviceKeyPrefix + ":dev:" + deviceID
}

func trustedUserIndexKey(userID int) string {
	return fmt.Sprintf("%s:user:%d", trustedDeviceKeyPrefix, userID)
}

func trustedDevicePhoneHash(normalizedPhone string) string {
	return common.GenerateHMACWithKey(authSigningKey("trusted-device-phone"), normalizedPhone)
}

func trustedDeviceSecretHash(secret string) string {
	return common.GenerateHMACWithKey(authSigningKey("trusted-device-secret"), secret)
}

// splitTrustedDeviceToken parses "<device-uuid>.<secret>". The UUID check keeps
// arbitrary cookie values from becoming Redis key material.
func splitTrustedDeviceToken(raw string) (string, string, bool) {
	id, secret, ok := strings.Cut(strings.TrimSpace(raw), ".")
	if !ok || id == "" || secret == "" || strings.Contains(secret, ".") {
		return "", "", false
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", "", false
	}
	return id, secret, true
}

func constantTimeStringEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// pruneTrustedDeviceIndex drops index members whose device record already
// expired, so an old index cannot block new devices or leak stale entries.
func pruneTrustedDeviceIndex(ctx context.Context, store trustedDeviceStore, indexKey string) error {
	members, err := store.SetMembers(ctx, indexKey)
	if err != nil {
		return err
	}
	for _, member := range members {
		_, found, err := store.Get(ctx, trustedDeviceKey(member))
		if err != nil {
			return err
		}
		if !found {
			if err := store.SetRemove(ctx, indexKey, member); err != nil {
				return err
			}
		}
	}
	return nil
}

// IssueTrustedDevice records the device and writes the cookie. Callers must
// invoke it only after the SMS code for phone was verified, so the trust is
// anchored in proof of phone control rather than in a mere request.
func IssueTrustedDevice(c *gin.Context, user *model.User, phone string) error {
	if c == nil || c.Request == nil || user == nil || user.Id <= 0 || user.AuthVersion <= 0 {
		return ErrTrustedDeviceNotFound
	}
	normalizedPhone, err := NormalizePhoneInput(phone)
	if err != nil {
		return err
	}
	ctx := c.Request.Context()
	store := trustedDeviceStoreInstance()
	indexKey := trustedUserIndexKey(user.Id)
	if err := pruneTrustedDeviceIndex(ctx, store, indexKey); err != nil {
		return err
	}
	// A browser that already holds a valid device for this account and number
	// rotates its secret instead of spending another slot. Otherwise the sixth
	// SMS login would silently stop extending trust.
	if deviceID, secret, ok := currentTrustedDeviceToken(c); ok {
		reused, err := rotateTrustedDevice(ctx, store, indexKey, c, user, normalizedPhone, deviceID, secret)
		if err != nil {
			return err
		}
		if reused {
			return nil
		}
	}
	members, err := store.SetMembers(ctx, indexKey)
	if err != nil {
		return err
	}
	if len(members) >= trustedDeviceMaxPerUser {
		return ErrTrustedDeviceLimit
	}
	secret, err := common.GenerateRandomCharsKey(trustedDeviceSecretLen)
	if err != nil {
		return err
	}
	deviceID := uuid.NewString()
	record := trustedDeviceRecord{
		UserID:      user.Id,
		PhoneHash:   trustedDevicePhoneHash(normalizedPhone),
		SecretHash:  trustedDeviceSecretHash(secret),
		AuthVersion: user.AuthVersion,
		UserAgent:   truncateAuthMetadata(c.Request.UserAgent(), 512),
		IP:          truncateAuthMetadata(c.ClientIP(), 64),
		CreatedAt:   time.Now().Unix(),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := store.Set(ctx, trustedDeviceKey(deviceID), string(payload), TrustedDeviceTTL); err != nil {
		return err
	}
	if err := store.SetAdd(ctx, indexKey, deviceID, trustedDeviceIndexTTL); err != nil {
		_ = store.Del(ctx, trustedDeviceKey(deviceID))
		return err
	}
	WriteTrustedDeviceCookie(c, deviceID+"."+secret)
	return nil
}

// rotateTrustedDevice refreshes an existing device after a fresh SMS proof.
// The old secret must still match, so knowing only a device ID is not enough
// to take over the trust.
func rotateTrustedDevice(ctx context.Context, store trustedDeviceStore, indexKey string, c *gin.Context, user *model.User, normalizedPhone, deviceID, secret string) (bool, error) {
	value, found, err := store.Get(ctx, trustedDeviceKey(deviceID))
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	var record trustedDeviceRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return false, nil
	}
	if record.UserID != user.Id || record.AuthVersion != user.AuthVersion {
		return false, nil
	}
	if !constantTimeStringEqual(record.PhoneHash, trustedDevicePhoneHash(normalizedPhone)) {
		return false, nil
	}
	if !constantTimeStringEqual(record.SecretHash, trustedDeviceSecretHash(secret)) {
		return false, nil
	}
	nextSecret, err := common.GenerateRandomCharsKey(trustedDeviceSecretLen)
	if err != nil {
		return false, err
	}
	record.SecretHash = trustedDeviceSecretHash(nextSecret)
	record.UserAgent = truncateAuthMetadata(c.Request.UserAgent(), 512)
	record.IP = truncateAuthMetadata(c.ClientIP(), 64)
	payload, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	if err := store.Set(ctx, trustedDeviceKey(deviceID), string(payload), TrustedDeviceTTL); err != nil {
		return false, err
	}
	if err := store.SetAdd(ctx, indexKey, deviceID, trustedDeviceIndexTTL); err != nil {
		return false, err
	}
	WriteTrustedDeviceCookie(c, deviceID+"."+nextSecret)
	return true, nil
}

// currentTrustedDeviceToken returns the cookie's device ID and secret without
// validating them; callers still have to compare the stored hash.
func currentTrustedDeviceToken(c *gin.Context) (string, string, bool) {
	raw, err := c.Cookie(TrustedDeviceCookieName)
	if err != nil {
		return "", "", false
	}
	return splitTrustedDeviceToken(raw)
}

// ValidateTrustedDevice reports whether the request carries a live device
// record for exactly this phone number. Any storage error is returned so the
// caller can fail closed and demand the challenge.
func ValidateTrustedDevice(c *gin.Context, phone string) (bool, error) {
	if c == nil || c.Request == nil {
		return false, nil
	}
	raw, err := c.Cookie(TrustedDeviceCookieName)
	if err != nil || strings.TrimSpace(raw) == "" {
		return false, nil
	}
	deviceID, secret, ok := splitTrustedDeviceToken(raw)
	if !ok {
		return false, nil
	}
	store := trustedDeviceStoreInstance()
	value, found, err := store.Get(c.Request.Context(), trustedDeviceKey(deviceID))
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	var record trustedDeviceRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return false, nil
	}
	if !constantTimeStringEqual(record.SecretHash, trustedDeviceSecretHash(secret)) {
		return false, nil
	}
	user, err := trustedDeviceUserLookup(record.UserID, false)
	if err != nil {
		return false, nil
	}
	if user.Status != common.UserStatusEnabled || user.AuthVersion != record.AuthVersion {
		return false, nil
	}
	normalizedPhone, err := NormalizePhoneInput(phone)
	if err != nil {
		return false, nil
	}
	if !constantTimeStringEqual(record.PhoneHash, trustedDevicePhoneHash(normalizedPhone)) {
		return false, nil
	}
	// A number moved to another account must not inherit the old trust.
	if !constantTimeStringEqual(record.PhoneHash, trustedDevicePhoneHash(model.NormalizePhone(user.Phone))) {
		return false, nil
	}
	return true, nil
}

// TrustedDeviceIDFromRequest returns the device ID in the cookie, if any.
func TrustedDeviceIDFromRequest(c *gin.Context) string {
	raw, err := c.Cookie(TrustedDeviceCookieName)
	if err != nil {
		return ""
	}
	deviceID, _, ok := splitTrustedDeviceToken(raw)
	if !ok {
		return ""
	}
	return deviceID
}

// ListTrustedDevices returns the account's live devices, newest first.
func ListTrustedDevices(userID int, currentDeviceID string) ([]TrustedDeviceView, error) {
	if userID <= 0 {
		return nil, ErrTrustedDeviceNotFound
	}
	ctx := context.Background()
	store := trustedDeviceStoreInstance()
	indexKey := trustedUserIndexKey(userID)
	members, err := store.SetMembers(ctx, indexKey)
	if err != nil {
		return nil, err
	}
	views := make([]TrustedDeviceView, 0, len(members))
	for _, member := range members {
		value, found, err := store.Get(ctx, trustedDeviceKey(member))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		var record trustedDeviceRecord
		if err := json.Unmarshal([]byte(value), &record); err != nil {
			continue
		}
		if record.UserID != userID {
			continue
		}
		views = append(views, TrustedDeviceView{
			ID:        member,
			Current:   member == currentDeviceID,
			UserAgent: record.UserAgent,
			IP:        record.IP,
			CreatedAt: record.CreatedAt,
			ExpiresAt: record.CreatedAt + int64(TrustedDeviceTTL/time.Second),
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].CreatedAt > views[j].CreatedAt
	})
	return views, nil
}

// RevokeTrustedDevice removes one device. The ownership check happens before
// deletion so a user cannot revoke someone else's device by ID.
func RevokeTrustedDevice(userID int, deviceID string) error {
	if userID <= 0 {
		return ErrTrustedDeviceNotFound
	}
	deviceID = strings.TrimSpace(deviceID)
	if _, err := uuid.Parse(deviceID); err != nil {
		return ErrTrustedDeviceNotFound
	}
	ctx := context.Background()
	store := trustedDeviceStoreInstance()
	value, found, err := store.Get(ctx, trustedDeviceKey(deviceID))
	if err != nil {
		return err
	}
	if !found {
		return ErrTrustedDeviceNotFound
	}
	var record trustedDeviceRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil || record.UserID != userID {
		return ErrTrustedDeviceNotFound
	}
	if err := store.Del(ctx, trustedDeviceKey(deviceID)); err != nil {
		return err
	}
	return store.SetRemove(ctx, trustedUserIndexKey(userID), deviceID)
}

// RevokeAllTrustedDevices removes every device for an account.
func RevokeAllTrustedDevices(userID int) error {
	if userID <= 0 {
		return ErrTrustedDeviceNotFound
	}
	ctx := context.Background()
	store := trustedDeviceStoreInstance()
	members, err := store.SetMembers(ctx, trustedUserIndexKey(userID))
	if err != nil {
		return err
	}
	for _, member := range members {
		if err := store.Del(ctx, trustedDeviceKey(member)); err != nil {
			return err
		}
	}
	return store.Del(ctx, trustedUserIndexKey(userID))
}

// WriteTrustedDeviceCookie scopes the cookie to /api so it reaches the send
// endpoint but not the rest of the site.
func WriteTrustedDeviceCookie(c *gin.Context, rawToken string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     TrustedDeviceCookieName,
		Value:    rawToken,
		Path:     "/api",
		MaxAge:   int(TrustedDeviceTTL / time.Second),
		Expires:  time.Now().Add(TrustedDeviceTTL),
		HttpOnly: true,
		Secure:   common.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearTrustedDeviceCookie removes the cookie from the current browser.
func ClearTrustedDeviceCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     TrustedDeviceCookieName,
		Value:    "",
		Path:     "/api",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   common.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}
