package shape

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// woff2Collection wraps fonts as a WOFF 2 collection, a table two of them carry
// with the same bytes stored once, none transformed.
func woff2Collection(fonts ...[]byte) []byte {
	var dir []fonttest.WOFF2Table
	stored := map[string]int{}
	c := &fonttest.WOFF2Collection{}
	for _, data := range fonts {
		tables := font.SFNTTables(data)
		var tags []string
		for tag := range tables {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		var f fonttest.WOFF2CollectionFont
		f.Flavor = binary.BigEndian.Uint32(data)
		for _, tag := range tags {
			key := tag + string(tables[tag])
			i, ok := stored[key]
			if !ok {
				i = len(dir)
				stored[key] = i
				dir = append(dir, fonttest.WOFF2Table{Tag: tag, Data: tables[tag]})
			}
			f.Tables = append(f.Tables, i)
		}
		c.Fonts = append(c.Fonts, f)
	}
	return fonttest.WOFF2(fonttest.WOFF2Options{Tables: dir, Collection: c, SpellOutTags: true})
}

// TestAFaceOfAWOFF2CollectionIsTheFontItWasMadeFrom wraps fixture faces — a
// colour face, a bitmap face, a variable face — as a WOFF 2 collection, and
// requires that each is loaded from it, described and cut as from the font it
// was made from, and that Load says to load such a file face by face.
func TestAFaceOfAWOFF2CollectionIsTheFontItWasMadeFrom(t *testing.T) {
	sources := [][]byte{harfbuzzFont(t, "ColourPaint.ttf"), harfbuzzFont(t, "BitmapInk.ttf"), harfbuzzFont(t, "VarComposite.ttf")}
	w := woff2Collection(sources...)
	descs, err := CollectionFaces(w)
	if err != nil || len(descs) != len(sources) {
		t.Fatalf("CollectionFaces: %d, %v", len(descs), err)
	}
	if _, err := Load(w); err == nil || !bytes.Contains([]byte(err.Error()), []byte("LoadCollection")) {
		t.Errorf("Load of a WOFF 2 collection: %v", err)
	}
	for i, src := range sources {
		want, err := Load(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := LoadCollection(w, i)
		if err != nil {
			t.Fatalf("face %d: %v", i, err)
		}
		if descs[i].Name != want.Name() || got.Name() != want.Name() || got.Descriptor() != want.Descriptor() {
			t.Errorf("face %d is %s %+v, alone %s %+v", i, got.Name(), got.Descriptor(), want.Name(), want.Descriptor())
		}
		for _, s := range []string{"AB", "office"} {
			g1, _ := got.ShapeGlyphs(s)
			g2, _ := want.ShapeGlyphs(s)
			if !reflect.DeepEqual(g1, g2) {
				t.Errorf("face %d shapes %q differently", i, s)
			}
		}
		for gid := range want.NumGlyphs() {
			a, b := &recordingPainter{}, &recordingPainter{}
			_ = got.PaintGlyph(gid, PaintOptions{}, a)
			_ = want.PaintGlyph(gid, PaintOptions{}, b)
			if !reflect.DeepEqual(a.lines, b.lines) {
				t.Errorf("face %d glyph %d paints differently", i, gid)
			}
		}
	}
	// The variable face, cut.
	coords := map[string]float64{"wght": 700}
	want, err := LoadInstance(sources[2], coords)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadCollectionInstance(w, 2, coords)
	if err != nil || got.Descriptor() != want.Descriptor() {
		t.Errorf("the variable face cut from the WOFF 2 collection: %v", err)
	}
}

// buildCollection writes a TrueType collection of fonts, storing a table that
// two of them carry with the same bytes once, as a collection's tools do: the
// header, each font's table directory, and the tables, each at an offset from
// the start of the file.
func buildCollection(fonts ...[]byte) []byte {
	type entry struct {
		tag  string
		data []byte
	}
	dirs := make([][]entry, len(fonts))
	versions := make([]uint32, len(fonts))
	for i, data := range fonts {
		versions[i] = binary.BigEndian.Uint32(data)
		tables := font.SFNTTables(data)
		for tag, t := range tables {
			dirs[i] = append(dirs[i], entry{tag, t})
		}
		sort.Slice(dirs[i], func(a, b int) bool { return dirs[i][a].tag < dirs[i][b].tag })
	}
	at := 12 + 4*len(fonts)
	dirAt := make([]int, len(fonts))
	for i := range fonts {
		dirAt[i] = at
		at += 12 + 16*len(dirs[i])
	}
	var blobs [][]byte
	offsets := map[string]int{}
	offsetOf := func(t []byte) int {
		if off, ok := offsets[string(t)]; ok {
			return off
		}
		at = (at + 3) &^ 3
		offsets[string(t)] = at
		blobs = append(blobs, t)
		at += len(t)
		return offsets[string(t)]
	}
	out := make([]byte, 12+4*len(fonts))
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], uint32(len(fonts)))
	var records []byte
	for i, d := range dirs {
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(dirAt[i]))
		head := make([]byte, 12)
		binary.BigEndian.PutUint32(head, versions[i])
		binary.BigEndian.PutUint16(head[4:], uint16(len(d)))
		records = append(records, head...)
		for _, e := range d {
			rec := make([]byte, 16)
			copy(rec, e.tag)
			binary.BigEndian.PutUint32(rec[8:], uint32(offsetOf(e.data)))
			binary.BigEndian.PutUint32(rec[12:], uint32(len(e.data)))
			records = append(records, rec...)
		}
	}
	out = append(out, records...)
	for _, b := range blobs {
		for len(out) < offsets[string(b)] {
			out = append(out, 0)
		}
		out = append(out, b...)
	}
	return out
}

