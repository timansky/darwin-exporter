//go:build darwin

package cmd

import "testing"

func TestSetColorMode_AlwaysEnablesANSI(t *testing.T) {
	t.Cleanup(func() { SetColorMode("never") })
	SetColorMode("always")

	got := styleSuccess("ok")
	if got == "ok" {
		t.Fatal("expected ANSI-colored output for always mode")
	}
}

func TestSetColorMode_NeverDisablesANSI(t *testing.T) {
	SetColorMode("always")
	SetColorMode("never")

	if got := styleSuccess("ok"); got != "ok" {
		t.Fatalf("expected plain output for never mode, got %q", got)
	}
}
