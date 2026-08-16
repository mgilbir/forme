package font

import (
	"os"
	"testing"
)

// The widths a CFF states against the widths its sfnt states.
//
// An OpenType/CFF font carries the same advance twice: hmtx has it, and every
// charstring may state its own as a delta from the Private DICT's
// nominalWidthX. They have to agree — hmtx is what a renderer lays out with and
// what this engine treats as authoritative, and the charstring's copy is what a
// consumer embedding the outlines reads.
//
// So hmtx is the oracle for the CFF reader, and it is a real one: it is written
// by the same tool from the same source, but it is a different table read by a
// different function, and every defect this file has had would have shown here.
// The FD Private DICTs were 17,707 of Noto Sans JP's widths reading as zero; the
// subroutine width was 289 of them reading as the default. Neither is subtle
// against hmtx and neither was visible without it.
func TestCFFWidthsAgreeWithHmtx(t *testing.T) {
	for _, path := range []string{
		"../testdata/notocjk/NotoSansJP-Regular.otf",
		"../testdata/wpt/fonts/noto/cjk/NotoSansCJKjp-Regular-subset-chws.otf",
		"../testdata/wpt/fonts/adobe-fonts/CSSHWOrientationTest.otf",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // fetched corpora; `make notocjk wpt` puts them there
		}
		tables := SFNTTables(data)
		if tables == nil || tables["CFF "] == nil {
			continue
		}
		sfnt := ParseSFNT(data, 1<<22)
		cff := ParseCFF(tables["CFF "])
		if sfnt == nil || cff == nil {
			t.Errorf("%s: did not parse", path)
			continue
		}
		if len(sfnt.WidthByGID) == 0 {
			t.Errorf("%s: the sfnt reports no widths, so there is no oracle here", path)
			continue
		}
		n := len(cff.WidthByGID)
		if len(sfnt.WidthByGID) < n {
			n = len(sfnt.WidthByGID)
		}
		if n == 0 {
			t.Errorf("%s: no glyphs to compare", path)
			continue
		}
		var wrong, first int
		var gotW, wantW float64
		for g := 0; g < n; g++ {
			// A unit of slack: hmtx is an integer in font units and the
			// charstring's is a delta the FontMatrix scales, so the two can
			// round differently in the last place.
			d := cff.WidthByGID[g] - sfnt.WidthByGID[g]
			if d < 0 {
				d = -d
			}
			if d > 1 {
				if wrong == 0 {
					first, gotW, wantW = g, cff.WidthByGID[g], sfnt.WidthByGID[g]
				}
				wrong++
			}
		}
		if wrong != 0 {
			t.Errorf("%s: %d of %d glyphs have a CFF width that is not the hmtx one; "+
				"the first is glyph %d, %v against %v", path, wrong, n, first, gotW, wantW)
		}
	}
}
