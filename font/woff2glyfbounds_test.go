package font

import (
	"encoding/binary"
	"strings"
	"testing"
)

// The glyf transform is the part of a WOFF 2 that is rebuilt rather than
// copied, and so the part where a font gets to choose the arithmetic. It is
// reached from an @font-face URL with nothing above it that recovers, which
// makes every one of these a crash or a wrong outline from bytes off the web.
//
// The streams are assembled here rather than wrapped in a whole font, because
// what is under test is the rebuild and a container round it would only be a
// second thing to get right.

// glyfStreams are the seven streams of W3C WOFF 2.0 §5.1's glyf transform, in
// the order the format writes their lengths.
type glyfStreams struct {
	nContours    []byte
	nPoints      []byte
	flags        []byte
	glyphs       []byte
	composites   []byte
	bboxes       []byte
	instructions []byte
}

func (s glyfStreams) encode(numGlyphs, indexFormat int) []byte {
	var b []byte
	u16 := func(v int) { b = binary.BigEndian.AppendUint16(b, uint16(v)) }
	u16(0) // the transform version, which has only ever been 0
	u16(0) // flags: no overlap bitmap
	u16(numGlyphs)
	u16(indexFormat)
	all := [7][]byte{s.nContours, s.nPoints, s.flags, s.glyphs, s.composites,
		s.bboxes, s.instructions}
	for _, st := range all {
		b = binary.BigEndian.AppendUint32(b, uint32(len(st)))
	}
	for _, st := range all {
		b = append(b, st...)
	}
	return b
}

// rebuildGlyf runs the transform over hand-built streams.
func rebuildGlyf(numGlyphs, indexFormat int, s glyfStreams) ([]byte, error) {
	want := 2 * (uint32(numGlyphs) + 1)
	if indexFormat != 0 {
		want *= 2
	}
	f := &woff2Font{loca: &woff2Table{origLength: want}}
	out, _, err := reconstructGlyf(nil, s.encode(numGlyphs, indexFormat), &woff2Table{}, f)
	return out, err
}

// bboxBitmapFor is the "did this glyph state its own box" bitmap, with no bit
// set: every glyph's box is computed from its points, which is the path the
// transform exists for and the one that reads the points.
func bboxBitmapFor(numGlyphs int) []byte {
	return make([]byte, ((numGlyphs+31)>>5)<<2)
}

// TestAContourWithNoPointsIsRefused is the crash.
//
// glyf states a contour by the index of its last point, so a contour with no
// points has no index to state — the first one writes 65,535, which addresses a
// point the glyph does not have. A glyph made only of them has no points at
// all, and computing its bounding box from them sliced an empty slice: a panic
// out of the font parser, from a font an @font-face URL delivered, with nothing
// above it that recovers.
func TestAContourWithNoPointsIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name      string
		nContours int
		counts    []byte
	}{
		{"one contour with no points", 1, []byte{0}},
		{"a leading contour with no points", 2, []byte{0, 2}},
		{"a trailing contour with no points", 2, []byte{2, 0}},
		{"every contour with no points", 2, []byte{0, 0}},
	} {
		// Two points' worth of triplet data, which the non-empty contours use.
		s := glyfStreams{
			nContours: binary.BigEndian.AppendUint16(nil, uint16(tc.nContours)),
			nPoints:   tc.counts,
			flags:     []byte{1, 1},
			glyphs:    []byte{1, 1, 0}, // two one-byte points, then no instructions
			bboxes:    bboxBitmapFor(1),
		}
		_, err := rebuildGlyf(1, 0, s)
		if err == nil {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), "contour with no points") {
			t.Errorf("%s was refused as %q, want the contour message", tc.name, err)
		}
	}
}

