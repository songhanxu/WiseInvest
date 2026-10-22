package auth

import "testing"

func TestPasswordCredentials(t *testing.T) {
	username, err := NormalizeUsername("  Admin  ")
	if err != nil || username != "admin" {
		t.Fatal("username normalization failed")
	}
	for _, invalid := range []string{"ab", "admin space", "user@name", "../user"} {
		if _, err := NormalizeUsername(invalid); err == nil {
			t.Errorf("accepted invalid username %q", invalid)
		}
	}
	hash, err := HashPassword("test-password")
	if err != nil || hash == "test-password" || !HasPassword(hash) || !CheckPassword(hash, "test-password") || CheckPassword(hash, "wrong-password") {
		t.Fatal("password hashing/check failed")
	}
	if HasPassword("") || HasPassword("$2a$10$dummy.hash.for.demo.user.only") {
		t.Fatal("invalid legacy hashes treated as usable credentials")
	}
	if _, err := HashPassword("12345"); err == nil {
		t.Fatal("accepted too-short password")
	}
	if err := ValidatePassword("654321"); err != nil {
		t.Fatal("six-character passwords rejected")
	}
	if err := ValidatePassword("密码短"); err == nil {
		t.Fatal("password character count ignored")
	}
}
