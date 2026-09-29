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

// TestAGlyphDrawnByIndexIsKept: a glyph no shaping returned — the kind a
// formula's stretched operator is drawn with — is in Used and in the subset
// once Use has recorded it, for glyf outlines and for CFF ones, since each has
// its own subsetter reading the record. An index the face does not have is not
// recorded, a standard face has no indices to record, and a clone's record is
// its own.
func TestAGlyphDrawnByIndexIsKept(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	cff, err := Load(harfbuzzFont(t, "CFFInk.otf"))
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]*Face{"glyf": noto.Clone(), "CFF": cff.Clone()} {
		last := f.NumGlyphs() - 1
		if last < 2 {
			t.Fatalf("%s: the face has %d glyphs", name, f.NumGlyphs())
		}
		if len(f.Used()) != 0 {
			t.Fatalf("%s: a fresh clone has used %v", name, f.Used())
		}
		f.Use(last, 1, -1, f.NumGlyphs(), 1<<20)
		if got := f.Used(); len(got) != 2 || got[0] != 1 || got[1] != last {
			t.Errorf("%s: Used is %v after Use(%d, 1) and three indices past the face, want [1 %d]",
				name, got, last, last)
		}
		_, kept, err := f.SubsetGlyphs()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		in := map[int]bool{}
		for _, g := range kept {
			in[g] = true
		}
		if !in[1] || !in[last] {
			t.Errorf("%s: the subset kept %v, which lacks a glyph drawn by index", name, kept)
		}
	}
	if len(noto.Used()) != 0 || len(cff.Used()) != 0 {
		t.Errorf("Use on a clone reached the face it was made from: %v, %v", noto.Used(), cff.Used())
	}
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	std.Use(0, 65)
	if got := std.Used(); len(got) != 0 {
		t.Errorf("a standard face recorded %v, and has no glyph indices", got)
	}
}