// TestAFaceOfACollectionIsTheFontItWasMadeFrom builds a collection of two
// fixture faces and requires that each face loaded from it is the font it
// was made from: described the same, shaping, measuring and painting the
// same; that its tables are the collection's bytes and not a copy; and that
// its program is a font of its own, which loads as the original does.
func TestAFaceOfACollectionIsTheFontItWasMadeFrom(t *testing.T) {
	sources := [][]byte{harfbuzzFont(t, "ColourPaint.ttf"), harfbuzzFont(t, "BitmapInk.ttf")}
	coll := buildCollection(sources...)
	descs, err := CollectionFaces(coll)
	if err != nil || len(descs) != 2 {
		t.Fatalf("CollectionFaces: %d faces, %v", len(descs), err)
	}
	for i, src := range sources {
		want, err := Load(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := LoadCollection(coll, i)
		if err != nil {
			t.Fatalf("face %d: %v", i, err)
		}
		d := descs[i]
		wd := want.Descriptor()
		if d.Index != i || d.Name != want.Name() || d.Family != want.Family() || d.Subfamily != want.Subfamily() ||
			d.Weight != wd.Weight || d.WidthClass != wd.WidthClass || d.Italic != wd.Italic || d.Oblique != wd.Oblique {
			t.Errorf("face %d is described as %+v; loaded, it is %s %q %q %+v", i, d, want.Name(), want.Family(), want.Subfamily(), wd)
		}
		if got.Name() != want.Name() || got.Descriptor() != wd || got.NumGlyphs() != want.NumGlyphs() {
			t.Errorf("face %d: %s, %+v, and loaded alone %s, %+v", i, got.Name(), got.Descriptor(), want.Name(), wd)
		}
		for _, s := range []string{"AB", "office"} {
			g1, m1 := got.ShapeGlyphs(s)
			g2, m2 := want.ShapeGlyphs(s)
			if m1 != m2 || !reflect.DeepEqual(g1, g2) {
				t.Errorf("face %d shapes %q differently", i, s)
			}
		}
		for gid := range want.NumGlyphs() {
			a, b := &recordingPainter{}, &recordingPainter{}
			_ = got.PaintGlyph(gid, PaintOptions{}, a)
			_ = want.PaintGlyph(gid, PaintOptions{}, b)
			if !reflect.DeepEqual(a.lines, b.lines) {
				t.Errorf("face %d glyph %d paints differently", i, gid)
			}
		}
		// The tables are slices of the collection.
		head := got.sfntTables()["head"]
		if &head[0] != &coll[bytes.Index(coll, head)] {
			t.Errorf("face %d's head is a copy, not the collection's", i)
		}
		// The program is a font of its own, which loads as the original does.
		prog := got.Program()
		if font.CollectionOffsets(prog) != nil || font.SFNTTables(prog) == nil {
			t.Fatalf("face %d's program is not a single font", i)
		}
		again, err := Load(prog)
		if err != nil {
			t.Fatalf("face %d's program does not load: %v", i, err)
		}
		if again.Name() != want.Name() || again.NumGlyphs() != want.NumGlyphs() {
			t.Errorf("face %d's program loads as %s with %d glyphs", i, again.Name(), again.NumGlyphs())
		}
		got.ShapeGlyphs("AB")
		if _, err := got.Subset(); err != nil && !got.BitmapOnly() {
			t.Errorf("face %d does not subset: %v", i, err)
		}
	}
}

// The tables two faces have in common are stored once in the collection
// buildCollection writes, and read from that one place by both.
func TestFacesOfACollectionShareTheirCommonTables(t *testing.T) {
	src := harfbuzzFont(t, "ColourPaint.ttf")
	coll := buildCollection(src, src)
	a, err := LoadCollection(coll, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadCollection(coll, 1)
	if err != nil {
		t.Fatal(err)
	}
	if &a.sfntTables()["COLR"][0] != &b.sfntTables()["COLR"][0] {
		t.Error("the two faces read their COLR from two places")
	}
}

func TestACollectionIsAskedOnlyForTheFacesItHas(t *testing.T) {
	src := harfbuzzFont(t, "ColourPaint.ttf")
	coll := buildCollection(src, src)
	for _, i := range []int{-1, 2} {
		if _, err := LoadCollection(coll, i); err == nil {
			t.Errorf("face %d of two loaded", i)
		}
	}
	if _, err := Load(coll); err == nil || !bytes.Contains([]byte(err.Error()), []byte("LoadCollection")) {
		t.Errorf("Load of a collection: %v", err)
	}
	// A single font is a collection of one.
	if f, err := LoadCollection(src, 0); err != nil || f.Name() == "" {
		t.Errorf("a single font at 0: %v", err)
	}
	if _, err := LoadCollection(src, 1); err == nil {
		t.Error("a single font at 1 loaded")
	}
	if d, err := CollectionFaces(src); err != nil || len(d) != 1 {
		t.Errorf("a single font described: %d, %v", len(d), err)
	}
	// A header naming more fonts than it has room for.
	bad := append([]byte(nil), coll[:16]...)
	binary.BigEndian.PutUint32(bad[8:], 1000)
	if _, err := CollectionFaces(bad); err == nil {
		t.Error("a header of a thousand fonts in sixteen bytes was read")
	}
	if _, err := CollectionFaces([]byte("not a font")); !errors.Is(err, err) || err == nil {
		t.Error("not a font was described")
	}
}

// TestAnInstanceOfACollectionFaceIsTheInstanceOfItsFont builds a collection of
// variable faces — glyf outlines with gvar, colour paints that vary, variable
// composites and CFF2 — and requires that each face of it, cut at the ends of
// its axes and between, is the face LoadInstance cuts from the font it was
// made from: described the same, advancing, shaping, drawing and painting the
// same.
func TestAnInstanceOfACollectionFaceIsTheInstanceOfItsFont(t *testing.T) {
	noto, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatal(err)
	}
	sources := [][]byte{noto, harfbuzzFont(t, "ColourInk.ttf"), harfbuzzFont(t, "VarComposite.ttf"), harfbuzzFont(t, "CFF2Blend.otf")}
	coll := buildCollection(sources...)
	cut := 0
	for i, src := range sources {
		def, err := Load(src)
		if err != nil {
			t.Fatal(err)
		}
		axes := def.Axes()
		if len(axes) == 0 {
			t.Fatalf("source %d does not vary", i)
		}
		for _, at := range []func(Axis) float64{
			func(a Axis) float64 { return a.Min },
			func(a Axis) float64 { return a.Max },
			func(a Axis) float64 { return (a.Default + a.Max) / 2 },
		} {
			coords := map[string]float64{}
			for _, a := range axes {
				coords[a.Tag] = at(a)
			}
			want, werr := LoadInstance(src, coords)
			got, gerr := LoadCollectionInstance(coll, i, coords)
			if (werr == nil) != (gerr == nil) {
				t.Fatalf("face %d at %v: %v alone and %v from the collection", i, coords, werr, gerr)
			}
			if werr != nil {
				continue
			}
			cut++
			label := fmt.Sprintf("face %d at %v", i, coords)
			if got.Descriptor() != want.Descriptor() || got.Name() != want.Name() || got.NumGlyphs() != want.NumGlyphs() {
				t.Fatalf("%s: %+v, and alone %+v", label, got.Descriptor(), want.Descriptor())
			}
			for _, s := range []string{"AB", "office", "Á"} {
				g1, _ := got.ShapeGlyphs(s)
				g2, _ := want.ShapeGlyphs(s)
				if !reflect.DeepEqual(g1, g2) {
					t.Errorf("%s shapes %q differently", label, s)
				}
			}
			for gid := range min(want.NumGlyphs(), 120) {
				if got.GlyphAdvance(gid) != want.GlyphAdvance(gid) {
					t.Errorf("%s glyph %d advances %v, alone %v", label, gid, got.GlyphAdvance(gid), want.GlyphAdvance(gid))
				}
				var a, b []Segment
				ea := got.GlyphOutline(gid, func(s Segment) bool { a = append(a, s); return true })
				eb := want.GlyphOutline(gid, func(s Segment) bool { b = append(b, s); return true })
				if (ea == nil) != (eb == nil) || !reflect.DeepEqual(a, b) {
					t.Errorf("%s glyph %d draws differently", label, gid)
				}
				pa, pb := &recordingPainter{}, &recordingPainter{}
				_ = got.PaintGlyph(gid, PaintOptions{}, pa)
				_ = want.PaintGlyph(gid, PaintOptions{}, pb)
				if !reflect.DeepEqual(pa.lines, pb.lines) {
					t.Errorf("%s glyph %d paints differently", label, gid)
				}
			}
		}
	}
	if cut < len(sources)*2 {
		t.Fatalf("only %d instances were cut, so this test reaches too little", cut)
	}
	if _, err := LoadCollectionInstance(coll, len(sources), nil); err == nil {
		t.Error("a face past the collection's was instanced")
	}
	if f, err := LoadCollectionInstance(sources[0], 0, map[string]float64{"wght": 700}); err != nil || f.Descriptor().Weight != 700 {
		t.Errorf("a single font at 0: %v", err)
	}
}
