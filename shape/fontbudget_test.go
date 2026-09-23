package shape

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/internal/costtest"
)

// Load reads a font under one work budget, shared by the sfnt and CFF readers;
// see maxFontWork. These are what is true of it only here, at the call: that
// the CFF's widths are not what Load pays for, and that a font which spends the
// budget — from either reader — is refused with an error that says so.

// TestLoadDoesNotInterpretCharstrings is audit C4 through the door a document
// uses. A subroutine that calls the next one k times, five deep, is k^5 calls,
// and finding a glyph's width means making them; nothing in Load reads a CFF
// width — the advances come from hmtx — so a fan-out four times wider costs
// Load nothing more. With the widths read it cost a thousand times more.
func TestLoadDoesNotInterpretCharstrings(t *testing.T) {
	face := func(fanOut int) []byte {
		glyphs := []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}
		cff := fonttest.SubrFanOut(fanOut, 5, len(glyphs)+1)
		return fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs})
	}
	small, large := face(4), face(16)
	for _, data := range [][]byte{small, large} {
		if f, err := Load(data); err != nil || !f.IsCFF() {
			t.Fatalf("the fixture did not load as CFF: %v", err)
		}
	}
	// Timed, because Load's budget is its own and not one a caller can read;
	// see costtest.Time.
	c := costtest.Time(t, "loading a CFF of subroutine fan-out 4 and 16",
		func() { _, _ = Load(small) }, func() { _, _ = Load(large) })
	if c.Ratio > 8 {
		t.Errorf("a fan-out of 4 loaded in %v and of 16 in %v, %.1f times; the "+
			"font is a few bytes larger and nothing else about it is, so Load is "+
			"walking the subroutines", c.Small, c.Large, c.Ratio)
	}
}

// TestACFFThatSpendsTheFontsBudgetIsRefused is the same refusal from the CFF
// side, which draws on the same budget the sfnt reader has already spent some
// of: a thousand Font DICTs each naming a different eight-kilobyte Private DICT
// — different because each is a byte shorter than the last, so none is read in
// place of another — out of about sixteen kilobytes of font.
func TestACFFThatSpendsTheFontsBudgetIsRefused(t *testing.T) {
	const fds, privSize = 1000, 8192
	op3 := func(v int) []byte { return []byte{28, byte(v >> 8), byte(v)} }
	index := func(items ...[]byte) []byte { // two-byte offsets
		out := []byte{byte(len(items) >> 8), byte(len(items)), 2, 0, 1}
		off := 1
		for _, it := range items {
			off += len(it)
			out = binary.BigEndian.AppendUint16(out, uint16(off))
		}
		for _, it := range items {
			out = append(out, it...)
		}
		return out
	}
	top := func(csOff, fdaOff int) []byte {
		d := append(append(append(op3(0), op3(0)...), op3(0)...), 12, 30) // ROS
		d = append(append(d, op3(csOff)...), 17)                          // CharStrings
		return append(append(d, op3(fdaOff)...), 12, 36)                  // FDArray
	}
	// Built twice: once to learn where the FDArray and the charstrings land,
	// and once with the top DICT saying so. Its operands are all three bytes,
	// so the second pass lays everything out where the first found it.
	var build func(csOff, fdaOff int) []byte
	build = func(csOff, fdaOff int) []byte {
		data := []byte{1, 0, 4, 1}
		data = append(data, index([]byte("F"))...)
		data = append(data, index(top(csOff, fdaOff))...)
		data = append(data, 0, 0, 0, 0) // String and Global Subr INDEXes, empty
		privAt := len(data)
		for range privSize {
			data = append(data, 139) // the operand 0, which no operator takes
		}
		dicts := make([][]byte, fds)
		for i := range dicts {
			dicts[i] = append(append(op3(privSize-i), op3(privAt)...), 18)
		}
		fda := len(data)
		data = append(data, index(dicts...)...)
		cs := len(data)
		data = append(data, index([]byte{14}, []byte{14})...)
		if fda != fdaOff || cs != csOff {
			return build(cs, fda)
		}
		return data
	}
	cff := build(0, 0)
	glyphs := []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}
	_, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err == nil {
		t.Fatalf("a CFF of %d bytes naming %d Private DICTs of %d bytes was loaded",
			len(cff), fds, privSize)
	}
	if !strings.Contains(err.Error(), "Private DICT") ||
		!strings.Contains(err.Error(), "not read in full") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the "+
			"budget's, naming what it was reading", err)
	}
}

// TestAFontThatSpendsItsBudgetIsRefused: a font whose reading runs past the
// budget is refused, with an error naming what was being read, rather than
// loaded with part of it missing.
//
// The shape is the glyf walk, which the cmap refusal above does not reach:
// loca offsets alternating 0 and L give every other glyph the whole of glyf,
// and each of them walks every component in it.
func TestAFontThatSpendsItsBudgetIsRefused(t *testing.T) {
	const components, glyphs = 50000, 200
	composite := make([]byte, 10)
	binary.BigEndian.PutUint16(composite, 0xFFFF) // numberOfContours -1
	for range components {
		composite = append(composite, 0, 0x20, 0, 1, 0, 0) // MORE_COMPONENTS
	}
	loca := make([]byte, 4*(glyphs+2))
	for i := 1; i < glyphs+2; i += 2 {
		binary.BigEndian.PutUint32(loca[4*i:], uint32(len(composite)))
	}
	gs := make([]fonttest.Glyph, glyphs)
	for i := range gs {
		gs[i] = fonttest.Glyph{Rune: rune('A' + i), Advance: 500}
	}
	_, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: gs,
		Extra:  map[string][]byte{"glyf": composite, "loca": loca},
	}))
	if err == nil {
		t.Fatalf("a font whose glyphs walk %d components each, %d times over, "+
			"was loaded", components, glyphs/2)
	}
	if !strings.Contains(err.Error(), "composite glyphs") ||
		!strings.Contains(err.Error(), "not read in full") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the "+
			"budget's, naming what it was reading", err)
	}
}
