package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("a secure passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "a secure passphrase") {
		t.Fatal("correct password was rejected")
	}
	if VerifyPassword(hash, "wrong passphrase") {
		t.Fatal("incorrect password was accepted")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("short account password was accepted")
	}
	if _, err := HashPassword("12345678901"); err == nil {
		t.Fatal("11-char account password was accepted (min 12)")
	}
	if _, err := HashPassword("123456789012"); err != nil {
		t.Fatalf("12-char valid password rejected: %v", err)
	}
}

func TestSharePasswordMinimumLength(t *testing.T) {
	if _, err := HashSharePassword("short"); err == nil {
		t.Fatal("short share password was accepted")
	}
	if _, err := HashSharePassword("12345678901"); err == nil {
		t.Fatal("11-char share password was accepted (min 12)")
	}
	hash, err := HashSharePassword("123456789012")
	if err != nil {
		t.Fatalf("valid 12-char share password rejected: %v", err)
	}
	if !VerifyPassword(hash, "123456789012") {
		t.Fatal("share password verification failed")
	}
}

func TestTokenHashIsStableAndRawTokenIsValidated(t *testing.T) {
	raw, hash, err := Token()
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := TokenFromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) != string(again) {
		t.Fatal("token hashes differ")
	}
	if _, _, err := TokenFromRaw("not a token"); err == nil {
		t.Fatal("invalid token was accepted")
	}
}
