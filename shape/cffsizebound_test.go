package shape

import (
	"strings"
	"testing"
)

// TestACFFTooLargeToSubsetIsRefused is the bound on the CFF program the
// subsetter will work on.
//
// A CFF is a font's outlines and a page may link any font. The subsetter walks
// it with offsets read from the font itself, so what it costs is decided by what
// the font declares; the bound is what stops a program too large to walk from
// being walked at all.
//
// Nothing had handed it one: every CFF fixture here is a few hundred bytes, so
// the shape suite passes with maxCFFSize raised.
func TestACFFTooLargeToSubsetIsRefused(t *testing.T) {
	// A literal, not maxCFFSize+1. Sized from the constant, a plant that raises
	// the bound asks this to allocate a terabyte — and a test that dies trying
	// reads as a broken machine rather than a bound that moved.
	const past = 1<<26 + 1
	if maxCFFSize != 1<<26 {
		t.Fatalf("maxCFFSize is %d and this fixture is %d bytes; it states the size "+
			"rather than following it, so it wants looking at", maxCFFSize, past)
	}

	// The bytes are never read: the length is checked before anything else, and
	// that is the point — a program this large is refused rather than parsed.
	_, _, err := subsetCFF(make([]byte, past), nil, fullBudget())
	if err == nil {
		t.Fatal("a CFF program past the bound was subsetted; the size is checked " +
			"before the walk, so that the walk is never the thing that finds out")
	}
	if !strings.Contains(err.Error(), "too large to subset") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the size, "+
			"not something the walk noticed afterwards", err)
	}

	// And a short one is refused for its own reason, or the test above would
	// pass on any input at all.
	if _, _, err := subsetCFF([]byte{1, 2}, nil, fullBudget()); err == nil {
		t.Error("a two-byte CFF was accepted")
	} else if strings.Contains(err.Error(), "too large to subset") {
		t.Errorf("a two-byte CFF was refused as too large: %v", err)
	}
}
