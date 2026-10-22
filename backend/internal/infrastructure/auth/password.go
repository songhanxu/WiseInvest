package auth

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_][a-z0-9_.-]{2,31}$`)

func NormalizeUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return "", fmt.Errorf("账号需为 3–32 位字母、数字、下划线、点或短横线")
	}
	return username, nil
}

func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 6 || len(password) > 72 {
		return fmt.Errorf("密码至少 6 位，最多 72 字节")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func HasPassword(hash string) bool { _, err := bcrypt.Cost([]byte(hash)); return err == nil }
