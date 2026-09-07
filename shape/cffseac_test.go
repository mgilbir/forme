package shape

import (
	"testing"
	"time"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// A seac names two other glyphs, and a subset that drops them ships a
// charstring that draws nothing.
//
// The subsetter's comment said a charstring reusing another shape does it
// "through seac or a subroutine, and both are carried along by keeping the
// subroutine INDEXes whole". A subroutine is a piece of this charstring and
// does travel with the INDEX. A seac is a reference to two glyphs by
// StandardEncoding code, and keeping every subroutine says nothing about them.

// cffNumber encodes one Type 2 operand.
func cffNumber(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		v -= 108
		return []byte{byte(247 + v/256), byte(v % 256)}
	case v >= -1131 && v <= -108:
		v = -v - 108
		return []byte{byte(251 + v/256), byte(v % 256)}
	}
	return []byte{28, byte(v >> 8), byte(v)}
}

// standardCode is the StandardEncoding code that stands for a glyph name.
func standardCode(t *testing.T, name string) int {
	t.Helper()
	for code, n := range font.StandardEncodingNames {
		if n == name {
			return int(code)
		}
	}
	t.Fatalf("StandardEncoding names no code %q, so the fixture cannot be built", name)
	return 0
}

// seacFace builds a CFF face of four glyphs: .notdef, "A", "acute", and an
// "Aacute" drawn by a seac naming the other two.
func seacFace(t *testing.T) (f *Face, gidA, gidAcute, gidAacute int) {
	t.Helper()
	sidA, ok := font.CFFStandardSID("A")
	if !ok {
		t.Fatal(`"A" is not a predefined CFF string`)
	}
	sidAcute, ok := font.CFFStandardSID("acute")
	if !ok {
		t.Fatal(`"acute" is not a predefined CFF string`)
	}
	sidAacute, ok := font.CFFStandardSID("Aacute")
	if !ok {
		t.Fatal(`"Aacute" is not a predefined CFF string`)
	}

	const endchar = 14
	seac := append([]byte(nil), cffNumber(0)...)                // adx
	seac = append(seac, cffNumber(0)...)                        // ady
	seac = append(seac, cffNumber(standardCode(t, "A"))...)     // bchar
	seac = append(seac, cffNumber(standardCode(t, "acute"))...) // achar
	seac = append(seac, endchar)

	// A and the accent draw something, so that a charstring the subsetter kept
	// is longer than the bare endchar it replaces a dropped one with.
	drawn := append([]byte(nil), cffNumber(100)...)
	drawn = append(drawn, 22, endchar) // hmoveto, endchar

	cff := fonttest.CFF(fonttest.CFFOptions{
		Glyphs:      4,
		CharsetSIDs: []int{sidA, sidAcute, sidAacute},
		Charstrings: [][]byte{{endchar}, drawn, drawn, seac},
	})
	glyphs := []fonttest.Glyph{
		{Rune: 'A', Advance: 500, HasShape: true},
		{Rune: 0x00B4, Advance: 0, HasShape: true},   // ACUTE ACCENT
		{Rune: 0x00C1, Advance: 500, HasShape: true}, // Á
	}
	face, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading the seac fixture: %v", err)
	}
	return face, 1, 2, 3
}

// TestASubsetKeepsWhatASeacIsDrawnFrom.
func TestASubsetKeepsWhatASeacIsDrawnFrom(t *testing.T) {
	f, gidA, gidAcute, gidAacute := seacFace(t)
	// Only the accented letter is used, which is the ordinary case: a document
	// setting "Á" never asks for "A" or for the accent on its own.
	if _, missing := f.Encode("Á"); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	data, _, err := f.subset()
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	cff := font.SFNTTables(data)["CFF "]
	cs, err := CharStringsForTest(cff)
	if err != nil {
		t.Fatalf("reading the subset's charstrings: %v", err)
	}
	if len(cs) != 4 {
		t.Fatalf("the subset holds %d charstrings, want 4 — the numbering does not change", len(cs))
	}
	for _, tc := range []struct {
		gid  int
		what string
	}{
		{gidAacute, "the accented letter the document used"},
		{gidA, "the letter its seac names as the base"},
		{gidAcute, "the accent its seac names"},
	} {
		if len(cs[tc.gid]) <= 1 {
			t.Errorf("glyph %d, %s, came out of the subsetter as a bare endchar; "+
				"the accented letter is drawn from glyphs the subset does not have",
				tc.gid, tc.what)
		}
	}
}

