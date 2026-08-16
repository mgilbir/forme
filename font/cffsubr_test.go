package font

import "testing"

// A Type 2 charstring's width is whatever is on the stack when the first
// stack-clearing operator arrives — and a charstring may put it there, or reach
// that operator, from inside a subroutine.
//
// Both halves are ordinary. A subsetter factors the hints and the opening move
// of every glyph into a shared subr precisely because they repeat, so in such a
// font the operator that decides the width is not in the charstring at all.
// Scanning for the first operator answered "no width" for a callsubr — and no
// width is not a refusal, it is defaultWidthX, so every glyph came back the same
// width.

// subrsOf builds an INDEX out of the given charstrings, which is what a Subrs or
// a Global Subr INDEX is.
func subrsOf(items ...[]byte) cffIndex {
	var idx cffIndex
	idx.items = items
	return idx
}

const (
	opHstem    = 1
	opCallsubr = 10
	opReturn   = 11
	opEndchar  = 14
	opRmoveto  = 21
	opCallgsub = 29
)

// num is a Type 2 operand in the one-byte form, which covers -107 to 107.
func num(v int) byte { return byte(v + 139) }

func TestAWidthInsideASubroutineIsFound(t *testing.T) {
	// The subr holds the operator; the width is pushed before the call, which is
	// where a real font puts it — the width is per glyph and the subr is shared.
	//
	// Index 107 is subr 0: the bias for a small INDEX is 107, so the operand
	// that names subr 0 is the encoding of -107.
	subr := []byte{opRmoveto}
	cs := []byte{num(42), num(0), num(0), num(-107), opCallsubr}
	got, has := type2CharstringWidth(cs, subrsOf(subr), cffIndex{})
	if !has {
		t.Fatal("no width found; the operator that decides it is in the subroutine")
	}
	if got != 42 {
		t.Errorf("the width came back %v, want 42", got)
	}
}

func TestAWidthAndItsOperatorBothInsideASubroutine(t *testing.T) {
	// The whole opening — width included — factored out, which is what a
	// subsetter does when every glyph in a range shares one.
	subr := []byte{num(42), num(0), num(0), opRmoveto}
	cs := []byte{num(-107), opCallsubr}
	got, has := type2CharstringWidth(cs, subrsOf(subr), cffIndex{})
	if !has || got != 42 {
		t.Errorf("the width came back %v (found=%v), want 42", got, has)
	}
}

func TestAWidthThroughAGlobalSubroutine(t *testing.T) {
	subr := []byte{opEndchar}
	cs := []byte{num(37), num(-107), opCallgsub}
	got, has := type2CharstringWidth(cs, cffIndex{}, subrsOf(subr))
	if !has || got != 37 {
		t.Errorf("the width came back %v (found=%v), want 37", got, has)
	}
}

func TestASubroutineThatReturnsCarriesOn(t *testing.T) {
	// The subr does the hints and returns; the charstring finishes. The width is
	// still the first operand, and the operator that settles it is the one after
	// the call.
	subr := []byte{opReturn}
	cs := []byte{num(19), num(-107), opCallsubr, opEndchar}
	got, has := type2CharstringWidth(cs, subrsOf(subr), cffIndex{})
	if !has || got != 19 {
		t.Errorf("the width came back %v (found=%v), want 19", got, has)
	}
}

// TestAGlyphWithNoWidthStillHasNone is the other half: following subroutines
// must not invent a width for a charstring that states none, or every glyph in
// the font gets the first operand of whatever its subr happens to push.
func TestAGlyphWithNoWidthStillHasNone(t *testing.T) {
	for _, c := range []struct {
		name  string
		cs    []byte
		subrs cffIndex
	}{
		{"endchar alone", []byte{opEndchar}, cffIndex{}},
		{"rmoveto with exactly its two arguments",
			[]byte{num(0), num(0), opRmoveto}, cffIndex{}},
		{"a subroutine whose operator takes what is on the stack",
			[]byte{num(0), num(0), num(-107), opCallsubr}, subrsOf([]byte{opRmoveto})},
	} {
		if got, has := type2CharstringWidth(c.cs, c.subrs, cffIndex{}); has {
			t.Errorf("%s: a width of %v was found where the charstring states none",
				c.name, got)
		}
	}
}

// TestACyclicSubroutineIsRefusedRatherThanFollowed.
//
// A subroutine may call a subroutine and a hostile font may make that a cycle.
// The specification bounds the nesting at ten; without a bound this is a font
// that makes the parser spin, and ParseCFF runs on whatever a document names.
func TestACyclicSubroutineIsRefusedRatherThanFollowed(t *testing.T) {
	// Subr 0 calls subr 0.
	self := []byte{num(-107), opCallsubr}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, has := type2CharstringWidth([]byte{num(5), num(-107), opCallsubr},
			subrsOf(self), cffIndex{}); has {
			t.Error("a cyclic subroutine reported a width")
		}
	}()
	<-done
}

// TestASubroutineIndexOutOfRangeIsRefused: the index is biased, so a font can
// name one either side of the INDEX it has.
func TestASubroutineIndexOutOfRangeIsRefused(t *testing.T) {
	subrs := subrsOf([]byte{opEndchar})
	for _, idx := range []int{-108, 107, 100} {
		cs := []byte{num(5), num(idx), opCallsubr}
		if _, has := type2CharstringWidth(cs, subrs, cffIndex{}); has {
			t.Errorf("subr operand %d named a subroutine that is not there", idx)
		}
	}
}

// TestTheBiasFollowsTheCount is the number the specification makes depend on how
// many subroutines there are, so that the commonest indices encode in one byte.
func TestTheBiasFollowsTheCount(t *testing.T) {
	for _, c := range []struct{ n, want int }{
		{0, 107}, {1239, 107}, {1240, 1131}, {33899, 1131}, {33900, 32768},
	} {
		if got := subrBias(c.n); got != c.want {
			t.Errorf("a font with %d subroutines biases by %d, want %d", c.n, got, c.want)
		}
	}
}
