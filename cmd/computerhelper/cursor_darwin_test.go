//go:build darwin

package main

import (
	"context"
	"testing"
)

func TestMacElementCenterRejectsMalformedElementIDBeforeOSCall(t *testing.T) {
	if _, err := macElementCenter(context.Background(), "not-an-element-id"); err == nil {
		t.Fatal("expected malformed element id to be rejected")
	}
	if _, err := macElementCenter(context.Background(), "nope:0.1"); err == nil {
		t.Fatal("expected non-numeric pid to be rejected")
	}
}
