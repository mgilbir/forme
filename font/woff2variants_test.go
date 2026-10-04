package font

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/brotli"
	"github.com/mgilbir/forme/fonttest"
)

// WOFF 2 collections in the forms the format allows and google/woff2's encoder
// never writes, and in the forms it forbids, each checked by two decoders that
// are not this one.
//
// The collections testdata/woff2-collections holds (woff2encoded_test.go) are
// google/woff2's encoder's, and it writes one form of each thing: its tables in
// one order, every 255UInt16 in its shortest spelling, every glyf and loca
// transformed. W3C WOFF 2.0 (2024) leaves a decoder more to read than that, and
// says what it must refuse. The variants here are built by the fixture builder
// around tables lifted from those collections — transforms google/woff2 made —
// and kept in testdata/woff2-collections/variants, so that the two other
// decoders can be run on the same bytes (see make.sh there):
//
//   - google/woff2's own decoder, the format's reference implementation, whose
//     verdict and whose rebuilt faces google.txt records; and
//   - allsorts (YesLogic, Rust), a decoder written apart from google/woff2's
//     code, whose rebuilt faces allsorts.txt records. It checks nothing of the
//     collection directory's rules and refuses nothing here, so it is a second
//     opinion on what each face rebuilds to, not on what is refused.
//
// Each variant states what the format says of it: that a decoder must accept
// it, that it must refuse it, or nothing — in which case this decoder does as
// the reference does.

// woff2Variant is one variant: its name, its bytes, what the format says of
// it and where, and for one a decoder must accept, the tables each of its faces
// must rebuild to.
type woff2Variant struct {
	name    string
	data    []byte
	verdict string // "accept", "refuse" or "open"
	why     string
	faces   []map[string][]byte
}

// liftedCollection is a WOFF 2 collection as the fixture builder takes one: its
// tables in directory order, each with the bytes it rebuilds to and the
// transformed bytes the file carries; its fonts; and each font's tables as they
// rebuild.
type liftedCollection struct {
	tables []fonttest.WOFF2Table
	fonts  []fonttest.WOFF2CollectionFont
	faces  []map[string][]byte
}

func liftCollection(t *testing.T, data []byte) liftedCollection {
	t.Helper()
	r := &woff2Reader{b: data, at: woff2HeaderSize}
	tables, err := readWOFF2Directory(r, int(binary.BigEndian.Uint16(data[12:])))
	if err != nil {
		t.Fatal(err)
	}
	_, fonts, err := readWOFF2Collection(r, len(tables))
	if err != nil {
		t.Fatal(err)
	}
	last := tables[len(tables)-1]
	compressed := binary.BigEndian.Uint32(data[20:])
	body, err := brotli.Decode(data[r.at:r.at+int(compressed)], int(last.srcOffset+last.srcLength))
	if err != nil {
		t.Fatal(err)
	}
	ttc, err := DecodeWOFF2(data)
	if err != nil {
		t.Fatal(err)
	}
	var lc liftedCollection
	for i, f := range fonts {
		lc.faces = append(lc.faces, CollectionTables(ttc, i))
		lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: f.flavor, Tables: append([]int(nil), f.tables...)})
	}
	for idx, tb := range tables {
		tag := tagString(tb.tag)
		lifted := fonttest.WOFF2Table{Tag: tag}
		for i, f := range fonts {
			for _, k := range f.tables {
				if k == idx && lifted.Data == nil {
					lifted.Data = lc.faces[i][tag]
				}
			}
		}
		if lifted.Data == nil {
			t.Fatalf("table %d (%s) is no font's", idx, tag)
		}
		if tb.transformed {
			lifted.Transformed = true
			lifted.Transform = body[tb.srcOffset : tb.srcOffset+tb.srcLength]
		}
		lc.tables = append(lc.tables, lifted)
	}
	return lc
}

func tagString(tag uint32) string {
	return string([]byte{byte(tag >> 24), byte(tag >> 16), byte(tag >> 8), byte(tag)})
}

// subset is the collection of some of its fonts only, and only the tables
// those fonts name, in the order the directory has them — which keeps each
// loca right after its glyf.
func (lc liftedCollection) subset(keep ...int) liftedCollection {
	used := map[int]bool{}
	for _, i := range keep {
		for _, k := range lc.fonts[i].Tables {
			used[k] = true
		}
	}
	remap := map[int]int{}
	var out liftedCollection
	for k := range lc.tables {
		if used[k] {
			remap[k] = len(out.tables)
			out.tables = append(out.tables, lc.tables[k])
		}
	}
	for _, i := range keep {
		f := lc.fonts[i]
		idx := make([]int, len(f.Tables))
		for j, k := range f.Tables {
			idx[j] = remap[k]
		}
		out.fonts = append(out.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: idx})
		out.faces = append(out.faces, lc.faces[i])
	}
	return out
}

