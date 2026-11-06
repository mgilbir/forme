package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestCmapGlyphAndCmapAllReadWhatCmapCopies: CmapGlyph and CmapAll are Cmap
// without the copy, so they are held to it entry for entry — for a face
// embedded as a composite font and one embedded as a simple font, where
// GlyphID answers with a character code instead and CmapGlyph must not.
func TestCmapGlyphAndCmapAllReadWhatCmapCopies(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Ascii", Glyphs: asciiGlyphs()})
	simple, err := LoadSimple(data)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]*Face{"composite": noto, "simple": simple} {
		cmap := f.Cmap()
		if len(cmap) == 0 {
			t.Fatalf("%s: the face maps nothing", name)
		}
		seen := 0
		for r, gid := range f.CmapAll() {
			seen++
			if want, ok := cmap[r]; !ok || want != gid {
				t.Errorf("%s: CmapAll yields %U -> %d, Cmap has %d (%v)", name, r, gid, want, ok)
			}
		}
		if seen != len(cmap) {
			t.Errorf("%s: CmapAll yields %d entries, Cmap has %d", name, seen, len(cmap))
		}
		for r, want := range cmap {
			gid, ok := f.CmapGlyph(r)
			if ok != (want != 0) || (ok && gid != want) {
				t.Errorf("%s: CmapGlyph(%U) = %d, %v; Cmap has %d", name, r, gid, ok, want)
			}
		}
		if _, ok := f.CmapGlyph(0x10FFFF); ok {
			t.Errorf("%s: CmapGlyph maps a character the font does not have", name)
		}
		// Stopping early stops.
		n := 0
		for range f.CmapAll() {
			n++
			break
		}
		if n != 1 {
			t.Errorf("%s: a loop that breaks after one entry saw %d", name, n)
		}
	}

	// A simple face's GlyphID is a WinAnsi code; CmapGlyph is the glyph.
	code, _ := simple.GlyphID('A')
	gid, ok := simple.CmapGlyph('A')
	if !ok || gid != simple.Cmap()['A'] || code != 'A' {
		t.Errorf("simple face: A is code %d and glyph %d (%v); want code 65 and glyph %d",
			code, gid, ok, simple.Cmap()['A'])
	}

	// A standard face has no program, and maps nothing.
	helvetica, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := helvetica.CmapGlyph('A'); ok {
		t.Error("a standard face maps A to a glyph of a program it does not have")
	}
	for r := range helvetica.CmapAll() {
		t.Errorf("a standard face yields %U", r)
	}
}

// BenchmarkCmapLookup is what a caller looking up a few characters paid with
// Cmap, against what it pays with CmapGlyph.
func BenchmarkCmapLookup(b *testing.B) {
	f, err := NotoSans()
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Cmap", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = f.Cmap()['A']
		}
	})
	b.Run("CmapGlyph", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = f.CmapGlyph('A')
		}
	})
}
