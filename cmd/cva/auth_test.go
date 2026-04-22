package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadAuthUserIDUsesDefault(t *testing.T) {
	t.Parallel()

	userID, err := readAuthUserID(bufio.NewReader(strings.NewReader("\n")), &strings.Builder{})
	if err != nil {
		t.Fatalf("readAuthUserID() error = %v", err)
	}
	if userID != "admin" {
		t.Fatalf("userID = %q, want admin", userID)
	}
}

func TestReadAuthUserIDRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	if _, err := readAuthUserID(bufio.NewReader(strings.NewReader("bad:id\n")), &strings.Builder{}); err == nil {
		t.Fatal("readAuthUserID() error = nil, want invalid user id error")
	}
}