// TestAGlyphWithPointsIsStillRebuilt is the control: the refusal above must not
// be a refusal to read a glyph at all.
func TestAGlyphWithPointsIsStillRebuilt(t *testing.T) {
	s := glyfStreams{
		nContours: binary.BigEndian.AppendUint16(nil, 1),
		nPoints:   []byte{2},
		flags:     []byte{1, 1},
		glyphs:    []byte{1, 1, 0},
		bboxes:    bboxBitmapFor(1),
	}
	out, err := rebuildGlyf(1, 0, s)
	if err != nil {
		t.Fatalf("an ordinary two-point glyph was refused: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("an ordinary two-point glyph rebuilt to nothing")
	}
	if got := int16(binary.BigEndian.Uint16(out)); got != 1 {
		t.Errorf("the rebuilt glyph declares %d contours, want 1", got)
	}
	if got := binary.BigEndian.Uint16(out[10:]); got != 1 {
		t.Errorf("the contour ends at point %d, want 1 for a two-point contour", got)
	}
}

// fourBytePoint is a triplet-encoded point that moves by dx and dy, each
// written in sixteen bits with its sign in the flag. It is the widest of the
// encoding's five forms and the only one that can name a large move.
func fourBytePoint(dx, dy int) (flag byte, data []byte) {
	flag = 124
	if dx >= 0 {
		flag |= 1
	} else {
		dx = -dx
	}
	if dy >= 0 {
		flag |= 2
	} else {
		dy = -dy
	}
	data = []byte{byte(dx >> 8), byte(dx), byte(dy >> 8), byte(dy)}
	return flag, data
}

// TestAnOutlineOutsideWhatGlyfCanStateIsRefused is the silent one.
//
// A glyf coordinate is sixteen bits and a delta between two points is sixteen
// bits, and there is no other way to write either. The rebuild wrote the low
// sixteen bits of whatever it had: an outline with points in places the font
// never named, which is a glyph that looks like a glyph and is not the one the
// font describes. The bound it did check was the thirty-two-bit one, which no
// representable outline can reach.
func TestAnOutlineOutsideWhatGlyfCanStateIsRefused(t *testing.T) {
	build := func(moves [][2]int) glyfStreams {
		var flags, data []byte
		for _, m := range moves {
			f, d := fourBytePoint(m[0], m[1])
			flags = append(flags, f)
			data = append(data, d...)
		}
		return glyfStreams{
			nContours: binary.BigEndian.AppendUint16(nil, 1),
			nPoints:   []byte{byte(len(moves))},
			flags:     flags,
			glyphs:    append(data, 0),
			bboxes:    bboxBitmapFor(1),
		}
	}

	for _, tc := range []struct {
		name  string
		moves [][2]int
		says  string
	}{
		{"a point past the right of the coordinate space",
			[][2]int{{40000, 0}}, "coordinate space"},
		{"a point past the top of the coordinate space",
			[][2]int{{0, 40000}}, "coordinate space"},
		{"a point reached in two moves",
			[][2]int{{20000, 0}, {20000, 0}}, "coordinate space"},
		{"two points further apart than a delta can say",
			[][2]int{{-20000, 0}, {40000, 0}}, "further between two points"},
		{"the same on the other axis",
			[][2]int{{0, -20000}, {0, 40000}}, "further between two points"},
	} {
		_, err := rebuildGlyf(1, 0, build(tc.moves))
		if err == nil {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s was refused as %q, want a message about %q", tc.name, err, tc.says)
		}
	}

	// The control: an outline that fits is rebuilt, so the bound is a bound and
	// not a refusal to read a large glyph.
	if _, err := rebuildGlyf(1, 0, build([][2]int{{-20000, -20000}, {30000, 30000}})); err != nil {
		t.Errorf("an outline spanning most of the coordinate space was refused: %v", err)
	}
}

// TestGlyphsPastWhatAShortLocaCanAddressAreRefused is the same shape of defect
// one table along.
//
// The short form of loca stores half the offset in sixteen bits, so it reaches
// 128 KiB and no further. A font whose glyphs come to more than that and still
// declares the short form is one no reader can use — and writing the low bits
// produced a loca that parses and sends every glyph past the first 128 KiB to
// the wrong bytes.
func TestGlyphsPastWhatAShortLocaCanAddressAreRefused(t *testing.T) {
	// Four glyphs of forty thousand instruction bytes each. Instructions are
	// copied through, so this is the cheapest way to make a large glyf.
	const glyphs, instrLen = 4, 40000
	var nContours, nPoints, flags, glyphStream []byte
	for i := 0; i < glyphs; i++ {
		nContours = binary.BigEndian.AppendUint16(nContours, 1)
		nPoints = append(nPoints, 1)
		flags = append(flags, 1)
		// One one-byte point, then the instruction length as a 255UInt16.
		glyphStream = append(glyphStream, 1, 253)
		glyphStream = binary.BigEndian.AppendUint16(glyphStream, instrLen)
	}
	s := glyfStreams{
		nContours:    nContours,
		nPoints:      nPoints,
		flags:        flags,
		glyphs:       glyphStream,
		bboxes:       bboxBitmapFor(glyphs),
		instructions: make([]byte, glyphs*instrLen),
	}

	if _, err := rebuildGlyf(glyphs, 0, s); err == nil {
		t.Error("a font whose glyphs run past what its short loca can address was accepted")
	} else if !strings.Contains(err.Error(), "short loca") {
		t.Errorf("it was refused as %q, want a message about the short loca", err)
	}

	// The same font declaring the long form is fine, which is what says the
	// refusal is about the offsets and not about the size.
	if _, err := rebuildGlyf(glyphs, 1, s); err != nil {
		t.Errorf("the same glyphs with a long loca were refused: %v", err)
	}
}

// TestAFontWithEveryGlyphIndexIsNotRefusedForItsSize is the wrap.
//
// loca has one more entry than the font has glyphs, and that "+ 1" was computed
// in the sixteen bits the count is stated in. A font with the full 65,535
// glyphs wrapped it to zero and was refused as "not the size its glyph count
// calls for" — the arithmetic reporting the font as malformed for reaching a
// count the format allows. Large CJK faces reach it.
func TestAFontWithEveryGlyphIndexIsNotRefusedForItsSize(t *testing.T) {
	const glyphs = 65535
	// Every glyph empty: no contours, no points, no bytes. The size check
	// happens before any of them is read, which is the whole of what is
	// being tested.
	s := glyfStreams{
		nContours: make([]byte, 2*glyphs),
		bboxes:    bboxBitmapFor(glyphs),
	}
	if _, err := rebuildGlyf(glyphs, 1, s); err != nil {
		t.Errorf("a font with %d glyphs was refused: %v", glyphs, err)
	}
}