// join is two collections as one: the second's tables after the first's, and
// its fonts after the first's.
func (lc liftedCollection) join(other liftedCollection) liftedCollection {
	out := liftedCollection{
		tables: append(append([]fonttest.WOFF2Table(nil), lc.tables...), other.tables...),
		fonts:  append([]fonttest.WOFF2CollectionFont(nil), lc.fonts...),
		faces:  append(append([]map[string][]byte(nil), lc.faces...), other.faces...),
	}
	for _, f := range other.fonts {
		idx := make([]int, len(f.Tables))
		for i, k := range f.Tables {
			idx[i] = k + len(lc.tables)
		}
		out.fonts = append(out.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: idx})
	}
	return out
}

// moved is the collection with its directory in another order, given as each
// new position's old index, and each font's indices renumbered to match.
func (lc liftedCollection) moved(order []int) liftedCollection {
	at := make([]int, len(order))
	out := liftedCollection{faces: lc.faces}
	for to, from := range order {
		at[from] = to
		out.tables = append(out.tables, lc.tables[from])
	}
	for _, f := range lc.fonts {
		idx := make([]int, len(f.Tables))
		for i, k := range f.Tables {
			idx[i] = at[k]
		}
		out.fonts = append(out.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: idx})
	}
	return out
}

func (lc liftedCollection) build(spell func([]byte, int) []byte) []byte {
	return fonttest.WOFF2(fonttest.WOFF2Options{Tables: lc.tables, Collection: &fonttest.WOFF2Collection{
		Fonts: lc.fonts, Spell255: spell,
	}})
}

// Spellings of a 255UInt16 other than the shortest (W3C WOFF 2.0 (2024) §3.1):
// every value as a word; and a value from 253 to 508 after the code 255, and
// one from 506 to 761 after the code 254 — the two overlap, at 506 to 508 —
// with whichever code a spelling prefers where both can.
func spellWord(dst []byte, v int) []byte { return append(dst, 253, byte(v>>8), byte(v)) }

func spellPrefer255(dst []byte, v int) []byte {
	switch {
	case v < 253:
		return append(dst, byte(v))
	case v <= 508:
		return append(dst, 255, byte(v-253))
	case v <= 761:
		return append(dst, 254, byte(v-506))
	}
	return spellWord(dst, v)
}

func spellPrefer254(dst []byte, v int) []byte {
	switch {
	case v < 253:
		return append(dst, byte(v))
	case v >= 506 && v <= 761:
		return append(dst, 254, byte(v-506))
	case v <= 508:
		return append(dst, 255, byte(v-253))
	}
	return spellWord(dst, v)
}

