package css

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// What reading a stylesheet costs in memory, per byte of it.
//
// A stylesheet can be a token per byte — "a{b:,,,…}" is — and every token
// becomes a ComponentValue in the tree the parser returns, so the tree is the
// floor: nearly a hundred bytes held for each byte of such a sheet, which is
// what the public types are. What these guard is everything above the floor. A
// megabyte of commas allocated 950 megabytes on its way to a 100-megabyte tree
// and held 430 at its peak, because three things each paid for the sheet again:
//
//   - the whole token slice, held beside the tree for the length of the parse,
//     after growing to its size a quarter at a time;
//   - every list in the tree growing to its size the same way, and keeping up
//     to a quarter again in capacity;
//   - and, in a declaration list, the look-ahead for a nested rule reading the
//     declaration into a buffer of its own.
//
// The bounds are on what is allocated rather than on what is held at a moment,
// because allocation is deterministic and a peak is not; what is allocated is
// never less than what is held.

// allocated is what fn allocates, in bytes.
func allocated(fn func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// commaSheetTokens is how many commas the fixtures below are made of: one
// declaration of them is a sheet of a token per byte.
const commaSheetTokens = 1 << 18

// TestATokenIsSeventyTwoBytes is the padding that was a tenth of every parsed
// sheet: Kind and the two flags, each a byte, each given a word of its own by
// the order they were declared in.
func TestATokenIsSeventyTwoBytes(t *testing.T) {
	if got := unsafe.Sizeof(Token{}); got > 72 {
		t.Errorf("a Token is %d bytes; its fields need 72, and the rest is padding "+
			"paid once for every value of every stylesheet", got)
	}
}

// TestTokenizingAllocatesInProportion is Tokenize's slice growing by doubling.
//
// It is sized for one token per two bytes, and a sheet of one per byte outgrows
// that; growing it a quarter at a time from there allocated over four times the
// slice it ended with. Doubling allocates the guess and one slice twice its
// size, so the whole is a slice and a half.
func TestTokenizingAllocatesInProportion(t *testing.T) {
	src := "a{b:" + strings.Repeat(",", commaSheetTokens) + "}"
	var toks []Token
	got := allocated(func() { toks, _ = Tokenize(src) })
	per := float64(got) / float64(len(toks)) / float64(unsafe.Sizeof(Token{}))
	if per > 2.5 {
		t.Errorf("tokenizing %d tokens allocated %d bytes, %.1f times the tokens it "+
			"returned; growing the slice by doubling costs at most twice", len(toks), got, per)
	}
}

// TestANameIsNotCopiedOutOfItsSheet is the text of a token, on the fast path:
// a name or a number with no escape in it, in a sheet that is its own
// preprocessed form, is the bytes it was written as, and is handed back as those
// bytes. Each used to be built in a buffer of its own, which was an allocation
// for every name and every number in the sheet. What is left is the slice the
// tokens go in, which grows a handful of times whatever the sheet's length.
func TestANameIsNotCopiedOutOfItsSheet(t *testing.T) {
	src := strings.Repeat("margin-left 12px 1.5e3 -4 50% #abc @page ", 2000)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	toks, _ := Tokenize(src)
	runtime.ReadMemStats(&after)
	if n := after.Mallocs - before.Mallocs; n > 64 {
		t.Errorf("tokenizing %d tokens made %d allocations; a name or a number on the "+
			"fast path is a slice of the input and allocates nothing", len(toks), n)
	}
}

// TestADeclarationKeepsOnlyItsValue is the other side of a declaration's value
// being left where it lies in its run when it is most of the run, which the
// test below needs: when it is not, the value is copied out, so that the
// trimmed "!important" and white space are not held for as long as the
// declaration is — for a sheet of short declarations, most of what is parsed.
func TestADeclarationKeepsOnlyItsValue(t *testing.T) {
	decls, _, _ := ParseDeclarations("b: c !important; d: e   ")
	for _, d := range decls {
		if len(d.Value) != 1 || cap(d.Value) != 1 {
			t.Errorf("%s: a value of one component is held in %d of capacity", d.Name, cap(d.Value))
		}
	}
}

// TestCountTokensCountsAndStops is the question a caller bounding a sheet's
// cost asks: how many tokens, and — for a sheet past the bound — no further than
// one past it, which is what makes refusing a sheet cost no more than the bound.
func TestCountTokensCountsAndStops(t *testing.T) {
	for _, src := range []string{"", "a", "a{b:c}", `@font-face{src:url("data:x;base64,AAAA")}`,
		"é { b: 1px } /* c */", "\"unterminated"} {
		toks, _ := Tokenize(src)
		want := len(toks) - 1 // not the EOF
		if got := CountTokens(src, 1<<20); got != want {
			t.Errorf("%q: counted %d tokens, the tokenizer makes %d", src, got, want)
		}
	}
	src := strings.Repeat(",", commaSheetTokens)
	if got := CountTokens(src, 100); got != 101 {
		t.Errorf("counting %d tokens against a bound of 100 gave %d; it should stop at 101",
			commaSheetTokens, got)
	}
	if n := allocated(func() { CountTokens(src, commaSheetTokens) }); n > 4096 {
		t.Errorf("counting %d tokens allocated %d bytes; it keeps nothing", commaSheetTokens, n)
	}
}

// TestParsingAllocatesTheTreeAndNotMuchMore is the parser's side of it, for each
// of the entry points that reads a string: the tree it returns, plus the stack
// every list is gathered on before it is copied out once at its own size.
//
// Twice the tree, and a little: the stack's chunks for the one list the sheet
// is, and the tree. A token slice held beside the tree adds three quarters of
// the tree again, lists grown where they live add two to four times it, and a
// declaration read into the look-ahead's buffer adds the token slice back.
func TestParsingAllocatesTheTreeAndNotMuchMore(t *testing.T) {
	commas := strings.Repeat(",", commaSheetTokens)
	cv := float64(unsafe.Sizeof(ComponentValue{}))
	for _, tc := range []struct {
		name  string
		parse func()
	}{
		{"a stylesheet", func() { ParseStylesheet("a{b:" + commas + "}") }},
		{"a list of rules", func() { ParseRules("a{b:" + commas + "}") }},
		{"a list of declarations", func() { ParseDeclarations("b:" + commas) }},
		{"a list of component values", func() { ParseComponentValues(commas) }},
		{"a block", func() { ParseComponentValues("(" + commas + ")") }},
	} {
		got := allocated(tc.parse)
		per := float64(got) / float64(commaSheetTokens) / cv
		if per > 2.5 {
			t.Errorf("%s: parsing %d commas allocated %d bytes, %.1f times the tree of "+
				"them; gathering each list once and copying it out once is twice",
				tc.name, commaSheetTokens, got, per)
		}
	}
}

// TestReadingAParsedBlockCopiesNothing is the parser over component values,
// which is how the cascade reads every rule's declarations and every @media
// block's rules: what it is given is a list already, and a list it hands back
// is a stretch of that list. Gathering them again made a second copy of every
// block the cascade read — for one <style> of a megabyte of commas, half of
// everything reading the document allocated.
func TestReadingAParsedBlockCopiesNothing(t *testing.T) {
	rules, _ := ParseStylesheet("a{b:" + strings.Repeat(",", commaSheetTokens) + "}")
	block := rules[0].Block
	var decls []Declaration
	got := allocated(func() { decls, _, _ = ParseDeclarationValues(block) })
	if len(decls) != 1 || len(decls[0].Value) != commaSheetTokens {
		t.Fatalf("the fixture read as %d declarations; it is one of %d commas",
			len(decls), commaSheetTokens)
	}
	tree := uint64(commaSheetTokens) * uint64(unsafe.Sizeof(ComponentValue{}))
	if got > tree/16 {
		t.Errorf("reading a block of %d values allocated %d bytes, %.2f times the block; "+
			"a declaration read from a block is a stretch of it",
			commaSheetTokens, got, float64(got)/float64(tree))
	}
	for _, rs := range []string{"@media print { a { b: c } d { e: f } }"} {
		outer, _ := ParseStylesheet(rs)
		inner, _ := ParseRulesFromValues(outer[0].Block)
		if len(inner) != 2 || len(inner[1].Prelude) != 2 || inner[1].Prelude[0].Token.Value != "d" {
			t.Errorf("%q: the rules read from the block are %+v", rs, inner)
		}
	}
}

// TestTheLookAheadLeavesTheStreamWhereItWas is the correctness half of the
// look-ahead that is not kept: it reads ahead with a copy of the tokenizer, and
// the parser must then read the same tokens from its own, from where it was —
// and report what is wrong in them exactly once, as the tokenizer alone would.
//
// Each source makes the look-ahead read past the window: the stretch before the
// "{", ";" or "}" that decides it holds more than the one token already pulled.
func TestTheLookAheadLeavesTheStreamWhereItWas(t *testing.T) {
	for _, tc := range []struct {
		src   string
		decls []string
		rules int
	}{
		{"color: \"a\nb; margin: 0", []string{"color", "margin"}, 0},
		{"span \"x\n { color: red } margin: 0", []string{"margin"}, 1},
		{"color: red; & b > c { color: blue }; margin: 0", []string{"color", "margin"}, 1},
	} {
		decls, rules, errs := ParseDeclarations(tc.src)
		var names []string
		for _, d := range decls {
			names = append(names, d.Name)
		}
		if strings.Join(names, ",") != strings.Join(tc.decls, ",") || len(rules) != tc.rules {
			t.Errorf("%q: declarations %q and %d rules, want %q and %d",
				tc.src, names, len(rules), tc.decls, tc.rules)
		}
		_, tokErrs := Tokenize(tc.src)
		for _, te := range tokErrs {
			n := 0
			for _, e := range errs {
				if e == te {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%q: the tokenizer's %v is in the parse's problems %d times", tc.src, te, n)
			}
		}
	}
}