// TestASubsetStillDropsWhatNothingNames is the control. The closure must add
// what a seac names and nothing else, or it is not a subsetter.
func TestASubsetStillDropsWhatNothingNames(t *testing.T) {
	f, _, _, _ := seacFace(t)
	if _, missing := f.Encode("A"); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	data, _, err := f.subset()
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	cs, err := CharStringsForTest(font.SFNTTables(data)["CFF "])
	if err != nil {
		t.Fatalf("reading the subset's charstrings: %v", err)
	}
	// "A" was used; the accent and the accented letter were not, and nothing
	// the document kept names either.
	if len(cs[3]) > 1 {
		t.Error("the accented letter was kept although nothing used it or named it")
	}
	if len(cs[2]) > 1 {
		t.Error("the accent was kept although nothing used it or named it")
	}
}

// TestASeacIsToldFromAnOrdinaryEndchar. Almost every charstring ends with a
// bare endchar, and reading one as a seac would keep two glyphs chosen by
// whatever happened to be on the stack.
func TestASeacIsToldFromAnOrdinaryEndchar(t *testing.T) {
	const endchar = 14
	for _, tc := range []struct {
		what string
		code []byte
		ok   bool
		b, a int
	}{
		{"a bare endchar", []byte{endchar}, false, 0, 0},
		{"an endchar carrying only a width", append(cffNumber(42), endchar), false, 0, 0},
		{"a seac", func() []byte {
			c := append([]byte(nil), cffNumber(10)...)
			c = append(c, cffNumber(20)...)
			c = append(c, cffNumber(65)...)
			c = append(c, cffNumber(194)...)
			return append(c, endchar)
		}(), true, 65, 194},
		{"a seac with a width in front of it", func() []byte {
			c := append([]byte(nil), cffNumber(500)...)
			c = append(c, cffNumber(10)...)
			c = append(c, cffNumber(20)...)
			c = append(c, cffNumber(65)...)
			c = append(c, cffNumber(194)...)
			return append(c, endchar)
		}(), true, 65, 194},
		{"a moveto and an endchar", func() []byte {
			c := append([]byte(nil), cffNumber(10)...)
			c = append(c, cffNumber(20)...)
			c = append(c, 21) // rmoveto, which clears the stack
			return append(c, endchar)
		}(), false, 0, 0},
	} {
		b, a, ok := cffSeac(tc.code, nil, nil)
		if ok != tc.ok || (ok && (b != tc.b || a != tc.a)) {
			t.Errorf("%s: seac=%v (%d, %d), want %v (%d, %d)",
				tc.what, ok, b, a, tc.ok, tc.b, tc.a)
		}
	}
}

// TestASeacPushedFromASubroutineIsFound. The four arguments may be pushed
// anywhere — by the charstring, by a subroutine it calls, or partly by each —
// which is why the stack has to be walked rather than the last few bytes read.
func TestASeacPushedFromASubroutineIsFound(t *testing.T) {
	const endchar = 14
	// Subroutine 0 pushes the two chars and returns.
	sub := append([]byte(nil), cffNumber(65)...)
	sub = append(sub, cffNumber(194)...)
	sub = append(sub, 11) // return
	local := [][]byte{sub}

	// The charstring pushes the two displacements, calls the subroutine, and
	// ends. The call operand is biased.
	code := append([]byte(nil), cffNumber(10)...)
	code = append(code, cffNumber(20)...)
	code = append(code, cffNumber(0-font.CFFSubrBias(len(local)))...)
	code = append(code, 10) // callsubr
	code = append(code, endchar)

	b, a, ok := cffSeac(code, local, nil)
	if !ok || b != 65 || a != 194 {
		t.Errorf("a seac whose chars come from a subroutine reads as %v (%d, %d), "+
			"want true (65, 194)", ok, b, a)
	}
}

// TestACyclicSubroutineDoesNotHangTheWalk. A font is untrusted input, and a
// subroutine that calls itself is a few bytes to write.
func TestACyclicSubroutineDoesNotHangTheWalk(t *testing.T) {
	local := [][]byte{nil}
	// Subroutine 0 calls subroutine 0.
	call := append([]byte(nil), cffNumber(0-font.CFFSubrBias(len(local)))...)
	local[0] = append(call, 10) // callsubr

	done := make(chan bool, 1)
	go func() {
		cffSeac(local[0], local, nil)
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a self-calling subroutine did not finish")
	}
}
