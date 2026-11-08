package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestAZIndexPastThirtyTwoBitsIsStillOne: z-index-001 and z-index-012 set a
// z-index one past what 32 bits hold, either side of zero. Where int is 32
// bits it was read as auto, and the box stacked as if it had none; it is a
// z-index above, or below, every ordinary one on every platform.
func TestAZIndexPastThirtyTwoBitsIsStillOne(t *testing.T) {
	for _, tc := range []struct {
		value string
		above bool // above 100, or else below -100
	}{
		{"2147483648", true},
		{"9223372036854775807", true},
		{"-2147483649", false},
		{"-9223372036854775808", false},
	} {
		z, auto := zIndexOf(style.Initial().With("z-index", tc.value))
		if auto || tc.above && z <= 100 || !tc.above && z >= -100 {
			t.Errorf("z-index %s reads as %d (auto %v)", tc.value, z, auto)
		}
	}
	// Past what 64 bits hold is not an integer this reads, as before.
	if _, auto := zIndexOf(style.Initial().With("z-index", "99999999999999999999")); !auto {
		t.Error("a z-index past 64 bits was read as a number")
	}
}
