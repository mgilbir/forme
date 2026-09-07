package paragraph

import "testing"

// TestANumberIsWhatCSSCallsOne.
//
// <number> is "[+-]? [digits ['.' digits]? | '.' digits] [e [+-]? digits]?".
// Two halves of that were missing in opposite directions: an exponent was
// refused, so "line-height: 1e2" was not a hundred, and a dot with nothing
// after it was accepted, so "5." was five where no browser reads it as
// anything.
func TestANumberIsWhatCSSCallsOne(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want float64
		ok   bool
	}{
		{"1", 1, true},
		{"1.5", 1.5, true},
		{".5", 0.5, true},
		{"+5", 5, true},
		{"-5", -5, true},
		{"1e2", 100, true},
		{"1E2", 100, true},
		{"1e+2", 100, true},
		{"1e-2", 0.01, true},
		{"1.5e2", 150, true},
		{".5e1", 5, true},
		{"-1e2", -100, true},

		{"5.", 0, false},
		{".", 0, false},
		{"", 0, false},
		{"e2", 0, false},
		{"1e", 0, false},
		{"1e+", 0, false},
		{"1ee2", 0, false},
		{"1.2.3", 0, false},
		{"1e1000", 0, false},
		{"abc", 0, false},
	} {
		got, ok := ParseNumber(tc.src)
		if ok != tc.ok {
			t.Errorf("%q parses = %v, want %v", tc.src, ok, tc.ok)
			continue
		}
		if d := got - tc.want; ok && (d > 1e-9 || d < -1e-9) {
			t.Errorf("%q is %v, want %v", tc.src, got, tc.want)
		}
	}
}

// TestAnIntegerMayCarryASign. CSS's <integer> takes a leading sign, and "+3" is
// three — so "-webkit-line-clamp: +3", an integer written the way the grammar
// allows, clamped nothing.
func TestAnIntegerMayCarryASign(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
		ok   bool
	}{
		{"3", 3, true},
		{"+3", 3, true},
		{" +3 ", 3, true},

		{"-3", 0, false},
		{"+", 0, false},
		{"", 0, false},
		{"3.5", 0, false},
		{"0", 0, false},
	} {
		got, ok := PositiveInteger(tc.src)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%q parses to (%d, %v), want (%d, %v)", tc.src, got, ok, tc.want, tc.ok)
		}
	}
}

// TestAnUnmatchedLeaveDoesNotPanic. A Leave with no Enter to match is a
// caller's mistake, and not a reason to bring a layout engine down: it used to
// slice a stack of nothing and panic, reached from a tree the caller built with
// no way back.
func TestAnUnmatchedLeaveDoesNotPanic(t *testing.T) {
	var b BidiBuilder
	b.Leave([]rune{'‪'}, []rune{'‬'})
	b.Leave([]rune{'‪'}, []rune{'‬'})

	// And the ordinary pairing still works.
	var ok BidiBuilder
	ok.Add("a")
	ok.Enter([]rune{'‪'})
	ok.Add("b")
	ok.Leave([]rune{'‪'}, []rune{'‬'})
	ok.Add("c")
	if len(ok.Paras) == 0 {
		t.Error("a balanced builder produced no paragraph")
	}
}
