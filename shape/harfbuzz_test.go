package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Shaping checked against HarfBuzz.
//
// Everything else in this package tests shaping against itself: a fixture built
// by fonttest, read by this package, asserted by this package. That catches a
// reader that contradicts itself and cannot catch a reader that is consistently
// wrong — and consistently wrong is what a misread of a font table looks like,
// because the same misreading writes the fixture and the expectation.
//
// HarfBuzz is outside. It is what browsers, terminals and typesetters actually
// shape with, so where the two differ, the page a reader sees is HarfBuzz's and
// not this package's. That is what makes it ground truth rather than a second
// opinion.
//
// # What it found
//
// It was written after three defects had already reached the tree, all three
// invisible to a test built from this package's own reading:
//
//   - mark attachment took anchors from whichever subtable was read last, so a
//     third of the sample had its accents in the wrong place;
//   - a ligature deleted the marks it stepped over;
//   - pair adjustments were read from 'kern' alone, so every Devanagari conjunct
//     was set at its nominal width, because this font states Devanagari spacing
//     under 'dist' and declares no 'kern' for it at all.
//
// Each was a plausible reading of the specification that no self-consistent test
// would ever have contradicted.
//
// # Why a checked-in file
//
// Regenerating it needs Python, uharfbuzz and the font (make hbshaping); running
// it needs a Go toolchain. An oracle only consultable on a machine with the
// right Python on it is an oracle nobody consults, and this one has to run on
// every change to the shaper.

const harfbuzzDir = "../testdata/harfbuzz"

// harfbuzzCases are the fonts compared and the corpus each is compared over.
//
// One font cannot cover this. The bundled face has no Arabic and no Khmer in it,
// and those are the two shapers with the most to get wrong — cursive joining
// picks one of four forms for every letter from its neighbours, and the Khmer
// model draws a syllable in an order the characters are not written in. Until
// these two arrived neither had ever been compared against anything outside this
// repository.
//
// The extra fonts are Google's Noto builds under the SIL Open Font License, the
// same licence and the same publisher as the bundled face, with their copyright
// notices beside them as that licence requires. They are test data and are not
// embedded in anything this package ships.
var harfbuzzCases = []struct {
	name string
	// font is a path under harfbuzzDir, or empty for the bundled face.
	font, corpus, expected string
	// agree is how many cases must match. Zero means every one of them, which
	// is what a script this package shapes is held to, and what all four are
	// held to now. A number is a ratchet for a script it does not shape yet: it
	// may only go up, so work can land in pieces without any piece being able to
	// undo an earlier one — Javanese carried one at 438 while the Universal
	// Shaping Engine was being written, and reached 894 of 894.
	agree int
	// why explains a ratchet. A bare number in a test is a number nobody can
	// judge; this says what is missing and what would remove it.
	why string
}{
	{name: "latin", corpus: "corpus.txt", expected: "expected.txt"},
	{name: "arabic", font: "fonts/NotoSansArabic.ttf",
		corpus: "arabic.txt", expected: "arabic.expected.txt"},
	{name: "khmer", font: "fonts/NotoSansKhmer.ttf",
		corpus: "khmer.txt", expected: "khmer.expected.txt"},
	{name: "javanese", font: "fonts/NotoSansJavanese.ttf",
		corpus: "javanese.txt", expected: "javanese.expected.txt"},
	// A second script for the universal engine, and the reason there is one:
	// Javanese is what the engine was written against and reached every case,
	// while Balinese — its close relative — was wrong in 66 of 764 at that
	// point. One script cannot tell a general model from an overfitted one.
	{name: "balinese", font: "fonts/NotoSansBalinese.ttf",
		corpus: "balinese.txt", expected: "balinese.expected.txt"},
	// The largest font here, and the one that earns its size: it declares 1190
	// lookups and states one of them in 738 subtables, which is how three
	// silent truncations were found.
	{name: "tibetan", font: "fonts/NotoSerifTibetan.ttf",
		corpus: "tibetan.txt", expected: "tibetan.expected.txt"},
}

