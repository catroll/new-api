package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type trustedDeviceTestEnv struct {
	store *memoryTrustedDeviceStore
	user  *model.User
	now   *time.Time
}

func newTrustedDeviceTestEnv(t *testing.T) *trustedDeviceTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousSecret := common.SessionSecret
	common.SessionSecret = "trusted-device-test-secret"
	previousStore := trustedDeviceStoreOverride
	store := newMemoryTrustedDeviceStore()
	trustedDeviceStoreOverride = store
	previousLookup := trustedDeviceUserLookup
	user := &model.User{
		Id:          42,
		AuthVersion: 7,
		Status:      common.UserStatusEnabled,
		Phone:       "+8613800138000",
	}
	trustedDeviceUserLookup = func(id int, _ bool) (*model.User, error) {
		if id != user.Id {
			return nil, gorm.ErrRecordNotFound
		}
		return user, nil
	}
	previousClock := trustedDeviceClock
	now := time.Unix(1_700_000_000, 0)
	trustedDeviceClock = func() time.Time { return now }
	t.Cleanup(func() {
		common.SessionSecret = previousSecret
		trustedDeviceStoreOverride = previousStore
		trustedDeviceUserLookup = previousLookup
		trustedDeviceClock = previousClock
	})
	return &trustedDeviceTestEnv{store: store, user: user, now: &now}
}

func newTrustedDeviceContext(t *testing.T) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/user/login/sms", nil)
	request.RemoteAddr = "203.0.113.9:4321"
	request.Header.Set("User-Agent", "trusted-device-test/1.0")
	c.Request = request
	return c
}

func contextWithCookie(t *testing.T, cookie *http.Cookie) *gin.Context {
	t.Helper()
	c := newTrustedDeviceContext(t)
	c.Request.AddCookie(cookie)
	return c
}

func issuedCookie(t *testing.T, c *gin.Context) *http.Cookie {
	t.Helper()
	response := &http.Response{Header: c.Writer.Header()}
	cookies := response.Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected a Set-Cookie header, got %v", c.Writer.Header())
	}
	return cookies[0]
}

func TestTrustedDeviceRoundTrip(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	issueContext := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(issueContext, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	cookie := issuedCookie(t, issueContext)
	if !cookie.HttpOnly {
		t.Fatal("trusted device cookie must be HttpOnly")
	}
	if cookie.Path != "/api" {
		t.Fatalf("cookie path = %q, want /api", cookie.Path)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Secure != common.SessionCookieSecure {
		t.Fatalf("cookie Secure = %v, want %v", cookie.Secure, common.SessionCookieSecure)
	}
	if cookie.MaxAge <= 0 {
		t.Fatalf("cookie MaxAge = %d, want positive", cookie.MaxAge)
	}

	trusted, err := ValidateTrustedDevice(contextWithCookie(t, cookie), env.user.Phone)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !trusted {
		t.Fatal("expected the issued device to be trusted")
	}

	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, cookie), "+8613900139000"); trusted {
		t.Fatal("a different phone number must not reuse the device")
	}

	tampered := &http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"}
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, tampered), env.user.Phone); trusted {
		t.Fatal("a tampered secret must not validate")
	}
	if trusted, _ := ValidateTrustedDevice(newTrustedDeviceContext(t), env.user.Phone); trusted {
		t.Fatal("a request without the cookie must not validate")
	}
}

func TestTrustedDeviceValidationTracksAccountState(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	issueContext := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(issueContext, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	cookie := issuedCookie(t, issueContext)

	env.user.AuthVersion = 8
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, cookie), env.user.Phone); trusted {
		t.Fatal("a bumped auth version must invalidate the device")
	}

	env.user.AuthVersion = 7
	env.user.Phone = "+8613900139000"
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, cookie), "+8613900139000"); trusted {
		t.Fatal("a rebound phone number must invalidate the device")
	}
}

func TestTrustedDeviceRevoke(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	issueContext := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(issueContext, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	cookie := issuedCookie(t, issueContext)
	deviceID, _, ok := splitTrustedDeviceToken(cookie.Value)
	if !ok {
		t.Fatal("issued cookie is malformed")
	}

	if err := RevokeTrustedDevice(999, deviceID); !errors.Is(err, ErrTrustedDeviceNotFound) {
		t.Fatalf("revoking another account's device: err = %v, want ErrTrustedDeviceNotFound", err)
	}
	if err := RevokeTrustedDevice(env.user.Id, deviceID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, cookie), env.user.Phone); trusted {
		t.Fatal("a revoked device must not validate")
	}
	if err := RevokeTrustedDevice(env.user.Id, deviceID); !errors.Is(err, ErrTrustedDeviceNotFound) {
		t.Fatalf("second revoke: err = %v, want ErrTrustedDeviceNotFound", err)
	}
}

