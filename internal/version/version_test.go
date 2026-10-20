package version

import "testing"

func TestString(t *testing.T) {
	if got := String(); got != "fluxor "+Version {
		t.Fatalf("unexpected version string: %q", got)
	}
}