// TestShapingAgreesWithHarfBuzz compares every case in every corpus.
func TestShapingAgreesWithHarfBuzz(t *testing.T) {
	for _, tc := range harfbuzzCases {
		t.Run(tc.name, func(t *testing.T) {
			corpus, expected, header := readHarfBuzzGolden(t, tc.corpus, tc.expected)
			f := harfbuzzFace(t, tc.font, header)

			if len(corpus) != len(expected) {
				t.Fatalf("%d strings in %s and %d lines of expectations", len(corpus), tc.corpus, len(expected))
			}
			if len(corpus) == 0 {
				t.Fatalf("%s is empty, so this proves nothing", tc.corpus)
			}

			known := deliberateDifferences[tc.name]
			unseen := map[string]bool{}
			for s := range known {
				unseen[s] = true
			}
			var differing, expectedDiffs int
			for i, s := range corpus {
				glyphs, _ := f.ShapeGlyphs(s)
				same, why := sameAsHarfBuzz(f, glyphs, expected[i])
				if _, listed := known[s]; listed {
					delete(unseen, s)
					if same {
						t.Errorf("%s is listed as a deliberate difference and now agrees with "+
							"HarfBuzz.\nRemove it: a stale exception is a hole in this test.",
							describeRunes(s))
						continue
					}
					expectedDiffs++
					continue
				}
				if !same {
					differing++
					if differing <= 40 && tc.agree == 0 {
						t.Errorf("%s\n  %s\n  pdf0     %s\n  harfbuzz %s",
							describeRunes(s), why, describeGlyphs(glyphs), describeExpected(f, expected[i]))
					}
				}
			}
			if differing > 40 && tc.agree == 0 {
				t.Errorf("... and %d more", differing-40)
			}
			for s := range unseen {
				t.Errorf("%s is listed as a deliberate difference but is not in the corpus", describeRunes(s))
			}
			agreed := len(corpus) - differing - expectedDiffs
			t.Logf("%d of %d agree, %d deliberate differences (harfbuzz %s)",
				agreed, len(corpus), expectedDiffs, header["harfbuzz"])
			if tc.agree > 0 && agreed < tc.agree {
				t.Errorf("%d of %d agree and the baseline is %d.\n%s",
					agreed, len(corpus), tc.agree, tc.why)
			}
			if tc.agree > 0 && agreed > tc.agree {
				t.Errorf("%d of %d agree and the baseline is %d — more than it was.\n"+
					"Raise the baseline to %d so that what has been fixed cannot come back.",
					agreed, len(corpus), tc.agree, agreed)
			}
		})
	}
}

// harfbuzzFace loads the face a case is compared over, and refuses to run
// against a font the expectations were not generated from.
//
// That is a hard stop rather than a skip: every expectation is about a glyph
// index, so a different font would leave this asserting yesterday's answers
// about today's glyphs and every one of them would be about the wrong glyph.
func harfbuzzFace(t *testing.T, path string, header map[string]string) *Face {
	t.Helper()
	data := notoSansBytes(t)
	if path != "" {
		var err error
		data, err = os.ReadFile(filepath.Join(harfbuzzDir, path))
		if err != nil {
			t.Fatalf("reading the font: %v\nRun `make hbshaping` after fetching it.", err)
		}
	}
	sum := sha256.Sum256(data)
	if got, want := hex.EncodeToString(sum[:]), header["font-sha256"]; got != want {
		t.Fatalf("the expectations were generated against font %s and this one is %s.\n"+
			"Run `make hbshaping` to regenerate them.", want, got)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading the font: %v", err)
	}
	return f
}

