package crypto

import (
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestCipherAndTokens(t *testing.T) {
	cipher := NewCipher("test-master-secret")

	enc, err := cipher.Encrypt("LTAI_SECRET_12345")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "LTAI_SECRET_12345" || enc == "" {
		t.Fatalf("expected encrypted ciphertext, got %q", enc)
	}
	plain, err := cipher.Decrypt(enc)
	if err != nil || plain != "LTAI_SECRET_12345" {
		t.Fatalf("Decrypt = (%q, %v)", plain, err)
	}
	if empty, err := cipher.Encrypt(""); err != nil || empty != "" {
		t.Fatalf("Encrypt empty = (%q, %v)", empty, err)
	}

	token, err := SignToken("sess-secret", "admin", time.Hour)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	user, ok := VerifyToken("sess-secret", token)
	if !ok || user != "admin" {
		t.Fatalf("VerifyToken = (%q, %v)", user, ok)
	}
	if _, ok := VerifyToken("wrong-secret", token); ok {
		t.Fatal("VerifyToken should reject wrong secret")
	}
	expired, _ := SignToken("sess-secret", "admin", -time.Minute)
	if _, ok := VerifyToken("sess-secret", expired); ok {
		t.Fatal("VerifyToken should reject expired token")
	}
}

func TestPasswordVerification(t *testing.T) {
	if !VerifyPassword("abc123", "abc123", "") {
		t.Fatal("expected plaintext password match")
	}
	if VerifyPassword("abc124", "abc123", "") {
		t.Fatal("expected plaintext mismatch")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("bcrypt-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("bcrypt-pass", "", string(hash)) {
		t.Fatal("expected bcrypt password match")
	}
	if VerifyPassword("wrong-pass", "", string(hash)) {
		t.Fatal("expected bcrypt mismatch")
	}
}
