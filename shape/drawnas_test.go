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
