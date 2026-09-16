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
		t.Fatal("short password was accepted")
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
