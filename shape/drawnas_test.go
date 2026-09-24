package shape

import (
	"bytes"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// What is measured is what is drawn, for a character the face draws as its
// decomposition. Measure took such a character apart; the path that sets a face
// by character code, and Encode, did not, and drew it as a space or a .notdef.
// Audit C180.

// asciiGlyphs is printable ASCII, which is enough of WinAnsi for a face to be
// embedded as a simple font, and a combining acute.
func asciiGlyphs() []fonttest.Glyph {
	var glyphs []fonttest.Glyph
	for r := rune(' '); r <= '~'; r++ {
		glyphs = append(glyphs, fonttest.Glyph{Rune: r, Advance: 400 + int(r), HasShape: r != ' '})
	}
	return append(glyphs, fonttest.Glyph{Rune: 0x0301, Advance: 0, HasShape: true})
}

// TestEveryFaceDrawsWhatItMeasures: U+212A KELVIN SIGN is a singleton
// decomposition to K, and é is e and a combining acute; none of the faces here
// has the character, and each has what it decomposes into.
func TestEveryFaceDrawsWhatItMeasures(t *testing.T) {
	helvetica, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Ascii", Glyphs: asciiGlyphs()})
	simple, err := LoadSimple(data)
	if err != nil {
		t.Fatalf("loading the fixture as a simple face: %v", err)
	}
	composite, err := Load(data)
	if err != nil {
		t.Fatalf("loading the fixture: %v", err)
	}
	for _, tc := range []struct {
		face    string
		f       *Face
		s, want string
	}{
		{"Helvetica", helvetica, "K", "K"},
		{"a simple face", simple, "K", "K"},
		{"a composite face", composite, "K", "K"},
		{"a composite face", composite, "é", "é"},
	} {
		if _, ok := tc.f.GlyphID([]rune(tc.s)[0]); ok {
			t.Fatalf("%s has %q itself; the fixture asks nothing", tc.face, tc.s)
		}
		if got, want := tc.f.Measure(tc.s, 1000), tc.f.Measure(tc.want, 1000); got != want {
			t.Errorf("%s measures %q at %v, and %q at %v", tc.face, tc.s, got, tc.want, want)
		}
		codes, missing := tc.f.Encode(tc.s)
		wantCodes, _ := tc.f.Encode(tc.want)
		if missing != 0 || !bytes.Equal(codes, wantCodes) {
			t.Errorf("%s encodes %q as % x with %d missing, want % x as %q is",
				tc.face, tc.s, codes, missing, wantCodes, tc.want)
		}
		if tc.f.composite() {
			continue // shaped, and normalize has always done this there
		}
		glyphs, missing := tc.f.ShapeGlyphs(tc.s)
		wantGlyphs, _ := tc.f.ShapeGlyphs(tc.want)
		if missing != 0 || len(glyphs) != len(wantGlyphs) {
			t.Errorf("%s shapes %q to %v with %d missing, want %v", tc.face, tc.s,
				glyphs, missing, wantGlyphs)
			continue
		}
		for i := range glyphs {
			if glyphs[i].GID != wantGlyphs[i].GID || glyphs[i].XAdvance != wantGlyphs[i].XAdvance {
				t.Errorf("%s shapes %q to %v, want %v", tc.face, tc.s, glyphs, wantGlyphs)
				break
			}
		}
	}
}

// TestTheStackAsksWhatTheShaperAsks is audit C184. Stack chose a face by
// whether it mapped every character of a unit, where the shaper draws a
// character by its decomposition and draws nothing for a joiner.
func TestTheStackAsksWhatTheShaperAsks(t *testing.T) {
	ascii, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Ascii", Glyphs: asciiGlyphs()}))
	if err != nil {
		t.Fatal(err)
	}
	precomposed, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Precomposed",
		Glyphs: []fonttest.Glyph{{Rune: 0x00E9, Advance: 500, HasShape: true}}}))
	if err != nil {
		t.Fatal(err)
	}
	// é in the first face, which draws it as e and an acute, and not the
	// second, which has it whole: the unit is the first face's, as the letters
	// either side of it are.
	runs, missing := NewStack(ascii, precomposed).ShapeRuns("céd")
	if missing != 0 || len(runs) != 1 || runs[0].Face != ascii {
		t.Errorf("c, é, d over a face drawing é decomposed: %d runs, %d missing; "+
			"want one run in that face", len(runs), missing)
	}

	// A zero width joiner between two Arabic letters is set with them, so the
	// letters join: beh.fina then beh.init, drawn right to left.
	arabic := arabicFace(t)
	runs, missing = NewStack(latinFace(t), arabic).ShapeRuns(string([]rune{beh, 0x200D, beh}))
	if missing != 0 || len(runs) != 1 || runs[0].Face != arabic {
		t.Fatalf("beh, ZWJ, beh: %d runs, %d missing; want one run in the Arabic face",
			len(runs), missing)
	}
	var gids []int
	for _, g := range runs[0].Glyphs {
		gids = append(gids, g.GID)
	}
	if !sameGIDs(gids, []int{4, 2}) {
		t.Errorf("beh, ZWJ, beh drew %v, want the joined forms [4 2]", gids)
	}
	// A joiner that opens the string is set in the face of what follows it.
	runs, _ = NewStack(latinFace(t), arabic).ShapeRuns(string([]rune{0x200D, beh, beh}))
	if len(runs) != 1 || runs[0].Face != arabic {
		t.Errorf("ZWJ, beh, beh: %d runs; want one run in the Arabic face", len(runs))
	}
}

