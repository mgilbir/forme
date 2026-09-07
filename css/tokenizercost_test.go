package css

import (
	"runtime"
	"strings"
	"testing"
)

// What the tokenizer spends before it produces a token.
//
// §3.3's preprocessing was materialised: the whole input as code points, plus
// each one's byte offset in the original. Four bytes and eight, for every byte
// of the sheet, allocated before the first token — and the offsets are the
// numbers 0, 1, 2 for every stylesheet that is plain ASCII, which is nearly all
// of them.
//
// A sheet of ASCII carrying no carriage return and no null *is* its own
// preprocessed form. The byte index is the code point index, and the one rule
// still to apply — a form feed is a newline — is applied where the character is
// read. So there is nothing to materialise, and nothing is.

// tokenizerBytes is what building a tokenizer over the input allocates, per
// build, averaged over enough of them that the measurement's own noise is small.
func tokenizerBytes(src string) uint64 {
	const runs = 20
	var sink *tokenizer
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		sink = newTokenizer(src)
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(sink)
	return (after.TotalAlloc - before.TotalAlloc) / runs
}

// TestAnAsciiSheetIsNotCopiedToBeRead.
func TestAnAsciiSheetIsNotCopiedToBeRead(t *testing.T) {
	const rule = "p.one > a:hover { color: #abcdef; margin: 1px 2px }\n"
	small := strings.Repeat(rule, 100)
	large := strings.Repeat(rule, 2000)

	a, b := tokenizerBytes(small), tokenizerBytes(large)
	if b > a+1024 {
		t.Errorf("a sheet of %d bytes cost %d to prepare and one of %d cost %d; "+
			"an ASCII sheet is its own preprocessed form and there is nothing "+
			"to copy", len(small), a, len(large), b)
	}

	// The slow path is still there and still right, for a sheet that really
	// does need preprocessing — and it is the same cost it always was, which
	// this states rather than measures away.
	utf := strings.Repeat("p.öne > a:hover { color: #abcdef }\n", 2000)
	if got := tokenizerBytes(utf); got < uint64(len(utf)) {
		t.Errorf("a sheet needing preprocessing cost %d for %d bytes, which is "+
			"less than one byte each: it is not being preprocessed at all",
			got, len(utf))
	}
}

// TestTheTwoPathsReadTheSameSheet is the correctness half: whatever the fast
// path saves, it must read the same characters and report the same offsets.
//
// The same stylesheet is tokenized twice — once as it is, and once with a
// character that forces the slow path appended to a comment, which changes
// nothing about the rules — and the two have to agree token for token.
func TestTheTwoPathsReadTheSameSheet(t *testing.T) {
	const src = `@media print { p.one > a:hover { color: #abcdef; content: "x\"y" } }
		/* a comment */ q::before { quotes: none } .a\.b { width: calc(1px + 2%) }`

	fast := newTokenizer(src)
	if fast.src != nil {
		t.Fatal("the fixture did not take the fast path, so this compares nothing")
	}
	alwaysPreprocess = true
	slow := newTokenizer(src)
	alwaysPreprocess = false
	if slow.src == nil {
		t.Fatal("the slow path was not taken, so this compares one reading with itself")
	}

	for i := 0; ; i++ {
		a, b := fast.token(), slow.token()
		if a != b {
			t.Fatalf("token %d: the fast path read %+v and the slow one %+v", i, a, b)
		}
		if a.Kind == EOF {
			break
		}
	}
}
