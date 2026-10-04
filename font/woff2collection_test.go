package font

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/mgilbir/forme/brotli"
	"github.com/mgilbir/forme/fonttest"
)

// WOFF 2 font collections (woff2collection.go), from two sides. fontTools
// writes no WOFF 2 collection, so the tests that take one apart to malform it
// build the container with the fixture builder, around transforms a real
// encoder made: a WOFF 2 fontTools made, its glyf, loca and hmtx as
// transformed, lifted into a collection whole. And google/woff2's own encoder
// does write them: testdata/woff2-collections holds three it made, and what its
// own decoder makes of each (see make.py there).

// liftWOFF2 is a WOFF 2's tables as the fixture builder takes them: each
// table's rebuilt bytes, and for a transformed one the transformed bytes the
// file carries.
func liftWOFF2(t *testing.T, data []byte) ([]fonttest.WOFF2Table, map[string][]byte) {
	t.Helper()
	r := &woff2Reader{b: data, at: woff2HeaderSize}
	tables, err := readWOFF2Directory(r, int(binary.BigEndian.Uint16(data[12:])))
	if err != nil {
		t.Fatal(err)
	}
	last := tables[len(tables)-1]
	compressed := binary.BigEndian.Uint32(data[20:])
	body, err := brotli.Decode(data[r.at:r.at+int(compressed)], int(last.srcOffset+last.srcLength))
	if err != nil {
		t.Fatal(err)
	}
	sfnt, err := DecodeWOFF2(data)
	if err != nil {
		t.Fatal(err)
	}
	decoded := SFNTTables(sfnt)
	var out []fonttest.WOFF2Table
	for _, tb := range tables {
		tag := string([]byte{byte(tb.tag >> 24), byte(tb.tag >> 16), byte(tb.tag >> 8), byte(tb.tag)})
		lifted := fonttest.WOFF2Table{Tag: tag, Data: decoded[tag]}
		if tb.transformed {
			lifted.Transformed = true
			lifted.Transform = body[tb.srcOffset : tb.srcOffset+tb.srcLength]
		}
		out = append(out, lifted)
	}
	return out, decoded
}

// locaAfterGlyf is the tables with loca moved to right after glyf, which is
// where a collection has to have it (W3C WOFF 2.0 (2024) §5.5); a single font
// may have tables between them.
func locaAfterGlyf(tables []fonttest.WOFF2Table) []fonttest.WOFF2Table {
	var loca *fonttest.WOFF2Table
	var out []fonttest.WOFF2Table
	for _, tb := range tables {
		if tb.Tag == "loca" {
			loca = &tb
			continue
		}
		out = append(out, tb)
	}
	if loca == nil {
		return out
	}
	for i, tb := range out {
		if tb.Tag == "glyf" {
			return append(out[:i+1], append([]fonttest.WOFF2Table{*loca}, out[i+1:]...)...)
		}
	}
	return append(out, *loca)
}

// plainTables is a small font's tables, none transformed, in tag order but for
// loca, which is right after glyf as a collection has it.
func plainTables(r rune) ([]fonttest.WOFF2Table, map[string][]byte) {
	tabs := SFNTTables(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: r, Advance: 600, HasShape: true}},
	}))
	var tags []string
	for tag := range tabs {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var out []fonttest.WOFF2Table
	for _, tag := range tags {
		out = append(out, fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]})
	}
	return locaAfterGlyf(out), tabs
}

// sameTables compares a decoded font's tables to the ones it was made from,
// head's checkSumAdjustment aside: it is of the font as written, which in a
// collection is not the font alone.
func sameTables(t *testing.T, label string, got, want map[string][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d tables, want %d", label, len(got), len(want))
	}
	for tag, w := range want {
		g, ok := got[tag]
		if tag == "head" && ok && len(g) == len(w) && len(g) >= 12 {
			g = append(append([]byte(nil), g[:8]...), g[12:]...)
			w = append(append([]byte(nil), w[:8]...), w[12:]...)
		}
		if !ok || !bytes.Equal(g, w) {
			t.Errorf("%s: %s differs", label, tag)
		}
	}
}

