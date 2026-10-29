package model

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var phoneTestSequence atomic.Int64

func setupPhoneTestDB(t *testing.T) {
	t.Helper()
	previousDB := DB
	previousMain := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMain, previousMain)
	})
}

func newPhoneTestUser(t *testing.T, username string) *User {
	t.Helper()
	sequence := phoneTestSequence.Add(1)
	password, err := common.Password2Hash("test-password")
	require.NoError(t, err)
	user := &User{
		Username:    username,
		Password:    password,
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     username + "-aff",
	}
	require.NoError(t, DB.Create(user).Error)
	_ = sequence
	return user
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"13800138000":       "+8613800138000",
		"+86 138-0013-8000": "+8613800138000",
		"008613800138000":   "+8613800138000",
		"8613800138000":     "+8613800138000",
		"(138) 0013 8000":   "+8613800138000",
		"+1 202 555 0123":   "+12025550123",
		"":                  "",
	}
	for input, want := range cases {
		if got := NormalizePhone(input); got != want {
			t.Fatalf("NormalizePhone(%q) = %q, want %q", input, got, want)
		}
		if got := NormalizePhone(want); got != want {
			t.Fatalf("NormalizePhone is not idempotent for %q: %q", want, got)
		}
	}
}

func TestValidateNormalizedPhone(t *testing.T) {
	require.NoError(t, ValidateNormalizedPhone("+8613800138000"))
	for _, invalid := range []string{"", "13800138000", "+86", "+08613800138000", "+8613800138000123"} {
		if err := ValidateNormalizedPhone(invalid); !errors.Is(err, ErrPhoneInvalid) {
			t.Fatalf("ValidateNormalizedPhone(%q) = %v, want ErrPhoneInvalid", invalid, err)
		}
	}
}

func TestPhoneAvailabilityReservesSoftDeletedRows(t *testing.T) {
	setupPhoneTestDB(t)
	user := newPhoneTestUser(t, "phone-owner")
	require.NoError(t, BindUserPhone(user.Id, "+8613800138000"))

	available, err := IsPhoneAvailable("+8613800138000", 0)
	require.NoError(t, err)
	require.False(t, available, "a bound number must be reported as taken")

	available, err = IsPhoneAvailable("+8613800138000", user.Id)
	require.NoError(t, err)
	require.True(t, available, "the same account may keep its own number")

	require.NoError(t, DB.Delete(&User{}, user.Id).Error)
	available, err = IsPhoneAvailable("+8613800138000", 0)
	require.NoError(t, err)
	require.False(t, available, "soft-deleted accounts keep their number reserved, like email")
}

func TestBindUserPhoneRejectsSecondOwner(t *testing.T) {
	setupPhoneTestDB(t)
	first := newPhoneTestUser(t, "phone-first")
	second := newPhoneTestUser(t, "phone-second")

	require.NoError(t, BindUserPhone(first.Id, "+8613800138000"))
	if err := BindUserPhone(second.Id, "+8613800138000"); !errors.Is(err, ErrPhoneAlreadyTaken) {
		t.Fatalf("second bind = %v, want ErrPhoneAlreadyTaken", err)
	}

	user, err := GetUniqueUserByPhone("8613800138000")
	require.NoError(t, err)
	require.Equal(t, first.Id, user.Id)
}

func TestGetUniqueUserByPhoneReportsMissingNumber(t *testing.T) {
	setupPhoneTestDB(t)
	if _, err := GetUniqueUserByPhone("+8613800138001"); !errors.Is(err, ErrPhoneNotFound) {
		t.Fatalf("GetUniqueUserByPhone() = %v, want ErrPhoneNotFound", err)
	}
}

// The admin binding management dialog clears a phone exactly like the other
// bindings, so the column must be released and the number must become
// claimable by another account afterwards.
func TestClearBindingReleasesPhone(t *testing.T) {
	setupPhoneTestDB(t)
	owner := newPhoneTestUser(t, "phone-clear-owner")
	other := newPhoneTestUser(t, "phone-clear-other")
	require.NoError(t, BindUserPhone(owner.Id, "+8613800138000"))

	require.NoError(t, owner.ClearBinding("phone"))
	require.Empty(t, owner.Phone)

	available, err := IsPhoneAvailable("+8613800138000", 0)
	require.NoError(t, err)
	require.True(t, available, "clearing the binding must release the number")

	require.NoError(t, BindUserPhone(other.Id, "+8613800138000"))
	resolved, err := GetUniqueUserByPhone("+8613800138000")
	require.NoError(t, err)
	require.Equal(t, other.Id, resolved.Id)
}