// TestAHalfDrawableDecompositionIsMissing is the other side: a face with e and
// no combining acute cannot draw é as its decomposition, and says so rather
// than drawing half of it.
func TestAHalfDrawableDecompositionIsMissing(t *testing.T) {
	var glyphs []fonttest.Glyph
	for _, g := range asciiGlyphs() {
		if g.Rune != 0x0301 {
			glyphs = append(glyphs, g)
		}
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "NoAcute", Glyphs: glyphs}))
	if err != nil {
		t.Fatal(err)
	}
	if codes, missing := f.Encode("é"); missing != 1 || !bytes.Equal(codes, []byte{0, 0}) {
		t.Errorf("é in a face with e and no acute encodes as % x with %d missing, "+
			"want .notdef and one missing", codes, missing)
	}
}

// TestACharacterAFaceCannotCodeIsOneAnswer: a face addressed by character code
// has one answer for a character with no code — the space — and measuring,
// encoding and drawing it give that answer, with one count of what was
// missing. The simple faces gave three: Measure took the character's own
// advance (U+03B1 in a face that has an alpha and no WinAnsi code for it) or
// .notdef's, Encode left it out, and the by-code path drew a space. The
// by-code path also recorded its codes as the glyphs used, so a subset kept
// glyph 65 for an "A" and not the A.
func TestACharacterAFaceCannotCodeIsOneAnswer(t *testing.T) {
	helvetica, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	// Printable ASCII and an alpha: the alpha has a glyph and no WinAnsi code,
	// the euro sign has a code and no glyph, and U+E000 has neither.
	glyphs := append(asciiGlyphs(), fonttest.Glyph{Rune: 0x03B1, Advance: 900, HasShape: true})
	simple, err := LoadSimple(fonttest.SFNT(fonttest.SFNTOptions{Name: "Ascii", Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading the fixture as a simple face: %v", err)
	}
	const s = "a€αb"
	for _, tc := range []struct {
		name string
		f    *Face
		want []byte
	}{
		// Helvetica has a euro sign; the fixture does not.
		{"Helvetica", helvetica, []byte("a\x80 b ")},
		{"a simple face", simple, []byte("a  b ")},
	} {
		drawer, encoder := tc.f.Clone(), tc.f.Clone()
		glyphs, drawnMissing := drawer.ShapeGlyphs(s)
		codes, encodedMissing := encoder.Encode(s)
		var drawnCodes []byte
		for _, g := range glyphs {
			drawnCodes = append(drawnCodes, byte(g.GID))
		}
		if !bytes.Equal(drawnCodes, codes) || drawnMissing != encodedMissing {
			t.Errorf("%s draws %q as % x with %d missing and encodes it as % x with %d missing",
				tc.name, s, drawnCodes, drawnMissing, codes, encodedMissing)
		}
		if !bytes.Equal(codes, tc.want) {
			t.Errorf("%s encodes %q as % x, want % x: each character it has no code "+
				"for set as a space", tc.name, s, codes, tc.want)
		}
		if got, want := tc.f.Measure(s, 1000), MeasureGlyphs(glyphs, 1000); got != want {
			t.Errorf("%s measures %q at %v and draws it at %v", tc.name, s, got, want)
		}
		if got, want := drawer.Used(), encoder.Used(); !sameGIDs(got, want) {
			t.Errorf("%s records glyphs %v drawing %q and %v encoding it", tc.name, got, s, want)
		}
	}
	// And what the simple face records is the glyphs, not the codes: a, b and
	// the space are glyphs 66, 67 and 1 of the fixture, whose glyph 0 is
	// .notdef and whose first is the space.
	drawer := simple.Clone()
	drawer.ShapeGlyphs(s)
	if got := drawer.Used(); !sameGIDs(got, []int{1, 66, 67}) {
		t.Errorf("drawing %q in the simple face records glyphs %v, want [1 66 67]", s, got)
	}
}
