package password

import (
	"testing"
)

func TestHashVerify_RoundTrip(t *testing.T) {
	encoded, err := Hash("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := Verify("hunter2", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected match")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	encoded, _ := Hash("hunter2")
	ok, err := Verify("wrong", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no match")
	}
}

func TestVerify_InvalidEncoded(t *testing.T) {
	_, err := Verify("x", "not-a-valid-hash")
	if err == nil {
		t.Fatal("expected error")
	}
}