// deliberateDifferences are the cases where this package answers something other
// than HarfBuzz on purpose, per corpus, each with the reason.
//
// The list is checked in both directions: a case in it that agrees with HarfBuzz
// fails, and so does one that is not in the corpus. An exception that cannot go
// stale is a documented decision; one that can is a hole.
//
// # Two entries, and what each took to earn it
//
// Neither is a disagreement about shaping, and neither is what it was first
// written down as. Each was re-opened against the specification rather than
// against whichever engine agreed with this one, and each came back different.
// CoreText, through the harness in testdata/coretext, agrees with this package
// on both — but that is worth less than it looks and is not why either is here.
// CoreText is a layout engine rather than a shaper, and testdata/coretext says
// itself that it is the wrong instrument for measuring positions.
//
// # An invisible character that the font gave a width
//
// Noto Sans Arabic draws nothing for U+061C ARABIC LETTER MARK and gives it a
// glyph 600 units wide. The hamza written after it comes out at 250 here and at
// 850 from the oracle, and the 600 is that glyph.
//
// It is not a difference of policy. HarfBuzz answers this string two ways:
//
//	default                       hamza at 250, total advance 760
//	REMOVE_DEFAULT_IGNORABLES     hamza at 850, total advance 760
//
// The total is 760 either way, so HarfBuzz agrees with itself that the character
// takes no width — and then, having deleted the glyph rather than hidden it,
// leaves the mark that was positioned across it 600 units out. Its own default
// answer is this package's answer, to the unit, and so is the answer for the
// string with the character taken out.
//
// So what the oracle reports here is HarfBuzz not fixing up a dependent offset
// on one of its two paths. The entry stays because shape.py asks for that path
// on purpose — this package removes such characters rather than mapping them to
// an invisible glyph, and REMOVE_DEFAULT_IGNORABLES is the comparable policy —
// but what it is comparing against is a HarfBuzz inconsistency and not a reading
// of Unicode. Unicode does not settle it: what the display rule asks is that the
// character have no glyph of its own, "although they may have an effect on the
// display of other characters", and it says nothing about advance width.
//
// # A mark whose target moved after it was attached
//
// Five units of x on the last mark of one Tibetan string, and the mechanism is
// now known rather than guessed at.
//
// Lookup 19 — mark-to-mark, in blwm — attaches U+0F37 (anchor -135,0) to
// U+0F71 (anchor 163,140), a delta of (298,140). With the target at (129,-294)
// that puts the mark at (427,-154), and *both engines produce exactly that*.
// Lookup 21 then re-attaches the target and moves it by (-5,-887). HarfBuzz
// follows that move in x and not in y; this package follows it in neither.
//
// Three controls locate it. Take lookup 19 out of the font and both engines
// answer (425,40), which is what the mark-to-base anchors in lookup 18 predict on
// their own. Take lookup 21 out and both answer (427,-154). And move the anchor
// lookup 21 attaches the target by: +100 in x moves HarfBuzz's mark by exactly
// +100, +100 in y moves it not at all. So HarfBuzz propagates the x of that move
// and not the y, and it is the only engine that propagates either.
//
// What the specification asks for is a third answer. GPOS says the attachment
// points coincide, and after lookup 21 the target's anchor is at (287,-1041), so
// a mark whose anchor coincided with it would sit at (422,-1041). That number is
// not new: it is the 887 an earlier attempt recorded, which means that attempt
// implemented the specification correctly and found the specification agreeing
// with nobody.
//
// CoreText settles it, and on the axis nobody had measured. The earlier round
// compared only the x and reported 427, 427 and 422. Asked for the whole line, it
// answers (-127,-294) (204,-316) (-70,-835) (124,-1181) for the stack and
// (427,-154) for the mark — identical to this package on *both* axes and on every
// glyph. All three engines agree the y does not propagate; only HarfBuzz
// propagates the x.
//
// So this is listed rather than fixed because this package is not the odd one
// out. It resolves an attachment against the state at the moment it is applied,
// in both axes, and so does CoreText. Matching HarfBuzz would mean propagating x
// while not propagating y — an asymmetry no text states and no other engine has —
// and would trade agreement with CoreText for agreement with neither. Following
// GPOS literally moves the mark 887 units and matches nobody at all.
//
// # The thirty-seven that left
//
// It held thirty-seven: a character nothing is drawn for, written inside a
// syllable. This package removed every such character before shaping, so the
// syllable stayed whole and the conjunct formed; HarfBuzz keeps them, so the
// syllable breaks and the orphaned virama gets a dotted circle.
//
// The reason given was that Unicode "defines the property as characters that
// should be ignored in rendering". It does not. Section 5.21 enumerates the six
// processes such a character is to be ignored in — text segmentation, line
// breaking, cursive joining, identifiers, searching and sorting, and display —
// and shaping is not among them, while cursive joining is named explicitly, so
// the list is not illustrative. What the display rule asks is that the character
// have no glyph of its own, "although they may have an effect on the display of
// other characters", which is satisfied by drawing nothing for it rather than by
// removing it.
//
// And nothing else agreed with this package. CoreText, asked on a Mac through
// the harness in testdata/coretext, breaks the syllable too — for several of
// these it emits a zero-advance glyph of its own and breaks the syllable anyway,
// which is HarfBuzz's model reached independently. Two engines out of two.
//
// So the removal moved to the other side of the shaper: a syllabic run now keeps
// them, the syllable model is allowed to be broken by them, and the shaper drops
// them once they have said which cluster they broke. Every entry here went with
// the change.
//
// An empty list is worth more than a documented one. It means every difference
// this comparison reports is a defect.
var deliberateDifferences = map[string]map[string]string{
	"latin": {},
	"arabic": {
		"\u063D\u061C\u0655": "an invisible character the font gave a width to: " +
			"HarfBuzz deletes the glyph under REMOVE_DEFAULT_IGNORABLES without " +
			"taking the 600 units back off the mark positioned across it, and " +
			"answers 250 rather than 850 on its own default path",
	},
	"khmer":    {},
	"javanese": {},
	"balinese": {},
	"tibetan": {
		"\u0F52\u0F8F\u0FAD\u0F91\u0F73\u0F37": "a mark whose attachment target lookup 21 " +
			"moves by (-5,-887) after lookup 19 attached it: HarfBuzz follows the x " +
			"and not the y, this package and CoreText follow neither on either axis, " +
			"and GPOS as written asks for both and so agrees with nobody",
	},
}

