package shape

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A subset that drops cvt, fpgm and prep has to drop the glyphs' own
// instructions too.
//
// The two halves are one thing. A glyph's instruction stream calls functions
// defined in fpgm and reads values out of cvt, so keeping it and dropping those
// is a program whose first CALL names a function nobody defined — an error by
// the specification, and what a rasteriser makes of one is its own business.
// The subsetter's own comment said hinting was dropped "with the rest"; half of
// it was.

// glyphInstructions is the length of one glyf entry's instruction stream, and
// whether the entry has a place to hold one at all.
func glyphInstructions(g []byte) (n int, has bool) {
	if len(g) < 10 {
		return 0, false
	}
	contours := int(int16(binary.BigEndian.Uint16(g)))
	if contours >= 0 {
		at := 10 + 2*contours
		if at+2 > len(g) {
			return 0, false
		}
		return int(binary.BigEndian.Uint16(g[at:])), true
	}
	for at := 10; ; {
		if at+4 > len(g) {
			return 0, false
		}
		flags := int(binary.BigEndian.Uint16(g[at:]))
		at += 4
		if flags&compArgsAreWords != 0 {
			at += 4
		} else {
			at += 2
		}
		switch {
		case flags&compHaveScale != 0:
			at += 2
		case flags&compHaveXYScale != 0:
			at += 4
		case flags&compHave2x2 != 0:
			at += 8
		}
		if at > len(g) {
			return 0, false
		}
		if flags&compMoreComponents != 0 {
			continue
		}
		if flags&compHaveInstructions == 0 {
			return 0, true
		}
		if at+2 > len(g) {
			return 0, false
		}
		return int(binary.BigEndian.Uint16(g[at:])), true
	}
}

// glyfEntries splits a glyf table by its loca, in whichever of the two forms
// head declares. A subset is always the long one; a shipped face is usually the
// short one, whose offsets are halved.
func glyfEntries(t *testing.T, data []byte) [][]byte {
	t.Helper()
	tables := font.SFNTTables(data)
	head, loca, glyf := tables["head"], tables["loca"], tables["glyf"]
	if len(head) < 52 {
		t.Fatalf("no usable head (%d bytes)", len(head))
	}
	long := binary.BigEndian.Uint16(head[50:]) == 1
	size := 2
	if long {
		size = 4
	}
	at := func(i int) uint32 {
		if long {
			return binary.BigEndian.Uint32(loca[size*i:])
		}
		return uint32(binary.BigEndian.Uint16(loca[size*i:])) * 2
	}
	var out [][]byte
	for i := 0; size*(i+2) <= len(loca); i++ {
		start, end := at(i), at(i+1)
		if start > end || int(end) > len(glyf) {
			t.Fatalf("glyph %d lies outside glyf: %d..%d of %d", i, start, end, len(glyf))
		}
		out = append(out, glyf[start:end])
	}
	return out
}

func notoFaces(t *testing.T) []string {
	t.Helper()
	return notoTTFs(t)
}

// TestASubsetCarriesNoInstructionsAtAll.
func TestASubsetCarriesNoInstructionsAtAll(t *testing.T) {
	hinted := 0
	for _, name := range notoFaces(t) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		f, err := Load(data)
		if err != nil {
			t.Fatalf("loading %s: %v", name, err)
		}
		f.Encode("The quick brown fox 0123456789")
		sub, err := f.Subset()
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(name), err)
		}
		// The tables the instructions would have needed are gone, which is the
		// premise of the whole test.
		st := font.SFNTTables(sub)
		for _, tag := range []string{"cvt ", "fpgm", "prep"} {
			if _, ok := st[tag]; ok {
				t.Errorf("%s: the subset kept %q", filepath.Base(name), tag)
			}
		}
		for gid, g := range glyfEntries(t, sub) {
			if n, _ := glyphInstructions(g); n != 0 {
				t.Errorf("%s: glyph %d carries %d bytes of instructions, and the "+
					"fpgm they call is not in the subset",
					filepath.Base(name), gid, n)
			}
		}
		// And the subset is still a font that draws the same letters. Stripping
		// a run out of the middle of every glyph is a rewrite, and the guard
		// against getting it wrong is that the result reads back.
		back, err := Load(sub)
		if err != nil {
			t.Fatalf("%s: the subset did not load: %v", filepath.Base(name), err)
		}
		const word = "The quick brown fox"
		want, wantMissing := f.ShapeGlyphs(word)
		got, gotMissing := back.ShapeGlyphs(word)
		if wantMissing != gotMissing || len(want) != len(got) {
			t.Fatalf("%s: the original shaped %d glyphs (%d missing) and the subset "+
				"%d (%d missing)", filepath.Base(name), len(want), wantMissing,
				len(got), gotMissing)
		}
		// The glyphs and not their advances: a subset drops GPOS with the rest
		// of the layout tables, so a kerned pair is a hundredth of an em wider
		// in it. That is the documented trade and not this test's business.
		for i := range want {
			if want[i].GID != got[i].GID {
				t.Errorf("%s: glyph %d is %d in the subset and %d in the original",
					filepath.Base(name), i, got[i].GID, want[i].GID)
				break
			}
		}
		// And the source really did have instructions, or this proves nothing.
		for _, g := range glyfEntries(t, data) {
			if n, _ := glyphInstructions(g); n > 0 {
				hinted++
				break
			}
		}
	}
	if hinted == 0 {
		t.Fatal("none of the faces is hinted, so nothing above was tested")
	}
	t.Logf("%d hinted faces subsetted", hinted)
}

// TestStrippingInstructionsLeavesTheOutlineAlone is the property that makes the
// splice safe: the instructions are a contiguous run with a length in front of
// them, so taking them out moves no point and re-encodes no coordinate.
func TestStrippingInstructionsLeavesTheOutlineAlone(t *testing.T) {
	checked := 0
	for _, name := range notoFaces(t) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for gid, g := range glyfEntries(t, data) {
			n, has := glyphInstructions(g)
			stripped := stripInstructions(g)
			if !has || n == 0 {
				if len(stripped) != len(g) {
					t.Errorf("%s glyph %d has no instructions and was changed from "+
						"%d bytes to %d", filepath.Base(name), gid, len(g), len(stripped))
				}
				continue
			}
			checked++
			if got, _ := glyphInstructions(stripped); got != 0 {
				t.Errorf("%s glyph %d still carries %d instruction bytes",
					filepath.Base(name), gid, got)
			}
			// Everything before the instruction stream, and everything after
			// it, byte for byte — which for a simple glyph is the header, the
			// contour ends and every coordinate.
			contours := int(int16(binary.BigEndian.Uint16(g)))
			if contours < 0 {
				// A composite: the components are the whole of it, and only the
				// one flag word may differ.
				if len(stripped) >= len(g) {
					t.Errorf("%s glyph %d is a composite whose instructions were "+
						"not removed", filepath.Base(name), gid)
				}
				continue
			}
			at := 10 + 2*contours
			if string(g[:at]) != string(stripped[:at]) {
				t.Errorf("%s glyph %d: the header and contour ends changed",
					filepath.Base(name), gid)
			}
			if string(g[at+2+n:]) != string(stripped[at+2:]) {
				t.Errorf("%s glyph %d: the coordinates changed", filepath.Base(name), gid)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no glyph in any face carried instructions, so nothing was tested")
	}
	t.Logf("%d hinted glyphs stripped with their outlines intact", checked)
}
