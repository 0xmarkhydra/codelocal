package cloudserver

import "testing"

func TestMaskUserEmail(t *testing.T) {
	if got := maskUserEmail("monglv36@gmail.com"); got != "mo***@gmail.com" {
		t.Fatalf("maskUserEmail() = %q", got)
	}
	if got := maskUserEmail("a@example.com"); got != "a***@example.com" {
		t.Fatalf("maskUserEmail(short) = %q", got)
	}
}

func TestUserInitial(t *testing.T) {
	if got := userInitial("mong@example.com"); got != "M" {
		t.Fatalf("userInitial() = %q", got)
	}
	if got := userInitial(""); got != "C" {
		t.Fatalf("userInitial(empty) = %q", got)
	}
}
