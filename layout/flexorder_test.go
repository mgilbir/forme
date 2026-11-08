package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestOrderIsBoundedWithoutWrapping: an order past the bound is read as the
// bound, on either side of zero, where int is 32 bits as where it is 64 — the
// digit before the bound was multiplied by ten after it and wrapped negative
// there (#989's sweep, make test-386).
func TestOrderIsBoundedWithoutWrapping(t *testing.T) {
	const bound = 1 << 30
	for value, want := range map[string]int{
		"0": 0, "7": 7, "-3": -3, "+12": 12,
		"1073741824":              bound,
		"1073741825":              bound,
		"2147483648":              bound,
		"-9999999999":             -bound,
		"99999999999999999999999": bound,
		"2.5":                     0,
	} {
		b := &Box{Style: style.Initial().With("order", value)}
		if got := orderOf(b); got != want {
			t.Errorf("order %s reads as %d, want %d", value, got, want)
		}
	}
}
