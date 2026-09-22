package css

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// The one reading of a <number>: which text is one, what number it is, and
// that the answer is never NaN.

// hostileNumbers are the spellings that have made a NaN or an infinity out of
// a number in this engine, or would in a reader that multiplies by a power of
// ten: a zero under an exponent past the largest float, the same the other way
// round, and mantissas long enough to overflow before the exponent brings them
// back.
var hostileNumbers = []struct {
	in      string
	value   float64
	inRange bool
}{
	// Nought under any exponent is nought. 0·10^400 was 0·Inf, which is NaN.
	{"0e400", 0, true},
	{"-0e400", math.Copysign(0, -1), true},
	{"+0e400", 0, true},
	{"0.0e999999999999999999999", 0, true},
	{"0e-400", 0, true},
	{".0e5", 0, true},
	// Past the largest float64: out of range, and the infinity says which way.
	{"1e400", math.Inf(1), false},
	{"-1e400", math.Inf(-1), false},
	{"1e99999999999999999999", math.Inf(1), false},
	{"1.8e308", math.Inf(1), false},
	{strings.Repeat("9", 310), math.Inf(1), false},
	// Below the smallest float64: nought, which is in range.
	{"1e-400", 0, true},
	{"-1e-400", math.Copysign(0, -1), true},
	{"1e-99999999999999999999", 0, true},
	{"0." + strings.Repeat("0", 400) + "1", 0, true},
	// A mantissa too long for a float64 brought back by the exponent: 10^400 − 1
	// times 10^-400, which was Inf·0.
	{strings.Repeat("9", 400) + "e-400", 1, true},
	{strings.Repeat("9", 310) + "e-310", 1, true},
	// A mantissa longer than strconv.ParseFloat will read an exponent against:
	// it stops accumulating one at five digits, and read this as nought. It is
	// 0.111…, to the last digit a float64 has.
	{strings.Repeat("1", 100000) + "e-100000", 0.1111111111111111, true},
	// The edges of the range, which must survive the normal form exactly.
	{"1.7976931348623157e308", math.MaxFloat64, true},
	{"4.9e-324", math.SmallestNonzeroFloat64, true},
	{"1e308", 1e308, true},
	{"123.456e-2", 1.23456, true},
}

func TestANumberIsNeverNaN(t *testing.T) {
	for _, tc := range hostileNumbers {
		v, inRange, ok := ParseNumber(tc.in)
		name := tc.in
		if len(name) > 40 {
			name = name[:20] + "…" + name[len(name)-12:]
		}
		if !ok {
			t.Errorf("%q is a number and was not read as one", name)
			continue
		}
		if math.IsNaN(v) {
			t.Errorf("%q is NaN", name)
			continue
		}
		if v != tc.value || math.Signbit(v) != math.Signbit(tc.value) || inRange != tc.inRange {
			t.Errorf("%q = %v (in range %v), want %v (in range %v)",
				name, v, inRange, tc.value, tc.inRange)
		}
		if inRange && math.IsInf(v, 0) {
			t.Errorf("%q is %v and called in range", name, v)
		}
		// And the tokenizer reads the same text as the same number, clamping
		// what is out of range rather than refusing it.
		want := v
		if !inRange {
			want = math.Copysign(math.MaxFloat64, v)
		}
		toks, _ := Tokenize(tc.in)
		if len(toks) != 2 || toks[0].Kind != Number {
			t.Errorf("%q tokenizes as %v, not one number", name, toks)
			continue
		}
		if got := toks[0].Number; got != want || math.Signbit(got) != math.Signbit(want) {
			t.Errorf("%q tokenizes as %v and reads as %v", name, got, want)
		}
	}
}

// TestWhatIsNotANumberIsNotOne: Go's own number syntax, and the text around a
// number that makes it not one number.
func TestWhatIsNotANumberIsNotOne(t *testing.T) {
	for _, s := range []string{
		"", " ", "+", "-", ".", "e5", "+e5", "1.", "1e", "1e+", "1e-", ".e1",
		"nan", "NaN", "inf", "-inf", "infinity", "0x1p3", "1_0", "1,5",
		" 1", "1 ", "1px", "1%", "1.5.5", "++1", "+-1", "1+1", "1e5e5", "1e5.5",
		"١", // an Arabic-Indic digit: a digit, and not CSS's
	} {
		if v, _, ok := ParseNumber(s); ok {
			t.Errorf("ParseNumber(%q) = %v; CSS has no way to write it", s, v)
		}
	}
}

// FuzzANumberIsNeverNaN is the property, over text nobody chose: nothing this
// reads is NaN, what it calls in range is finite, what it calls out of range is
// an infinity whose number really is past the largest float64, and the value
// is the text's own — the nearest float64 to the exact decimal, which math/big
// can say independently. And whatever it reads as a whole number, the
// tokenizer reads as the same one.
func FuzzANumberIsNeverNaN(f *testing.F) {
	for _, tc := range hostileNumbers {
		if len(tc.in) < 1000 {
			f.Add(tc.in)
		}
	}
	for _, s := range []string{"1", "-.5", "+1.5e-3", "0000.0001e4", "9e307", "2.5e-324"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 2000 {
			return
		}
		v, inRange, ok := ParseNumber(s)
		if math.IsNaN(v) {
			t.Fatalf("%q is NaN", s)
		}
		if !ok {
			return
		}
		if inRange == math.IsInf(v, 0) {
			t.Fatalf("%q = %v, called in range %v", s, v, inRange)
		}
		exact, _, err := big.ParseFloat(s, 10, 8000, big.ToNearestEven)
		if err != nil {
			// math/big reads the exponent as an int64 and gives up past one.
			// Such an exponent is further from nought than the longest
			// mantissa here can bring back, so the answer is nought or an
			// infinity and nothing in between.
			if v != 0 && !math.IsInf(v, 0) {
				t.Fatalf("%q = %v; an exponent past any int64 is nought or infinite", s, v)
			}
			return
		}
		want, _ := exact.Float64()
		if v != want && !(v == 0 && want == 0) {
			t.Fatalf("%q = %v; the nearest float64 is %v", s, v, want)
		}
		toks, _ := Tokenize(s)
		if len(toks) != 2 || toks[0].Kind != Number {
			t.Fatalf("%q is one number here and tokenizes as %v", s, toks)
		}
		if !inRange {
			v = math.Copysign(math.MaxFloat64, v)
		}
		if toks[0].Number != v {
			t.Fatalf("%q reads as %v and tokenizes as %v", s, v, toks[0].Number)
		}
	})
}
