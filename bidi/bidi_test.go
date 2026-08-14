package bidi

import (
	"testing"
)

// The bidirectional algorithm as the shaping pipeline uses it.
//
// bidi_conformance_test.go proves the algorithm against Unicode's own six
// hundred thousand cases; this file proves the wiring — that a shaped run comes
// back in the order it is drawn, that shaping still saw the text in the order it
// is written, and that the positioning a font states survives the reversal.
//
// Hebrew is used wherever the point is direction alone, because Hebrew does not
// join and so nothing else is going on; Arabic wherever the point is that
// joining and direction are separate stages.

const (
	alefHeb = 0x05D0 // HEBREW LETTER ALEF
	betHeb  = 0x05D1
	gimel   = 0x05D2
	beh     = 0x0628 // ARABIC LETTER BEH
)

// TestBidiClassTableNamesEveryClass guards the generated table against a Unicode
// release that renames or withdraws a property value.
//
// cmd/genbidi fails if the data does not use a value the algorithm names, but
// the generator is only run by hand. This is the same check on the committed
// file, so that a table regenerated from the wrong version of the UCD — or by
// hand, which the header forbids and nothing enforces — cannot leave a branch of
// the algorithm silently unreachable.
// TestBidiClassTableNamesEveryClass guards the generated table against a Unicode
// release that renames or withdraws a property value.
//
// cmd/genbidi fails if the data does not use a value the algorithm names, but
// the generator is only run by hand. This is the same check on the committed
// file, so that a table regenerated from the wrong version of the UCD — or by
// hand, which the header forbids and nothing enforces — cannot leave a branch of
// the algorithm silently unreachable.
func TestBidiClassTableNamesEveryClass(t *testing.T) {
	named := map[Class]string{
		R: "R", AL: "AL", EN: "EN", ES: "ES", ET: "ET",
		AN: "AN", CS: "CS", NSM: "NSM", BN: "BN", B: "B",
		S: "S", WS: "WS", ON: "ON", LRE: "LRE", RLE: "RLE",
		LRO: "LRO", RLO: "RLO", PDF: "PDF", LRI: "LRI",
		RLI: "RLI", FSI: "FSI", PDI: "PDI",
	}
	// L is not in the list: it is the default, and the table says it by
	// leaving a character out.
	for _, r := range classRanges {
		delete(named, r.class)
	}
	for _, name := range named {
		t.Errorf("no character in the table has Bidi_Class %s", name)
	}
	if len(brackets) == 0 {
		t.Error("the bracket table is empty; rule N0 has nothing to work with")
	}
	if len(mirrors) == 0 {
		t.Error("the mirroring table is empty; rule L4 has nothing to work with")
	}
}

// TestBidiClassesAreUnicodes pins the generated table against characters the
// rest of this rests on, including one that is not a character at all.
// TestBidiClassesAreUnicodes pins the generated table against characters the
// rest of this rests on, including one that is not a character at all.
func TestBidiClassesAreUnicodes(t *testing.T) {
	cases := map[rune]Class{
		'a':     L,
		alefHeb: R,
		beh:     AL,
		'5':     EN,
		0x0663:  AN, // ARABIC-INDIC DIGIT THREE
		' ':     WS,
		'(':     ON,
		0x0301:  NSM, // COMBINING ACUTE ACCENT
		0x200D:  BN,  // ZERO WIDTH JOINER
		0x202B:  RLE,
		0x2067:  RLI,
		0x2069:  PDI,
		// U+0590 is unassigned and always has been. It is right-to-left all the
		// same, because it sits in the Hebrew block — the block defaults are in
		// the table, and a character Unicode has not defined yet still has to be
		// laid out the way its neighbours will be.
		0x0590: R,
	}
	for r, want := range cases {
		if got := ClassOf(r); got != want {
			t.Errorf("ClassOf(U+%04X) = %d, want %d", r, got, want)
		}
	}
}

// TestBidiBracketAndMirrorTables pins the two tables rules N0 and L4 read.
// TestBidiBracketAndMirrorTables pins the two tables rules N0 and L4 read.
func TestBidiBracketAndMirrorTables(t *testing.T) {
	paired, open, ok := BracketOf('[')
	if !ok || !open || paired != ']' {
		t.Errorf("BracketOf('[') = (%q, open=%v, ok=%v), want (']', true, true)", paired, open, ok)
	}
	paired, open, ok = BracketOf('}')
	if !ok || open || paired != '{' {
		t.Errorf("BracketOf('}') = (%q, open=%v, ok=%v), want ('{', false, true)", paired, open, ok)
	}
	if _, _, ok := BracketOf('a'); ok {
		t.Error("a letter was reported as a bracket")
	}
	if m, ok := MirrorOf('('); !ok || m != ')' {
		t.Errorf("MirrorOf('(') = (%q, %v), want (')', true)", m, ok)
	}
	if _, ok := MirrorOf('a'); ok {
		t.Error("a letter was reported as mirrored")
	}
}