// woff2Variants builds every variant.
func woff2Variants(t *testing.T) []woff2Variant {
	t.Helper()
	shared := liftCollection(t, readEncoded(t, "shared", ".woff2"))
	distinct := liftCollection(t, readEncoded(t, "distinct", ".woff2"))
	mixed := liftCollection(t, readEncoded(t, "mixed", ".woff2"))
	var out []woff2Variant
	add := func(name, verdict, why string, lc liftedCollection, spell func([]byte, int) []byte) {
		out = append(out, woff2Variant{name: name, data: lc.build(spell), verdict: verdict, why: why, faces: lc.faces})
	}

	// The directory backwards, glyf and loca kept together as a collection's
	// must be, and each font naming its tables backwards.
	{
		var units [][]int
		for i := 0; i < len(distinct.tables); i++ {
			if distinct.tables[i].Tag == "glyf" && i+1 < len(distinct.tables) && distinct.tables[i+1].Tag == "loca" {
				units = append(units, []int{i, i + 1})
				i++
				continue
			}
			units = append(units, []int{i})
		}
		var order []int
		for u := len(units) - 1; u >= 0; u-- {
			order = append(order, units[u]...)
		}
		lc := distinct.moved(order)
		for i := range lc.fonts {
			idx := lc.fonts[i].Tables
			for a, b := 0, len(idx)-1; a < b; a, b = a+1, b-1 {
				idx[a], idx[b] = idx[b], idx[a]
			}
		}
		add("reordered", "accept", "§4.2: the tables within each nested font can be reordered; §5.5 orders only glyf and loca", lc, nil)
	}

	add("one-font", "accept", "§4.2: a collection directory has one or more font entries", shared.subset(0), nil)

	// A TrueType face, a CFF one and a CFF2 one, each with its own flavor.
	{
		cff2, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", "CFF2Blend.otf"))
		if err != nil {
			t.Fatal(err)
		}
		tabs := SFNTTables(cff2)
		var tags []string
		for tag := range tabs {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		var plain liftedCollection
		var idx []int
		for _, tag := range tags {
			idx = append(idx, len(plain.tables))
			plain.tables = append(plain.tables, fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]})
		}
		plain.fonts = []fonttest.WOFF2CollectionFont{{Flavor: 0x4F54544F, Tables: idx}}
		plain.faces = []map[string][]byte{tabs}
		cff := -1
		for i, f := range mixed.fonts {
			if f.Flavor == 0x4F54544F {
				cff = i
			}
		}
		if cff < 0 {
			t.Fatal("mixed.woff2 has no CFF face")
		}
		add("flavors", "accept", "§4.2: each font entry states its own flavor", shared.subset(0).join(mixed.subset(cff)).join(plain), nil)
	}

	// A font whose glyf and loca are transformed beside one whose glyf and loca
	// are the same glyphs untransformed. §4.2 has an encoder transform every
	// pair, and says nothing of a decoder given one that did not.
	{
		lc := shared.subset(0)
		f := lc.fonts[0]
		// The second font's own glyf, loca and hmtx, untransformed, written at
		// the end in that order: §5.5 has a collection's loca follow its glyf
		// at once.
		own := map[string]int{}
		for _, tag := range []string{"glyf", "loca", "hmtx"} {
			for _, k := range f.Tables {
				if lc.tables[k].Tag == tag {
					own[tag] = len(lc.tables)
					lc.tables = append(lc.tables, fonttest.WOFF2Table{Tag: tag, Data: lc.tables[k].Data})
				}
			}
		}
		if len(own) != 3 {
			t.Fatal("shared.woff2's first face has no glyf, loca and hmtx")
		}
		var idx []int
		for _, k := range f.Tables {
			if at, ok := own[lc.tables[k].Tag]; ok {
				idx = append(idx, at)
				continue
			}
			idx = append(idx, k)
		}
		lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: idx})
		lc.faces = append(lc.faces, lc.faces[0])
		add("mixed-transforms", "open", "§4.2 has an encoder transform each pair and says nothing of a decoder given one that did not", lc, nil)
	}

	// Every 255UInt16 spelled each way, in a collection with enough tables for
	// each spelling to be reached: 520 small tables first, named by a third
	// font with the first font's own, and the two fonts' tables after them.
	{
		var fillers liftedCollection
		var idx []int
		for i := range 520 {
			idx = append(idx, i)
			fillers.tables = append(fillers.tables, fonttest.WOFF2Table{Tag: fmt.Sprintf("z%03d", i), Data: []byte{byte(i), byte(i >> 8), 1}})
		}
		lc := fillers.join(shared)
		lc.fonts = lc.fonts[:0]
		for _, f := range shared.fonts {
			re := make([]int, len(f.Tables))
			for i, k := range f.Tables {
				re[i] = k + len(fillers.tables)
			}
			lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: re})
		}
		third := append(append([]int(nil), idx...), lc.fonts[0].Tables...)
		lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: lc.fonts[0].Flavor, Tables: third})
		thirdFace := map[string][]byte{}
		for k, v := range shared.faces[0] {
			thirdFace[k] = v
		}
		for _, tb := range fillers.tables {
			thirdFace[tb.Tag] = tb.Data
		}
		lc.faces = []map[string][]byte{shared.faces[0], shared.faces[1], thirdFace}
		const why = "§3.1 (and the 2024 changes): a decoder must accept every spelling of a 255UInt16"
		add("spell-word", "accept", why, lc, spellWord)
		add("spell-255", "accept", why, lc, spellPrefer255)
		add("spell-254", "accept", why, lc, spellPrefer254)
	}

	// A collection's loca not immediately after its glyf.
	{
		lc := distinct
		g := -1
		for i := 0; i+2 < len(lc.tables); i++ {
			if lc.tables[i].Tag == "glyf" && lc.tables[i+1].Tag == "loca" {
				g = i
				break
			}
		}
		if g < 0 {
			t.Fatal("distinct.woff2 has no glyf followed by its loca")
		}
		order := make([]int, 0, len(lc.tables))
		for i := range lc.tables {
			if i != g+1 {
				order = append(order, i)
			}
		}
		// The loca after the table that followed it.
		order = append(order[:g+2], append([]int{g + 1}, order[g+2:]...)...)
		add("loca-apart", "refuse", "§5.5: in a collection each loca MUST immediately follow its glyf", lc.moved(order), nil)
	}

	// A transformed hmtx shared by two fonts with glyf tables of their own.
	{
		lc := shared.subset(0)
		f := lc.fonts[0]
		var idx []int
		for _, k := range f.Tables {
			tb := lc.tables[k]
			if tb.Tag == "glyf" || tb.Tag == "loca" {
				idx = append(idx, len(lc.tables))
				lc.tables = append(lc.tables, tb)
				continue
			}
			idx = append(idx, k)
		}
		lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: f.Flavor, Tables: idx})
		lc.faces = append(lc.faces, lc.faces[0])
		add("hmtx-across-glyfs", "open", "§5.4 forbids an encoder to transform an hmtx two glyfs share unless both fit it, and says nothing of a decoder", lc, nil)
	}

	// A table no font names.
	{
		lc := shared.subset(0)
		lc.tables = append(lc.tables, fonttest.WOFF2Table{Tag: "zzzz", Data: []byte("no font's")})
		add("unreferenced", "open", "§4.2 has an encoder list each table a font has, and says nothing of a decoder given one no font names", lc, nil)
	}

	// A table named twice by one font, and a font of no tables.
	{
		lc := shared.subset(0)
		lc.fonts[0].Tables = append(lc.fonts[0].Tables, lc.fonts[0].Tables[0])
		add("named-twice", "open", "§4.2 says nothing of a font naming a table twice; an sfnt directory cannot hold it", lc, nil)
	}
	{
		lc := shared.subset(0)
		lc.fonts = append(lc.fonts, fonttest.WOFF2CollectionFont{Flavor: lc.fonts[0].Flavor})
		add("no-tables", "open", "§4.2 says nothing of a font entry with no tables", lc, nil)
	}
	return out
}

