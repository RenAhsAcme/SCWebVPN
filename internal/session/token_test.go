package session

import "testing"

func TestTokenIsOpaqueAndHashOnly(t *testing.T) {
	plain, digest, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateToken(plain); err != nil {
		t.Fatal(err)
	}
	if digest != HashToken(plain) {
		t.Fatal("token digest changed")
	}
	if digest == HashToken(plain+"x") {
		t.Fatal("different capabilities shared a digest")
	}
}
