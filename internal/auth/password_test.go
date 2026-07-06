package auth

import (
	"testing"
)

func TestHashVerify_RoundTrip(t *testing.T) {
	encoded, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("hunter2", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected match")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	encoded, _ := HashPassword("hunter2")
	ok, err := VerifyPassword("wrong", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no match")
	}
}

func TestVerify_InvalidEncoded(t *testing.T) {
	_, err := VerifyPassword("x", "not-a-valid-hash")
	if err == nil {
		t.Fatal("expected error")
	}
}