// TestAWOFF2CollectionIsTheCollectionItWasMadeFrom wraps three fonts in a WOFF 2
// collection: a real one whose glyf, loca and hmtx are transformed, the same
// font with a name table of its own and every other table shared, and a small
// font sharing nothing. Each font decoded is the font it was made from; the
// tables two fonts share are written once; and a font's head adjustment makes
// it sum as the format says.
func TestAWOFF2CollectionIsTheCollectionItWasMadeFrom(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "woff2", "HasubiMono-hmtx-dropped.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	real, realTables := liftWOFF2(t, src)
	real = locaAfterGlyf(real)
	transformed := 0
	for _, tb := range real {
		if tb.Transformed {
			transformed++
		}
	}
	if transformed < 3 {
		t.Fatalf("the fixture transforms %d tables, so this test does not reach glyf, loca and hmtx", transformed)
	}
	plain, plainTabs := plainTables('Z')

	dir := append([]fonttest.WOFF2Table(nil), real...)
	first := make([]int, len(real))
	second := make([]int, len(real))
	name := []byte("a name table of its own")
	for i, tb := range real {
		first[i], second[i] = i, i
		if tb.Tag == "name" {
			second[i] = len(dir)
			dir = append(dir, fonttest.WOFF2Table{Tag: "name", Data: name})
		}
	}
	var third []int
	for _, tb := range plain {
		third = append(third, len(dir))
		dir = append(dir, tb)
	}
	coll := fonttest.WOFF2(fonttest.WOFF2Options{Tables: dir, Collection: &fonttest.WOFF2Collection{
		Fonts: []fonttest.WOFF2CollectionFont{{Tables: first}, {Tables: second}, {Tables: third}},
	}})

	got, err := DecodeWOFF(coll)
	if err != nil {
		t.Fatal(err)
	}
	if offsets := CollectionOffsets(got); len(offsets) != 3 {
		t.Fatalf("decoded to %d fonts", len(offsets))
	}
	withName := map[string][]byte{}
	for tag, b := range realTables {
		withName[tag] = b
	}
	withName["name"] = name
	fonts := []map[string][]byte{CollectionTables(got, 0), CollectionTables(got, 1), CollectionTables(got, 2)}
	sameTables(t, "the transformed font", fonts[0], realTables)
	sameTables(t, "the font sharing it", fonts[1], withName)
	sameTables(t, "the plain font", fonts[2], plainTabs)
	for tag := range realTables {
		if tag != "name" && &fonts[0][tag][0] != &fonts[1][tag][0] {
			t.Errorf("%s is written twice, once for each font that has it", tag)
		}
	}
	// Once in the file, too, and not once named and once left behind.
	if n := bytes.Count(got, realTables["glyf"]); n != 1 {
		t.Errorf("the shared glyf is in the collection %d times", n)
	}
	// The plain font's head is its own, so its adjustment is of it alone.
	at := CollectionOffsets(got)[2]
	n := int(binary.BigEndian.Uint16(got[at+4:]))
	sum := computeULongSum(got[at : at+12+16*n])
	for i := 0; i < n; i++ {
		rec := at + 12 + 16*i
		off, length := binary.BigEndian.Uint32(got[rec+8:]), binary.BigEndian.Uint32(got[rec+12:])
		sum += computeULongSum(got[off : off+length])
	}
	if sum != 0xB1B0AFBA {
		t.Errorf("the plain font sums to %#x", sum)
	}
}

// TestAMalformedWOFF2CollectionIsRefused states each thing a collection
// directory can get wrong, beside the same collection with nothing wrong.
func TestAMalformedWOFF2CollectionIsRefused(t *testing.T) {
	plain, _ := plainTables('A')
	all := make([]int, len(plain))
	glyf, loca := -1, -1
	for i, tb := range plain {
		all[i] = i
		switch tb.Tag {
		case "glyf":
			glyf = i
		case "loca":
			loca = i
		}
	}
	intp := func(v int) *int { return &v }
	build := func(dir []fonttest.WOFF2Table, c fonttest.WOFF2Collection) []byte {
		return fonttest.WOFF2(fonttest.WOFF2Options{Tables: dir, Collection: &c})
	}
	one := fonttest.WOFF2CollectionFont{Tables: all}
	if _, err := DecodeWOFF2(build(plain, fonttest.WOFF2Collection{Fonts: []fonttest.WOFF2CollectionFont{one, one}})); err != nil {
		t.Fatalf("the control collection was refused: %v", err)
	}
	without := func(skip int) []int {
		var out []int
		for _, i := range all {
			if i != skip {
				out = append(out, i)
			}
		}
		return out
	}
	// A second loca, so that two fonts can share the glyf and not its loca.
	twoLocas := append(append([]fonttest.WOFF2Table(nil), plain...), plain[loca])
	otherLoca := append(without(loca), len(plain))
	for _, tc := range []struct {
		what string
		dir  []fonttest.WOFF2Table
		c    fonttest.WOFF2Collection
	}{
		{"no fonts", plain, fonttest.WOFF2Collection{}},
		{"more fonts than the directory holds", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{one}, StatedNumFonts: intp(1000)}},
		{"a version the format does not define", plain, fonttest.WOFF2Collection{
			Version: 0x00030000, Fonts: []fonttest.WOFF2CollectionFont{one}}},
		{"a table the directory does not have", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: append(without(-1), 99)}}}},
		{"a font of no tables", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: nil}}}},
		{"more tables than the font names", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: all, StatedNumTables: intp(len(all) + 40)}}}},
		{"a table named twice", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: append(without(-1), all[0])}}}},
		{"glyf without its loca", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: without(loca)}}}},
		{"loca without its glyf", plain, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: without(glyf)}}}},
		{"a glyf shared without its loca", twoLocas, fonttest.WOFF2Collection{
			Fonts: []fonttest.WOFF2CollectionFont{{Tables: all}, {Tables: otherLoca}}}},
	} {
		if _, err := DecodeWOFF2(build(tc.dir, tc.c)); err == nil {
			t.Errorf("%s was accepted", tc.what)
		}
	}
	// And cut short anywhere in the collection directory.
	whole := build(plain, fonttest.WOFF2Collection{Fonts: []fonttest.WOFF2CollectionFont{one, one}})
	for n := woff2HeaderSize; n < len(whole); n += 7 {
		cut := append([]byte(nil), whole[:n]...)
		binary.BigEndian.PutUint32(cut[8:], uint32(len(cut)))
		if _, err := DecodeWOFF2(cut); err == nil {
			t.Errorf("cut to %d of %d bytes and accepted", n, len(whole))
		}
	}
}
