package shape

import "testing"

// The facts a format needs to embed a face. They are read from the bundled
// Noto, whose values are known, because a test that only checks the fields are
// present would pass against a face that answered zero for all of them.
func TestTheEmbeddingFactsAreTheFontsOwn(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	d := f.Descriptor()
	if d.Ascent <= 0 || d.Descent >= 0 {
		t.Errorf("ascent %d and descent %d: a font rises above the line and falls below it",
			d.Ascent, d.Descent)
	}
	if d.CapHeight <= 0 || d.CapHeight > d.Ascent {
		t.Errorf("cap height %d against ascent %d", d.CapHeight, d.Ascent)
	}
	if d.BBox[2] <= d.BBox[0] || d.BBox[3] <= d.BBox[1] {
		t.Errorf("bbox %v encloses nothing", d.BBox)
	}
	if d.StemV <= 0 {
		t.Errorf("stem width %d", d.StemV)
	}
	if d.Flags == 0 {
		t.Error("no flags at all, and every font is at least symbolic or not")
	}
	// Upright, so the angle is zero; an italic would be negative.
	if d.ItalicAngle != 0 {
		t.Errorf("italic angle %v for an upright face", d.ItalicAngle)
	}

	// The advance table covers the face and agrees with the per-glyph answer.
	adv := f.GlyphAdvances()
	if len(adv) != f.NumGlyphs() {
		t.Fatalf("%d advances for %d glyphs", len(adv), f.NumGlyphs())
	}
	gid, ok := f.GlyphID('A')
	if !ok {
		t.Fatal("the bundled face has no A")
	}
	if got, want := f.GlyphAdvance(gid), adv[gid]; got != want {
		t.Errorf("glyph %d advances %v one way and %v the other", gid, got, want)
	}
	if adv[gid] <= 0 {
		t.Errorf("the letter A advances %v", adv[gid])
	}

	// The way back to characters.
	if cmap := f.Cmap(); cmap['A'] != gid {
		t.Errorf("the cmap sends A to %d, and GlyphID says %d", cmap['A'], gid)
	}
	// Copied, so a caller cannot reach into the face through it.
	c1, c2 := f.Cmap(), f.Cmap()
	c1['A'] = -1
	if c2['A'] != gid || func() int { g, _ := f.GlyphID('A'); return g }() != gid {
		t.Error("the returned cmap is the face's own map, so writing to it changes the face")
	}

	if f.IsCFF() {
		t.Error("the bundled face has glyf outlines, so this should be false")
	}
}

// SubsetGlyphs must report what the subsetter kept, not what was asked for.
func TestSubsetGlyphsReportsWhatItKept(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	if _, missing := f.ShapeGlyphs("Aig"); missing != 0 {
		t.Fatalf("%d characters have no glyph", missing)
	}
	program, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	if len(program) == 0 {
		t.Fatal("the subset is empty")
	}
	if len(kept) == 0 {
		t.Fatal("the subset kept no glyphs")
	}
	// .notdef is always kept, and so is everything used.
	if kept[0] != 0 {
		t.Errorf("the first kept glyph is %d, want .notdef", kept[0])
	}
	inKept := map[int]bool{}
	for _, g := range kept {
		inKept[g] = true
	}
	for _, g := range f.Used() {
		if !inKept[g] {
			t.Errorf("glyph %d was used and is not in the subset", g)
		}
	}
}