// TestTheHarfBuzzOracleHasTeeth is the guard on the guard.
//
// A comparison that reads no cases, or compares nothing about them, passes
// exactly as loudly as one that checks five thousand. This shapes a string with
// a deliberately wrong answer and requires the comparison to say so.
func TestTheHarfBuzzOracleHasTeeth(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	glyphs, _ := f.ShapeGlyphs("AV")
	if len(glyphs) != 2 {
		t.Fatalf("the fixture shaped to %d glyphs", len(glyphs))
	}
	right := []hbGlyph{
		{gid: glyphs[0].GID, adv: int(glyphs[0].XAdvance)},
		{gid: glyphs[1].GID, adv: int(glyphs[1].XAdvance)},
	}
	if same, why := sameAsHarfBuzz(f, glyphs, right); !same {
		t.Fatalf("the comparison rejects a correct answer: %s", why)
	}
	for _, tc := range []struct {
		mutate func([]hbGlyph) []hbGlyph
		why    string
	}{
		{func(g []hbGlyph) []hbGlyph { g[0].gid++; return g }, "a different glyph"},
		{func(g []hbGlyph) []hbGlyph { g[0].adv++; return g }, "a different advance"},
		{func(g []hbGlyph) []hbGlyph { g[1].dx = 5; return g }, "a different offset"},
		{func(g []hbGlyph) []hbGlyph { g[1].dy = -5; return g }, "a different vertical offset"},
		{func(g []hbGlyph) []hbGlyph { return g[:1] }, "one glyph too few"},
		{func(g []hbGlyph) []hbGlyph { return append(g, hbGlyph{gid: 1}) }, "one glyph too many"},
	} {
		wrong := append([]hbGlyph(nil), right...)
		if same, _ := sameAsHarfBuzz(f, glyphs, tc.mutate(wrong)); same {
			t.Errorf("the comparison accepts %s", tc.why)
		}
	}
}

// hbGlyph is one glyph as the expectations state it: font units throughout,
// which is what HarfBuzz reports and not what this package does.
type hbGlyph struct {
	gid, adv, dx, dy int
}

