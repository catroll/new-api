package model

import (
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrPhoneNotFound     = errors.New("Phone number not found")
	ErrPhoneAmbiguous    = errors.New("Phone number is linked to more than one account")
	ErrPhoneAlreadyTaken = errors.New("This phone number is already in use")
	ErrPhoneInvalid      = errors.New("Invalid phone number")
)

// A canonical phone is "+" followed by 7-15 digits (E.164). Stricter
// per-country rules live in the service layer, which can read the settings.
var canonicalPhonePattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// NormalizePhone converts user input to the stored form: "+" plus the country
// code and digits, with spaces, dashes and parentheses removed.
//
//	13800138000      -> +8613800138000
//	+86 138-0013-8000 -> +8613800138000
//	008613800138000  -> +8613800138000
//
// It is idempotent: normalizing an already canonical value returns it unchangeed.
func NormalizePhone(phone string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '-', '(', ')', '.':
			return -1
		}
		return r
	}, strings.TrimSpace(phone))
	cleaned = strings.TrimPrefix(cleaned, "00")
	if cleaned == "" {
		return ""
	}
	if strings.HasPrefix(cleaned, "+") {
		return "+" + strings.TrimLeft(cleaned, "+")
	}
	if len(cleaned) == 13 && strings.HasPrefix(cleaned, "86") {
		return "+" + cleaned
	}
	if len(cleaned) == 11 && strings.HasPrefix(cleaned, "1") {
		return "+86" + cleaned
	}
	return "+" + cleaned
}

// ValidateNormalizedPhone checks the canonical shape only (no country policy).
func ValidateNormalizedPhone(phone string) error {
	if !canonicalPhonePattern.MatchString(phone) {
		return ErrPhoneInvalid
	}
	return nil
}

func phoneQuery(tx *gorm.DB, phone string) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).Where("phone = ?", phone)
}

// IsPhoneAvailable reports whether the number is unclaimed. Soft-deleted rows
// keep their number reserved, exactly like email.
func IsPhoneAvailable(phone string, excludeUserID int) (bool, error) {
	phone = NormalizePhone(phone)
	if phone == "" {
		return true, nil
	}
	query := phoneQuery(DB, phone)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func EnsurePhoneAvailable(phone string, excludeUserID int) error {
	available, err := IsPhoneAvailable(phone, excludeUserID)
	if err != nil {
		return err
	}
	if !available {
		return ErrPhoneAlreadyTaken
	}
	return nil
}

// lockNormalizedPhone serialises "check then write" for a number inside tx.
// See lockNormalizedEmail for the dialect-specific reasoning.
func lockNormalizedPhone(tx *gorm.DB, phone string) error {
	phone = NormalizePhone(phone)
	if phone == "" {
		return nil
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", phone).Error; err != nil {
			return err
		}
	case common.UsingMainDatabase(common.DatabaseTypeMySQL):
		var ids []int
		if err := tx.Raw("SELECT id FROM users WHERE phone = ? FOR UPDATE", phone).Scan(&ids).Error; err != nil {
			return err
		}
	}
	return nil
}

// lockAndEnsurePhoneAvailable takes the per-number ownership lock inside tx and
// re-checks availability, so two concurrent registrations cannot claim one
// number even though the pre-check outside the transaction said it was free.
func lockAndEnsurePhoneAvailable(tx *gorm.DB, phone string) error {
	phone = NormalizePhone(phone)
	if phone == "" {
		return nil
	}
	if err := lockNormalizedPhone(tx, phone); err != nil {
		return err
	}
	var count int64
	if err := phoneQuery(tx, phone).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPhoneAlreadyTaken
	}
	return nil
}

// GetUniqueUserByPhone resolves an active account by number. A number shared by
// several rows (possible only with manual SQL) is reported instead of guessing.
func GetUniqueUserByPhone(phone string) (*User, error) {
	phone = NormalizePhone(phone)
	if phone == "" {
		return nil, ErrPhoneNotFound
	}
	var users []User
	if err := DB.Where("phone = ?", phone).Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, ErrPhoneNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, ErrPhoneAmbiguous
	}
}

// BindUserPhone attaches a verified number to an account. The caller must have
// verified the code and the step-up proof for account.binding.bind.
func BindUserPhone(userID int, phone string) error {
	phone = NormalizePhone(phone)
	if userID <= 0 || ValidateNormalizedPhone(phone) != nil {
		return ErrPhoneInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockNormalizedPhone(tx, phone); err != nil {
			return err
		}
		var count int64
		if err := phoneQuery(tx, phone).Where("id <> ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrPhoneAlreadyTaken
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("phone", phone).Error
	})
}

// ResetUserPasswordByPhone mirrors ResetUserPasswordByEmail: rotate the hash,
// bump the auth version and revoke every session.
func ResetUserPasswordByPhone(phone string, password string) error {
	if phone == "" || password == "" {
		return common.NewMessage("Phone or password is empty!")
	}
	user, err := GetUniqueUserByPhone(phone)
	if err != nil {
		return err
	}
	hashedPassword, err := common.HashAccountPassword(password)
	if err != nil {
		return err
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := IncrementUserAuthVersionWithTx(tx, user.Id); err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", user.Id).Update("password", hashedPassword).Error
	}); err != nil {
		return err
	}
	if err := PublishUserAuthCache(user.Id); err != nil {
		return err
	}
	_, err = RevokeAllUserSessions(user.Id, "password_reset")
	return err
}