// variantsDir is where the variants are kept.
var variantsDir = filepath.Join("testdata", "woff2-collections", "variants")

// TestTheWOFF2VariantsAreTheFilesKept holds the variants built here to the
// files kept, which are what the other decoders were run on. With
// WOFF2_VARIANTS=write it writes them instead.
func TestTheWOFF2VariantsAreTheFilesKept(t *testing.T) {
	for _, v := range woff2Variants(t) {
		path := filepath.Join(variantsDir, v.name+".woff2")
		if os.Getenv("WOFF2_VARIANTS") == "write" {
			if err := os.MkdirAll(variantsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, v.data, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		kept, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v; build them with WOFF2_VARIANTS=write and run make.sh", err)
		}
		if !bytes.Equal(kept, v.data) {
			t.Errorf("%s is no longer what the builder makes; rebuild the variants and run make.sh", v.name)
		}
	}
}

// otherDecoder is what another decoder made of each variant: whether it
// refused it, and each face's tables, each as its SHA-256 (google.txt) or
// whole (allsorts.txt).
type otherDecoder map[string]*otherVerdict

type otherVerdict struct {
	refused bool
	faces   []map[string]string
}

// readOtherDecoder reads google.txt or allsorts.txt: lines "file NAME", then
// "refused ..." or, for each face, "font I ..." and "table TAG VALUE".
func readOtherDecoder(t *testing.T, name string) otherDecoder {
	t.Helper()
	f, err := os.Open(filepath.Join(variantsDir, name))
	if err != nil {
		t.Fatalf("%v; run make.sh in %s", err, variantsDir)
	}
	defer f.Close()
	out := otherDecoder{}
	var cur *otherVerdict
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) == 0 || strings.HasPrefix(fs[0], "#") {
			continue
		}
		switch {
		case fs[0] == "file" && len(fs) == 2:
			cur = &otherVerdict{}
			out[strings.TrimSuffix(fs[1], ".woff2")] = cur
		case cur == nil:
			t.Fatalf("%s: %q before any file", name, sc.Text())
		case fs[0] == "refused":
			cur.refused = true
		case fs[0] == "font" && len(fs) >= 2 && !(len(fs) >= 3 && fs[2] == "refused"):
			cur.faces = append(cur.faces, map[string]string{})
		case fs[0] == "font":
			cur.refused = true
		case fs[0] == "table" && len(fs) == 3 && len(cur.faces) > 0:
			cur.faces[len(cur.faces)-1][strings.ReplaceAll(fs[1], "_", " ")] = fs[2]
		default:
			t.Fatalf("%s: %q", name, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAWOFF2CollectionVariantIsReadAsTheFormatSays decodes each variant, and
// requires that a decoder refuses what the format says it must refuse, accepts
// what it says it must accept, and where the format says nothing, does as
// google/woff2's decoder does; that each face of what is accepted rebuilds to
// the tables it was made from, glyphs compared by outline; that each such face
// is, table for table, the face google/woff2's decoder rebuilds, and the face
// allsorts rebuilds. shape/woff2collection_test.go loads the faces.
func TestAWOFF2CollectionVariantIsReadAsTheFormatSays(t *testing.T) {
	google := readOtherDecoder(t, "google.txt")
	allsorts := readOtherDecoder(t, "allsorts.txt")
	for _, v := range woff2Variants(t) {
		ttc, err := DecodeWOFF2(v.data)
		g, ok := google[v.name]
		if !ok {
			t.Fatalf("google.txt has no %s; run make.sh", v.name)
		}
		switch v.verdict {
		case "accept":
			if err != nil {
				t.Errorf("%s was refused (%v), and %s", v.name, err, v.why)
				continue
			}
			if g.refused {
				t.Errorf("%s: google/woff2's decoder refuses what the format says a decoder must accept (%s)", v.name, v.why)
			}
		case "refuse":
			if err == nil {
				t.Errorf("%s was accepted, and %s", v.name, v.why)
			}
			if !g.refused {
				t.Errorf("%s: google/woff2's decoder accepts what the format says a decoder must refuse (%s)", v.name, v.why)
			}
			continue
		case "open":
			if (err != nil) != g.refused {
				t.Errorf("%s: refused %v here and %v by google/woff2's decoder, which this decoder follows where %s", v.name, err != nil, g.refused, v.why)
			}
			if err != nil {
				continue
			}
		}
		offsets := CollectionOffsets(ttc)
		if len(offsets) != len(v.faces) {
			t.Errorf("%s: %d faces, made from %d", v.name, len(offsets), len(v.faces))
			continue
		}
		if len(g.faces) != len(v.faces) {
			t.Errorf("%s: google/woff2's decoder rebuilds %d faces of %d", v.name, len(g.faces), len(v.faces))
			continue
		}
		a := allsorts[v.name]
		for i, want := range v.faces {
			got := CollectionTables(ttc, i)
			label := fmt.Sprintf("%s face %d", v.name, i)
			sameFace(t, label, got, want)
			for tag, tb := range got {
				sum := sha256.Sum256(tb)
				ref, ok := g.faces[i][tag]
				switch {
				case !ok:
					t.Errorf("%s: google/woff2's decoder rebuilds no %s", label, tag)
				case tag == "head":
					// Its checkSumAdjustment is of the font as written, which
					// in a collection is of the font that names it last; the
					// reference records the head with it cleared.
					h := append([]byte(nil), tb...)
					copy(h[8:12], []byte{0, 0, 0, 0})
					if s := sha256.Sum256(h); hex.EncodeToString(s[:]) != ref {
						t.Errorf("%s: head differs from google/woff2's", label)
					}
				case hex.EncodeToString(sum[:]) != ref:
					t.Errorf("%s: %s differs from google/woff2's", label, tag)
				}
			}
			if a == nil || a.refused || i >= len(a.faces) {
				t.Errorf("%s: allsorts did not rebuild it", label)
				continue
			}
			theirs := map[string][]byte{}
			for tag, h := range a.faces[i] {
				b, err := hex.DecodeString(h)
				if err != nil {
					t.Fatal(err)
				}
				theirs[tag] = b
			}
			sameFace(t, label+" by allsorts", theirs, got)
		}
	}
}

// sameFace compares two rebuilds of a face: every table the same bytes, but
// head (sameHead) and glyf and loca, which are the same glyphs (sameGlyph) —
// the transform keeps a glyph's outline and not how it spells it.
func sameFace(t *testing.T, label string, got, want map[string][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d tables, want %d", label, len(got), len(want))
	}
	for tag, w := range want {
		g, ok := got[tag]
		switch {
		case !ok:
			t.Errorf("%s: no %s", label, tag)
		case tag == "head":
			if !sameHead(g, w) {
				t.Errorf("%s: head differs", label)
			}
		case tag == "glyf" || tag == "loca":
		default:
			if !bytes.Equal(g, w) {
				t.Errorf("%s: %s differs", label, tag)
			}
		}
	}
	if _, ok := want["glyf"]; ok {
		a, b := glyphs(t, got), glyphs(t, want)
		if len(a) != len(b) {
			t.Errorf("%s: %d glyphs, want %d", label, len(a), len(b))
			return
		}
		for i := range a {
			if !sameGlyph(a[i], b[i]) {
				t.Errorf("%s: glyph %d differs", label, i)
				return
			}
		}
	}
}