// sameAsHarfBuzz compares a shaped run against the expectation, converting the
// expectation into this package's units rather than the other way about — a
// comparison that rounded this package's answer could hide a real difference of
// less than a unit.
func sameAsHarfBuzz(f *Face, got []Glyph, want []hbGlyph) (bool, string) {
	if len(got) != len(want) {
		return false, fmt.Sprintf("%d glyphs, want %d", len(got), len(want))
	}
	for i := range got {
		switch {
		case got[i].GID != want[i].gid:
			return false, fmt.Sprintf("glyph %d is %d, want %d", i, got[i].GID, want[i].gid)
		case got[i].XAdvance != f.scale(want[i].adv):
			return false, fmt.Sprintf("glyph %d advances %v, want %v",
				i, got[i].XAdvance, f.scale(want[i].adv))
		case got[i].XOffset != f.scale(want[i].dx) || got[i].YOffset != f.scale(want[i].dy):
			return false, fmt.Sprintf("glyph %d is placed at (%v, %v), want (%v, %v)",
				i, got[i].XOffset, got[i].YOffset, f.scale(want[i].dx), f.scale(want[i].dy))
		}
	}
	return true, ""
}

// readHarfBuzzGolden reads the corpus and the expectations, together with the
// header that says what produced them.
func readHarfBuzzGolden(t *testing.T, corpusName, expectedName string) (corpus []string, expected [][]hbGlyph, header map[string]string) {
	t.Helper()
	corpus = readNonEmptyLines(t, filepath.Join(harfbuzzDir, corpusName))

	path := filepath.Join(harfbuzzDir, expectedName)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbshaping` to generate it.", path, err)
	}
	defer file.Close()

	header = map[string]string{}
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 0; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			if key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(text, "#")), " "); ok {
				header[key] = value
			}
			continue
		}
		glyphs, err := parseExpectedGlyphs(text)
		if err != nil {
			t.Fatalf("%s:%d: %v", path, line+1, err)
		}
		expected = append(expected, glyphs)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if header["font-sha256"] == "" {
		t.Fatalf("%s has no font-sha256 line, so there is nothing tying it to a font", path)
	}
	return corpus, expected, header
}

func parseExpectedGlyphs(line string) ([]hbGlyph, error) {
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	var out []hbGlyph
	for _, field := range strings.Fields(line) {
		parts := strings.Split(field, ",")
		if len(parts) != 2 && len(parts) != 4 {
			return nil, fmt.Errorf("%q has %d parts, want 2 or 4", field, len(parts))
		}
		nums := make([]int, len(parts))
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("%q: %v", field, err)
			}
			nums[i] = n
		}
		g := hbGlyph{gid: nums[0], adv: nums[1]}
		if len(nums) == 4 {
			g.dx, g.dy = nums[2], nums[3]
		}
		out = append(out, g)
	}
	return out, nil
}

func readNonEmptyLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	defer file.Close()
	var out []string
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return out
}

// describeRunes names a string by its code points as well as its text, because
// most of the corpus is invisible in a terminal and several cases differ only by
// a character that has no shape.
func describeRunes(s string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q [", s)
	for i, r := range []rune(s) {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "U+%04X", r)
	}
	b.WriteByte(']')
	return b.String()
}

func describeGlyphs(glyphs []Glyph) string {
	var parts []string
	for _, g := range glyphs {
		if g.XOffset != 0 || g.YOffset != 0 {
			parts = append(parts, fmt.Sprintf("%d,%v@%v,%v", g.GID, g.XAdvance, g.XOffset, g.YOffset))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d,%v", g.GID, g.XAdvance))
	}
	return strings.Join(parts, " ")
}

func describeExpected(f *Face, glyphs []hbGlyph) string {
	var parts []string
	for _, g := range glyphs {
		if g.dx != 0 || g.dy != 0 {
			parts = append(parts, fmt.Sprintf("%d,%v@%v,%v", g.gid, f.scale(g.adv), f.scale(g.dx), f.scale(g.dy)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d,%v", g.gid, f.scale(g.adv)))
	}
	return strings.Join(parts, " ")
}