// TestBidiRunsSplitAndReorder is the algorithm's output in the form the shaping
// pipeline consumes it: stretches of one direction, in the order they are drawn.
// TestBidiRunsSplitAndReorder is the algorithm's output in the form the shaping
// pipeline consumes it: stretches of one direction, in the order they are drawn.
func TestBidiRunsSplitAndReorder(t *testing.T) {
	// A right-to-left sentence with a left-to-right phrase inside it. The
	// phrase is one level deeper, and the whole line reverses around it.
	text := string([]rune{alefHeb, betHeb, gimel}) + " abc " + string([]rune{gimel, betHeb, alefHeb})
	runs := LogicalRuns(text)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3: %v", len(runs), runs)
	}
	wantLevels := []int{1, 2, 1}
	for i, r := range runs {
		if r.Level != wantLevels[i] {
			t.Errorf("run %d is at level %d, want %d", i, r.Level, wantLevels[i])
		}
	}
	levels := []int{runs[0].Level, runs[1].Level, runs[2].Level}
	order := VisualOrder(levels)
	// The line is right-to-left, so the last-written run is drawn first; the
	// Latin phrase inside it keeps its own direction.
	want := []int{2, 1, 0}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("visual order %v, want %v", order, want)
		}
	}
}

// TestBidiRunsOfPlainTextAreOne pins the cost of all this for text that needs
// none of it: one run, level zero, no reordering.
// TestBidiRunsOfPlainTextAreOne pins the cost of all this for text that needs
// none of it: one run, level zero, no reordering.
func TestBidiRunsOfPlainTextAreOne(t *testing.T) {
	runs := LogicalRuns("Hello, world!")
	if len(runs) != 1 || runs[0].Level != 0 || runs[0].RTL() {
		t.Fatalf("plain Latin gave %v, want one left-to-right run", runs)
	}
	if runs := LogicalRuns(""); runs != nil {
		t.Errorf("the empty string gave %v, want nothing", runs)
	}
}

// TestBidiShortcutAgreesWithTheAlgorithm is the only thing that makes the
// shortcut in LogicalRuns safe to have.
//
// Text with nothing right-to-left in it is answered without running the
// algorithm at all, because the pipeline asks the question of every string it
// sets and most of them are Latin. A shortcut that disagreed with the algorithm
// would be a silent wrong answer on exactly the text nobody thinks to check —
// so every case the shortcut claims is checked against the algorithm itself.
// TestBidiShortcutAgreesWithTheAlgorithm is the only thing that makes the
// shortcut in LogicalRuns safe to have.
//
// Text with nothing right-to-left in it is answered without running the
// algorithm at all, because the pipeline asks the question of every string it
// sets and most of them are Latin. A shortcut that disagreed with the algorithm
// would be a silent wrong answer on exactly the text nobody thinks to check —
// so every case the shortcut claims is checked against the algorithm itself.
func TestBidiShortcutAgreesWithTheAlgorithm(t *testing.T) {
	cases := []string{
		"", "a", "Hello, world!", "café — naïve",
		"1,234.56", "+42%", "$1.5m", "  leading and trailing  ",
		"line\nbreak", "tab\tseparated", "áb", // a combining mark
		"​zero width space", "emoji \U0001F600 too", // neither is directional
		"日本語のテキスト", "Ελληνικά", "Кириллица",
		"a‍b", // a zero-width joiner: boundary neutral, still not directional
	}
	for _, s := range cases {
		short := LogicalRuns(s)
		if s != "" && NeedsAlgorithm(s) {
			t.Errorf("%q was not taken by the shortcut; the case proves nothing", s)
			continue
		}
		var full []Run
		if s != "" {
			full = ResolveRuns(s)
		}
		if len(short) != len(full) {
			t.Errorf("%q: the shortcut gives %v and the algorithm %v", s, short, full)
			continue
		}
		for i := range full {
			if short[i] != full[i] {
				t.Errorf("%q: the shortcut gives %v and the algorithm %v", s, short, full)
				break
			}
		}
	}

	// And the other half: text that does need the algorithm must not be taken by
	// the shortcut.
	for _, s := range []string{
		string(rune(alefHeb)), "a" + string(rune(beh)), "‏", // right-to-left mark
		"a‫b‬", // an explicit embedding
		"a⁧b⁩", // an isolate
		"١٢",   // Arabic-Indic digits, which are Arabic numbers
	} {
		if !NeedsAlgorithm(s) {
			t.Errorf("%q was taken by the shortcut, and it must not be", s)
		}
	}
}

// sameInts, since a reordering is only ever checked against a whole expected
// order.
func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHebrewIsEmittedInVisualOrder is the defect this all exists to fix. A
// text-showing operator moves the pen left to right, so a right-to-left word has
// to arrive with its last letter first — otherwise the word is drawn backwards,
// which a reader of the script notices immediately and nobody else does.
// Anchors for a right-to-left cursive fixture. They are the mirror image of the
// Latin ones in cursive_test.go, and deliberately so: in a script written right
// to left the stroke *leaves* a letter at its left edge and *arrives* at the
// next one's right edge, so an exit is at a small x and an entry at a large one.
// TestCursiveJointsSurviveTheReversal is the positioning half of direction, and
// the part a reversal quietly breaks.
//
// Cursive attachment says where two letters meet. The rule is stated over the
// text as written, but the arithmetic that carries it out is about where the pen
// will be — and the pen meets a right-to-left run from the other end. Applying
// the left-to-right form of it and then reversing leaves every letter displaced
// by the width of its neighbour: the strokes no longer meet, which is what makes
// joined script look joined.