func TestTrustedDeviceLimitAndPrune(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	var cookies []*http.Cookie
	for i := 0; i < trustedDeviceMaxPerUser; i++ {
		c := newTrustedDeviceContext(t)
		if err := IssueTrustedDevice(c, env.user, env.user.Phone); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
		cookies = append(cookies, issuedCookie(t, c))
	}
	if err := IssueTrustedDevice(newTrustedDeviceContext(t), env.user, env.user.Phone); !errors.Is(err, ErrTrustedDeviceLimit) {
		t.Fatalf("sixth issue: err = %v, want ErrTrustedDeviceLimit", err)
	}

	// Drop the index entry only: the next issue must prune the stale member
	// instead of blocking forever on a device record that is already gone.
	deviceID, _, _ := splitTrustedDeviceToken(cookies[0].Value)
	_ = env.store.SetRemove(context.Background(), trustedUserIndexKey(env.user.Id), deviceID)
	if _, found, _ := env.store.Get(context.Background(), trustedDeviceKey(deviceID)); !found {
		t.Fatal("device record should still exist for the prune test")
	}
	if err := IssueTrustedDevice(newTrustedDeviceContext(t), env.user, env.user.Phone); err != nil {
		t.Fatalf("issue after index prune: %v", err)
	}
}

func TestTrustedDeviceExpiresWithStoreTTL(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	issueContext := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(issueContext, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	cookie := issuedCookie(t, issueContext)

	*env.now = env.now.Add(TrustedDeviceTTL + time.Minute)
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, cookie), env.user.Phone); trusted {
		t.Fatal("an expired device must not validate")
	}
	devices, err := ListTrustedDevices(env.user.Id, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expired devices still listed: %+v", devices)
	}
}

func TestTrustedDeviceStoreRejectsPermanentKeys(t *testing.T) {
	store := newMemoryTrustedDeviceStore()
	ctx := context.Background()
	if err := store.Set(ctx, "trusted:dev:x", "v", 0); !errors.Is(err, ErrTrustedDeviceTTL) {
		t.Fatalf("Set with zero TTL: err = %v, want ErrTrustedDeviceTTL", err)
	}
	if err := store.SetAdd(ctx, "trusted:user:1", "x", -time.Second); !errors.Is(err, ErrTrustedDeviceTTL) {
		t.Fatalf("SetAdd with negative TTL: err = %v, want ErrTrustedDeviceTTL", err)
	}
}

func TestTrustedDeviceListMarksCurrent(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	first := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(first, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	firstCookie := issuedCookie(t, first)
	second := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(second, env.user, env.user.Phone); err != nil {
		t.Fatalf("issue: %v", err)
	}
	secondCookie := issuedCookie(t, second)
	currentID, _, _ := splitTrustedDeviceToken(secondCookie.Value)

	devices, err := ListTrustedDevices(env.user.Id, currentID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	seenCurrent := false
	for _, device := range devices {
		if device.Current {
			seenCurrent = true
			if device.ID != currentID {
				t.Fatalf("current device = %s, want %s", device.ID, currentID)
			}
		}
		if device.ExpiresAt <= device.CreatedAt {
			t.Fatalf("device expiry %d must follow creation %d", device.ExpiresAt, device.CreatedAt)
		}
	}
	if !seenCurrent {
		t.Fatal("no device marked current")
	}
	if firstCookie.Value == secondCookie.Value {
		t.Fatal("each issue must mint a distinct token")
	}
}

func TestTrustedDeviceRotatesInsteadOfAdding(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	first := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(first, env.user, env.user.Phone); err != nil {
		t.Fatalf("first issue: %v", err)
	}
	firstCookie := issuedCookie(t, first)

	second := contextWithCookie(t, firstCookie)
	if err := IssueTrustedDevice(second, env.user, env.user.Phone); err != nil {
		t.Fatalf("second issue: %v", err)
	}
	secondCookie := issuedCookie(t, second)

	firstID, _, _ := splitTrustedDeviceToken(firstCookie.Value)
	secondID, _, _ := splitTrustedDeviceToken(secondCookie.Value)
	if firstID != secondID {
		t.Fatalf("device id changed on rotation: %s -> %s", firstID, secondID)
	}
	if firstCookie.Value == secondCookie.Value {
		t.Fatal("rotation must mint a new secret")
	}

	devices, err := ListTrustedDevices(env.user.Id, secondID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices after rotation, want 1", len(devices))
	}

	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, secondCookie), env.user.Phone); !trusted {
		t.Fatal("the rotated cookie must validate")
	}
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, firstCookie), env.user.Phone); trusted {
		t.Fatal("the pre-rotation cookie must stop validating")
	}
}

func TestTrustedDeviceDoesNotRotateWithWrongSecret(t *testing.T) {
	env := newTrustedDeviceTestEnv(t)
	first := newTrustedDeviceContext(t)
	if err := IssueTrustedDevice(first, env.user, env.user.Phone); err != nil {
		t.Fatalf("first issue: %v", err)
	}
	firstCookie := issuedCookie(t, first)
	deviceID, _, _ := splitTrustedDeviceToken(firstCookie.Value)

	forged := &http.Cookie{Name: firstCookie.Name, Value: deviceID + ".not-the-real-secret"}
	if err := IssueTrustedDevice(contextWithCookie(t, forged), env.user, env.user.Phone); err != nil {
		t.Fatalf("issue with forged cookie: %v", err)
	}
	devices, err := ListTrustedDevices(env.user.Id, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want the forged cookie to create a new device", len(devices))
	}
	if trusted, _ := ValidateTrustedDevice(contextWithCookie(t, firstCookie), env.user.Phone); !trusted {
		t.Fatal("the original device must keep working")
	}
}
