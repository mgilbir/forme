package shape

import (
	"testing"
	"time"
)

// callSubr is the Type 2 encoding of "call local subroutine k".
//
// A subroutine index is biased — the specification makes the offset depend on
// how many there are, so that the commonest indices encode in one byte — and
// below 1240 subroutines the bias is 107. A number from -107 to 107 is one
// byte, v+139, so the call is two bytes for any of the first 215 subroutines.
func callSubr(k int) []byte { return []byte{byte(k - 107 + 139), 10} }

const (
	csReturn  = 11
	csEndchar = 14
)

// csPush is the one-byte encoding of a number from -107 to 107.
func csPush(v int) byte { return byte(v + 139) }

// csSeac is an endchar with four operands, which is how a Type 2 charstring
// says "draw the accented character built from these two". Reaching it is the
// only thing cffSeac reports, so it is what tells a walk that finished from one
// that was stopped.
func csSeac(bchar, achar int) []byte {
	return []byte{csPush(0), csPush(0), csPush(bchar), csPush(achar), csEndchar}
}

// TestACharstringThatNestsPastTheDepthIsRefused is the bound on how deeply one
// charstring may call into another.
//
// A font writes its own subroutines and nothing in the format stops one from
// calling another without end. The walk here keeps its own stack rather than
// the machine's, so the fault is not a crash — it is a walk that does not
// finish, and the depth is what ends it.
//
// The seac at the bottom is what makes this a test. Without one, a walk stopped
// by the depth and a walk that simply found nothing both answer "no seac", and
// an assertion on that cannot tell them apart — which is how the first version
// of this passed with the bound raised. With one, the answer is "no" only
// because the depth stopped the walk, and raising the bound turns it into "yes".
//
// Nothing had made a font that reaches it: the shape suite passes with
// maxCharstringDepth raised, because every CFF fixture in it nests one or two
// deep.
func TestACharstringThatNestsPastTheDepthIsRefused(t *testing.T) {
	const bchar, achar = 65, 66
	// chain builds n subroutines where each calls the next and the last names a
	// seac. The indices are its own: a subroutine names another by position in
	// the table it is in, so a shorter chain is a different table and not a
	// slice of a longer one.
	chain := func(n int) [][]byte {
		local := make([][]byte, n)
		for i := range local {
			if i == n-1 {
				local[i] = csSeac(bchar, achar)
				continue
			}
			local[i] = append(callSubr(i+1), csReturn)
		}
		return local
	}
	code := append(callSubr(0), csReturn)

	// The fixture has to be one the walk would otherwise follow, or the test is
	// asserting nothing. Shallow enough to be read, it finds the seac.
	if b, a, ok := cffSeac(code, chain(4), nil, fullBudget()); !ok || b != bchar || a != achar {
		t.Fatalf("the same subroutines four deep gave %d, %d, %v; the fixture does "+
			"not reach what the depth is being asked about", b, a, ok)
	}

	const subrs = 20
	if b, a, ok := cffSeac(code, chain(subrs), nil, fullBudget()); ok {
		t.Errorf("a charstring nesting %d subroutines deep was followed to a seac "+
			"naming %d and %d; past the depth the walk has to stop", subrs, b, a)
	}
}

// TestACharstringThatReturnsAndCallsAgainIsBounded is the other half, and the
// half the depth cannot see.
//
// maxCharstringOps says why it is there: "A crafted font can write a subroutine
// that calls itself, and the depth bound alone does not stop a walk that returns
// and calls again." A subroutine calling the next one thirty-two times, seven
// levels down, never nests past the depth and is thirty billion operators — from
// a font of a few hundred bytes.
//
// The size matters and the first version of this got it wrong: eight calls seven
// levels down is two million operators, which finishes in milliseconds with the
// bound raised, so the test passed and proved nothing. A bomb has to be big
// enough that the bound is the only reason it ends.
func TestACharstringThatReturnsAndCallsAgainIsBounded(t *testing.T) {
	const levels, fanout = 7, 32
	local := make([][]byte, levels+1)
	local[levels] = []byte{csReturn}
	for i := levels - 1; i >= 0; i-- {
		var body []byte
		for j := 0; j < fanout; j++ {
			body = append(body, callSubr(i+1)...)
		}
		local[i] = append(body, csReturn)
	}

	done := make(chan bool, 1)
	go func() {
		_, _, ok := cffSeac(append(callSubr(0), csReturn), local, nil, fullBudget())
		done <- ok
	}()
	select {
	case <-done:
		// Either answer is fine: what matters is that it finished at all.
	case <-time.After(20 * time.Second):
		t.Fatal("a charstring whose subroutines call thirty-two at a time, seven " +
			"levels down, did not finish; the depth bounds the nesting and only the " +
			"operator count bounds the walk")
	}
}
