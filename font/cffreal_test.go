package font

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// A CFF DICT operand can be a BCD real — nibble 0x1e — and its exponent is as
// many digits as the font cares to spend. ParseFloat applies that exponent one
// multiplication at a time, so the digits are a direct instruction to the
// parser about how long to work.
//
// ParseCFF runs on whatever font a document names, so this is reached with
// bytes nobody wrote.

// TestABCDExponentDoesNotBuyUnboundedWork.
//
// The margin here is what makes a clock a fair thing to assert on: the exponent
// is 10^16, which unbounded is on the order of years, and bounded is a few
// hundred multiplications. A second is not close to either.
func TestABCDExponentDoesNotBuyUnboundedWork(t *testing.T) {
	for _, s := range []string{
		"1E9999999999999999",
		"1E-9999999999999999",
		// Long enough to overflow the accumulator rather than merely to take a
		// long time, which is the same input class arriving at a different
		// wrong answer.
		"1E99999999999999999999999999999999999999",
		"-1.5E99999999999999999999",
	} {
		done := make(chan float64, 1)
		go func() {
			var f float64
			ParseFloat(s, &f)
			done <- f
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("ParseFloat(%q) was still going after a second, so the "+
				"exponent decides how long the parser runs", s)
		}
	}
}

// TestABCDExponentSaturatesWhereItShould checks the claim the bound rests on:
// that no exponent past it can change an answer, because a float64 has already
// run out of range. strconv is the oracle — it is not this implementation, and
// infinity and zero are the two answers it can be held to exactly.
func TestABCDExponentSaturatesWhereItShould(t *testing.T) {
	for _, c := range []struct {
		s    string
		want float64
	}{
		{"1E400", math.Inf(1)},
		{"1E700", math.Inf(1)},
		{"1E7000", math.Inf(1)},
		{"1E999999999999", math.Inf(1)},
		{"-1E999999999999", math.Inf(-1)},
		{"1E-400", 0},
		{"1E-700", 0},
		{"1E-999999999999", 0},
		// E292 is the largest finite double exactly, so E293 is the first step
		// past it and E-1000 is a bound's worth the other way. 700 steps
		// carries a value from either end of the range off the far end, which
		// is the whole reason the bound is where it is.
		{"17976931348623157E293", math.Inf(1)},
		{"17976931348623157E-1000", 0},
	} {
		var got float64
		ParseFloat(c.s, &got)
		if got != c.want {
			t.Errorf("ParseFloat(%q) = %v, want %v", c.s, got, c.want)
		}
		// And the same answer strconv gives, for the inputs it accepts.
		if want, err := strconv.ParseFloat(c.s, 64); err == nil {
			if want != c.want {
				t.Errorf("the oracle disagrees with the expectation for %q: "+
					"strconv says %v", c.s, want)
			}
		} else if !math.IsInf(c.want, 0) {
			t.Errorf("strconv refused %q (%v) and it is not an overflow", c.s, err)
		}
	}
}

// TestAnOrdinaryExponentIsUntouched: the bound must not reach an exponent a
// real font uses. A FontMatrix of 0.001 is the one every CFF carries.
func TestAnOrdinaryExponentIsUntouched(t *testing.T) {
	for _, s := range []string{"1E-3", "0.001", "1E3", "-2.5E2", "6.5E-5", "1E308", "1E-308"} {
		var got float64
		ParseFloat(s, &got)
		want, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("the oracle refused %q: %v", s, err)
		}
		// Repeated multiplication is not correctly rounded, so this is a
		// closeness test rather than an equality one; what it is guarding is
		// that the exponent was applied at all and applied once.
		if want == 0 || got == 0 {
			if got != want {
				t.Errorf("ParseFloat(%q) = %v, want %v", s, got, want)
			}
			continue
		}
		if rel := math.Abs(got-want) / math.Abs(want); rel > 1e-12 {
			t.Errorf("ParseFloat(%q) = %v, want %v (relative error %g)", s, got, want, rel)
		}
	}
}
