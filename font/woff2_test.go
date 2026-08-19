package font

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/brotli"
	"github.com/mgilbir/forme/fonttest"
)

// brotliDecodeForTest is the decompressor the rebuilding helper needs, named
// here so the import reads as what it is.
func brotliDecodeForTest(src []byte, limit int) ([]byte, error) {
	return brotli.Decode(src, limit)
}

// What WOFF 2 decoding is checked against.
//
// A rebuilt font is right or it is not: one wrong byte in glyf is a wrong
// glyph, one wrong offset in loca is a font that will not load, and a wrong
// left side bearing moves every letter after it. So the outer test compares
// against google/woff2 — the format's own reference decoder, which shares no
// code with this and was run on the same bytes — and requires agreement byte
// for byte, checksums included.
//
// Everything the reference cannot reach is checked another way. The container
// is built here, by fonttest, so that a directory can be malformed on purpose.
// And two properties are asserted directly, because they say the font is a font
// rather than merely the bytes something else also produced: every table's
// recorded checksum is the sum over its own bytes, and the whole font sums to
// the constant the format defines.

func woff2Testdata(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "woff2", "*.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no WOFF 2 fonts in testdata/woff2")
	}
	sort.Strings(files)
	return files
}

// TestAWOFF2RebuildsToWhatTheReferenceSays.
func TestAWOFF2RebuildsToWhatTheReferenceSays(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "woff2", "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	expected := map[string][2]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("expected.txt: %q is not <file> <length> <digest>", line)
		}
		expected[fields[0]] = [2]string{fields[1], fields[2]}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}

	for _, path := range woff2Testdata(t) {
		name := filepath.Base(path)
		want, ok := expected[name]
		if !ok {
			t.Errorf("%s is in testdata/woff2 and not in expected.txt", name)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeWOFF(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strconv.Itoa(len(got)) != want[0] {
			t.Errorf("%s: rebuilt to %d bytes, and the reference decoder gets %s",
				name, len(got), want[0])
			continue
		}
		sum := sha256.Sum256(got)
		if d := hex.EncodeToString(sum[:])[:16]; d != want[1] {
			t.Errorf("%s: rebuilt to a font hashing to %s, and the reference "+
				"decoder's hashes to %s", name, d, want[1])
		}
	}
	if len(expected) < 2 {
		t.Errorf("%d fonts in expected.txt; this is meant to cover the transforms", len(expected))
	}
}

// TestDroppingTheLeftSideBearingsChangesNothing is the hmtx transform, and it is
// checked as the property it is rather than against a digest.
//
// The two fonts in testdata are the same face. One carries every left side
// bearing; the other has them dropped, because they are the left edge of each
// outline and can be read back off the glyphs. They must rebuild to the same
// bytes, and if the reconstruction is off by one glyph — which is the way this
// goes wrong — every letter after that point moves.
//
// No compressor in circulation emits the hmtx transform, which is why the
// second font was built rather than found. What makes it a real fixture and not
// a restatement of this decoder is that the reference decoder was run on it
// first, and produced the other font exactly.
func TestDroppingTheLeftSideBearingsChangesNothing(t *testing.T) {
	kept, err := os.ReadFile(filepath.Join("testdata", "woff2", "HasubiMono-Regular.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	dropped, err := os.ReadFile(filepath.Join("testdata", "woff2", "HasubiMono-hmtx-dropped.woff2"))
	if err != nil {
		t.Fatal(err)
	}
	// The two files are not the same, or this proves nothing.
	if string(kept) == string(dropped) {
		t.Fatal("the two fixtures are the same file")
	}
	a, err := DecodeWOFF(kept)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DecodeWOFF(dropped)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		at := 0
		for at < len(a) && at < len(b) && a[at] == b[at] {
			at++
		}
		t.Fatalf("the font that dropped its bearings rebuilt to %d bytes and the "+
			"one that kept them to %d, differing at byte %d", len(b), len(a), at)
	}
	// And the bearings really were dropped, so the fixture is the case it says.
	tabs := SFNTTables(a)
	if len(tabs["hmtx"]) == 0 {
		t.Fatal("the rebuilt font has no hmtx table")
	}
}

// TestARebuiltFontAddsUp checks the two things a font says about itself.
//
// Neither is carried by a WOFF 2 — both are computed as it is rebuilt — so
// getting them wrong produces a font that draws correctly and that a strict
// reader refuses. They are also the only assertion here that would survive the
// reference decoder itself being wrong.
func TestARebuiltFontAddsUp(t *testing.T) {
	for _, path := range woff2Testdata(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sfnt, err := DecodeWOFF(data)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
			continue
		}
		n := int(binary.BigEndian.Uint16(sfnt[4:]))
		for i := 0; i < n; i++ {
			rec := 12 + 16*i
			tag := string(sfnt[rec : rec+4])
			want := binary.BigEndian.Uint32(sfnt[rec+4:])
			off := binary.BigEndian.Uint32(sfnt[rec+8:])
			length := binary.BigEndian.Uint32(sfnt[rec+12:])
			if uint64(off)+uint64(length) > uint64(len(sfnt)) {
				t.Errorf("%s: %s lies outside the font", filepath.Base(path), tag)
				continue
			}
			body := sfnt[off : off+length]
			// head is the exception: the four bytes at offset 8 are the
			// adjustment, and the checksum in its record is of the table with
			// them zeroed.
			if tag == "head" {
				body = append([]byte(nil), body...)
				binary.BigEndian.PutUint32(body[8:], 0)
			}
			if got := computeULongSum(body); got != want {
				t.Errorf("%s: %s records checksum %08x and its bytes sum to %08x",
					filepath.Base(path), tag, want, got)
			}
		}
		// The whole font, padding included, sums to a constant. That is what
		// head's checkSumAdjustment is for and the only thing it is for.
		if got := computeULongSum(sfnt); got != 0xB1B0AFBA {
			t.Errorf("%s: the whole font sums to %08x, want b1b0afba",
				filepath.Base(path), got)
		}
	}
}

// TestARebuiltFontIsAFont: the outlines, the metrics and the character map are
// all there and readable by the parser that reads an ordinary file.
func TestARebuiltFontIsAFont(t *testing.T) {
	for _, path := range woff2Testdata(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sfnt, err := DecodeWOFF(data)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
			continue
		}
		prog := ParseSFNT(sfnt, generousCmapWork)
		if prog == nil {
			t.Errorf("%s: the rebuilt font does not parse", filepath.Base(path))
			continue
		}
		tabs := SFNTTables(sfnt)
		for _, tag := range []string{"glyf", "loca", "head", "hhea", "hmtx", "maxp", "cmap"} {
			if len(tabs[tag]) == 0 {
				t.Errorf("%s: the rebuilt font has no %s", filepath.Base(path), tag)
			}
		}
		// loca must address the whole of glyf and nothing beyond it, which is
		// the one relationship the glyf transform can get wrong silently.
		head, loca, glyf := tabs["head"], tabs["loca"], tabs["glyf"]
		long := binary.BigEndian.Uint16(head[50:]) != 0
		count := len(loca)/2 - 1
		if long {
			count = len(loca)/4 - 1
		}
		numGlyphs := int(binary.BigEndian.Uint16(tabs["maxp"][4:]))
		if count != numGlyphs {
			t.Errorf("%s: loca holds %d glyphs and maxp says %d",
				filepath.Base(path), count, numGlyphs)
		}
		at := func(i int) int {
			if long {
				return int(binary.BigEndian.Uint32(loca[4*i:]))
			}
			return int(binary.BigEndian.Uint16(loca[2*i:])) * 2
		}
		prev := 0
		for i := 0; i <= count; i++ {
			o := at(i)
			if o < prev {
				t.Errorf("%s: loca runs backwards at glyph %d", filepath.Base(path), i)
				break
			}
			if o > len(glyf) {
				t.Errorf("%s: loca points past the end of glyf at glyph %d",
					filepath.Base(path), i)
				break
			}
			prev = o
		}
		if prev != len(glyf) {
			t.Errorf("%s: loca ends at %d and glyf is %d bytes",
				filepath.Base(path), prev, len(glyf))
		}
	}
}

// syntheticWOFF2 wraps a synthetic font, table by table, with nothing
// transformed — which the format allows for every table and requires a
// particular version number to say.
func syntheticWOFF2(t *testing.T, opts fonttest.WOFF2Options) ([]byte, map[string][]byte) {
	t.Helper()
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 600, HasShape: true},
			{Rune: 'B', Advance: 700, HasShape: true},
			{Rune: ' ', Advance: 250},
			{Rune: 'C', Advance: 500, HasShape: true},
		},
	})
	tabs := SFNTTables(sfnt)
	var tags []string
	for tag := range tabs {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		opts.Tables = append(opts.Tables, fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]})
	}
	return fonttest.WOFF2(opts), tabs
}

// TestEveryTableComesBackAsItWentIn.
//
// The container is what this exercises: the header, the directory with its
// lengths written seven bits at a time, the Brotli stream, and the sfnt built
// back around it. Nothing is transformed, which is a real shape a WOFF 2 takes
// — a font whose glyf the encoder chose not to re-encode declares exactly this
// — and it is the shape that leaves the tables comparable to what went in.
func TestEveryTableComesBackAsItWentIn(t *testing.T) {
	built, want := syntheticWOFF2(t, fonttest.WOFF2Options{})
	sfnt, err := DecodeWOFF(built)
	if err != nil {
		t.Fatal(err)
	}
	got := SFNTTables(sfnt)
	if len(got) != len(want) {
		t.Errorf("%d tables came back and %d went in", len(got), len(want))
	}
	for tag, body := range want {
		switch {
		case got[tag] == nil:
			t.Errorf("%s did not come back", tag)
		case tag == "head":
			// Every byte but the four holding the checksum of the whole font,
			// which is of the font being built and cannot be the old one's.
			if string(got[tag][:8]) != string(body[:8]) || string(got[tag][12:]) != string(body[12:]) {
				t.Errorf("head came back changed outside its checksum")
			}
		case string(got[tag]) != string(body):
			t.Errorf("%s came back as %d bytes, want %d", tag, len(got[tag]), len(body))
		}
	}
}

// TestATagMayBeSpelledOutOrNamedByIndex. Sixty-three tags have a number and the
// rest are written in full, and a font is free to write any of them in full.
func TestATagMayBeSpelledOutOrNamedByIndex(t *testing.T) {
	byIndex, _ := syntheticWOFF2(t, fonttest.WOFF2Options{})
	spelled, _ := syntheticWOFF2(t, fonttest.WOFF2Options{SpellOutTags: true})
	if len(spelled) <= len(byIndex) {
		t.Errorf("spelling the tags out made the file no larger (%d against %d); "+
			"the fixture is not building what it says", len(spelled), len(byIndex))
	}
	a, err := DecodeWOFF(byIndex)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DecodeWOFF(spelled)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("the two spellings of the same font rebuilt differently")
	}
}

// TestTheRecordsAreSortedAndTheTablesAreNotMoved.
//
// The directory of an sfnt is sorted by tag because a reader may binary-search
// it. The tables themselves stay in the order the WOFF 2 named them, and they
// have to: glyf writes loca as it goes, and hmtx needs what glyf found, so a
// decoder that laid the tables out in tag order would be putting loca before
// glyf.
func TestTheRecordsAreSortedAndTheTablesAreNotMoved(t *testing.T) {
	built, _ := syntheticWOFF2(t, fonttest.WOFF2Options{})
	sfnt, err := DecodeWOFF(built)
	if err != nil {
		t.Fatal(err)
	}
	n := int(binary.BigEndian.Uint16(sfnt[4:]))
	for i := 1; i < n; i++ {
		prev := string(sfnt[12+16*(i-1) : 12+16*(i-1)+4])
		this := string(sfnt[12+16*i : 12+16*i+4])
		if this <= prev {
			t.Errorf("the records run %s then %s; they must be in tag order", prev, this)
		}
	}
	// The binary-search hints have to match the count, or a reader that uses
	// them looks in the wrong place.
	searchRange := int(binary.BigEndian.Uint16(sfnt[6:]))
	entrySelector := int(binary.BigEndian.Uint16(sfnt[8:]))
	rangeShift := int(binary.BigEndian.Uint16(sfnt[10:]))
	if 1<<entrySelector > n || 1<<(entrySelector+1) <= n {
		t.Errorf("entrySelector is %d for %d tables", entrySelector, n)
	}
	if searchRange != 16<<entrySelector {
		t.Errorf("searchRange is %d and entrySelector %d", searchRange, entrySelector)
	}
	if rangeShift != 16*n-searchRange {
		t.Errorf("rangeShift is %d for %d tables and a search range of %d",
			rangeShift, n, searchRange)
	}
}

// TestAMalformedWOFF2IsRefused.
//
// Each of these is a file a decoder could read some distance into and then
// produce something from, and each is refused instead. The list is not
// defensive programming: a font arrives from the network, and every one of
// these was written by choosing a field and lying about it.
func TestAMalformedWOFF2IsRefused(t *testing.T) {
	for _, tc := range []struct {
		what string
		opts fonttest.WOFF2Options
	}{
		{"a signature that is not wOF2", fonttest.WOFF2Options{Signature: 0x774F4646}},
		{"a length that is not the file's", fonttest.WOFF2Options{StatedLength: 99999}},
		{"a length short of the file's", fonttest.WOFF2Options{StatedLength: 12}},
		{"more tables than it carries", fonttest.WOFF2Options{NumTables: intp(200)}},
		{"no tables at all", fonttest.WOFF2Options{NumTables: intp(0)}},
		{"more tables than an sfnt can address", fonttest.WOFF2Options{NumTables: intp(65535)}},
		{"font data that is not a Brotli stream",
			fonttest.WOFF2Options{Garbage: []byte("this is not a Brotli stream at all")}},
		{"font data that decompresses to nothing",
			fonttest.WOFF2Options{Garbage: []byte{0x06}}}, // an empty stream
		{"a font collection", fonttest.WOFF2Options{Flavor: 0x74746366}},
	} {
		built, _ := syntheticWOFF2(t, tc.opts)
		if _, err := DecodeWOFF(built); err == nil {
			t.Errorf("%s was accepted", tc.what)
		}
	}

	// And the same fixture with nothing wrong with it is accepted, so the list
	// above is not passing because the builder makes nothing readable.
	ok, _ := syntheticWOFF2(t, fonttest.WOFF2Options{})
	if _, err := DecodeWOFF(ok); err != nil {
		t.Fatalf("the control fixture was refused: %v", err)
	}
}

func intp(v int) *int { return &v }

// TestATableThisCannotRebuildIsRefused. Three tables have a transform and the
// rest have none, so a directory that claims one for anything else is
// describing bytes this cannot turn back into a table. Copying them through as
// though the claim were not there would put the transformed form in the font.
func TestATableThisCannotRebuildIsRefused(t *testing.T) {
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 600, HasShape: true}},
	})
	tabs := SFNTTables(sfnt)
	var list []fonttest.WOFF2Table
	var tags []string
	for tag := range tabs {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		w := fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]}
		if tag == "cmap" {
			// A transform for a table that has none.
			w.Transform = tabs[tag]
			w.Version = 1
		}
		list = append(list, w)
	}
	built := fonttest.WOFF2(fonttest.WOFF2Options{Tables: list})
	if _, err := DecodeWOFF(built); err == nil {
		t.Error("a cmap claiming a transform was accepted")
	}
}

// TestGlyfAndLocaGoTogether. One without the other is not a font, and
// transforming one without the other is not a font either — the transform is of
// the pair, since loca is nothing but the offsets the glyf transform produces.
func TestGlyfAndLocaGoTogether(t *testing.T) {
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 600, HasShape: true}},
	})
	tabs := SFNTTables(sfnt)
	build := func(drop string, transform string) []byte {
		var list []fonttest.WOFF2Table
		var tags []string
		for tag := range tabs {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		for _, tag := range tags {
			if tag == drop {
				continue
			}
			w := fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]}
			if tag == transform {
				w.Transformed = true
				w.Transform = tabs[tag]
			}
			list = append(list, w)
		}
		return fonttest.WOFF2(fonttest.WOFF2Options{Tables: list})
	}
	for _, tc := range []struct{ what, drop, transform string }{
		{"glyf without loca", "loca", ""},
		{"loca without glyf", "glyf", ""},
		{"a transformed glyf beside an untransformed loca", "", "glyf"},
		{"a transformed loca beside an untransformed glyf", "", "loca"},
	} {
		if _, err := DecodeWOFF(build(tc.drop, tc.transform)); err == nil {
			t.Errorf("%s was accepted", tc.what)
		}
	}
	// The control: neither dropped and neither transformed.
	if _, err := DecodeWOFF(build("", "")); err != nil {
		t.Errorf("the control fixture was refused: %v", err)
	}
}

// TestWOFF2RebuildsAsTheReferenceDoes is the broad check, over every WOFF 2 in
// the CSS Working Group's suite rather than the two kept beside this file.
//
// The two in testdata are one face and one derivation of it, chosen because
// between them they reach every branch of the transform. The suite's are four
// unrelated faces — an Arabic one with twelve hundred composite glyphs, a
// monospace one that is mostly composites, a Mongolian one carrying tables the
// format has no number for, and one with no composites at all — and what they
// add is that they were made by encoders this had nothing to do with.
//
// Entries are keyed by the file's own hash, so a font the suite replaces
// upstream drops out rather than failing. That could erode to nothing, so the
// floor below is what keeps it honest.
func TestWOFF2RebuildsAsTheReferenceDoes(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "woff2-suite.txt"))
	if err != nil {
		t.Fatalf("the oracle is missing: %v", err)
	}
	defer f.Close()

	type expect struct {
		length int
		sum    string
	}
	want := map[string]expect{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Fields(line)
		if len(p) != 3 {
			t.Fatalf("cannot read %q", line)
		}
		n, err := strconv.Atoi(p[1])
		if err != nil {
			t.Fatalf("cannot read the length in %q", line)
		}
		want[p[0]] = expect{n, p[2]}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	dir := os.Getenv("WPT_TESTS")
	if dir == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) to check WOFF 2 against the reference decoder")
	}
	files, missing := 0, 0
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(p) != ".woff2" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		key := hex.EncodeToString(sum[:])[:16]
		exp, ok := want[key]
		if !ok {
			missing++
			return nil
		}
		files++
		got, err := DecodeWOFF(data)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			return nil
		}
		if len(got) != exp.length {
			t.Errorf("%s: rebuilt to %d bytes and the reference decoder gets %d",
				filepath.Base(p), len(got), exp.length)
			return nil
		}
		out := sha256.Sum256(got)
		if d := hex.EncodeToString(out[:])[:16]; d != exp.sum {
			t.Errorf("%s: rebuilt to a font hashing to %s and the reference "+
				"decoder's hashes to %s", filepath.Base(p), d, exp.sum)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d fonts compared against the reference decoder, %d not in the oracle", files, missing)
	if files < 4 {
		t.Errorf("only %d fonts were compared; the oracle has %d entries and this "+
			"check is worth nothing if it stops finding them", files, len(want))
	}
}

// FuzzDecodeWOFF2. A webfont arrives from the network and this is the first
// thing that reads it, so what holds for arbitrary bytes is what matters: it
// finishes, it does not read past the end of anything, and it gives the same
// answer twice.
func FuzzDecodeWOFF2(f *testing.F) {
	files, err := filepath.Glob(filepath.Join("testdata", "woff2", "*.woff2"))
	if err != nil {
		f.Fatal(err)
	}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	// The synthetic container as well, which is far smaller and so far easier
	// for a fuzzer to make progress from.
	sfnt := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'A', Advance: 600, HasShape: true}},
	})
	tabs := SFNTTables(sfnt)
	var tags []string
	for tag := range tabs {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	var list []fonttest.WOFF2Table
	for _, tag := range tags {
		list = append(list, fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]})
	}
	f.Add(fonttest.WOFF2(fonttest.WOFF2Options{Tables: list}))
	f.Add([]byte("wOF2"))

	f.Fuzz(func(t *testing.T, src []byte) {
		got, err := DecodeWOFF2(src)
		if err != nil {
			if len(got) != 0 {
				t.Fatalf("refused with %v and returned %d bytes", err, len(got))
			}
			return
		}
		if len(got) > maxWOFFSfntSize {
			t.Fatalf("rebuilt %d bytes, past the cap of %d", len(got), maxWOFFSfntSize)
		}
		// A rebuilt font that says it holds tables must hold them: every record
		// has to address bytes that are there, or something downstream reads
		// past the end of the slice.
		if len(got) < 12 {
			t.Fatalf("rebuilt a %d-byte font", len(got))
		}
		n := int(binary.BigEndian.Uint16(got[4:]))
		if 12+16*n > len(got) {
			t.Fatalf("%d records do not fit in %d bytes", n, len(got))
		}
		for i := 0; i < n; i++ {
			rec := 12 + 16*i
			off := uint64(binary.BigEndian.Uint32(got[rec+8:]))
			length := uint64(binary.BigEndian.Uint32(got[rec+12:]))
			if off+length > uint64(len(got)) {
				t.Fatalf("record %d addresses %d..%d of %d bytes", i, off, off+length, len(got))
			}
		}
		again, err := DecodeWOFF2(src)
		if err != nil {
			t.Fatalf("rebuilt once and then failed with %v", err)
		}
		if string(again) != string(got) {
			t.Fatalf("rebuilding twice gave %d and %d bytes", len(got), len(again))
		}
	})
}

// TestALengthWrittenTheLongWayIsRefused.
//
// A table's length is written seven bits to a byte, and a number written with
// more bytes than it needs has two spellings. The format forbids the longer
// one, and so does this: two files that differ byte for byte and describe the
// same font is how a signature over a font stops meaning anything.
func TestALengthWrittenTheLongWayIsRefused(t *testing.T) {
	for _, tc := range []struct {
		what string
		in   []byte
		want uint32
		ok   bool
	}{
		{"one byte", []byte{0x7f}, 127, true},
		{"two bytes", []byte{0x81, 0x00}, 128, true},
		{"the largest there is", []byte{0x8f, 0xff, 0xff, 0xff, 0x7f}, 0xffffffff, true},
		{"zero", []byte{0x00}, 0, true},
		{"a zero written in two bytes", []byte{0x80, 0x00}, 0, false},
		// Six bytes is refused, though not by the byte count: five continuation
		// bytes after a non-zero first already carry the value past what will
		// fit, so the overflow check gets there first.
		{"six bytes", []byte{0x81, 0x80, 0x80, 0x80, 0x80, 0x00}, 0, false},
		{"a number past thirty-two bits", []byte{0x9f, 0xff, 0xff, 0xff, 0x7f}, 0, false},
		{"a number that never ends", []byte{0x81}, 0, false},
		{"nothing at all", []byte{}, 0, false},
	} {
		r := &woff2Reader{b: tc.in}
		got, ok := r.base128()
		if ok != tc.ok {
			t.Errorf("%s: accepted=%v, want %v", tc.what, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("%s: read %d, want %d", tc.what, got, tc.want)
		}
	}
}

// TestTheBearingsThatWereKeptAreTheOnesTheFlagsName.
//
// The hmtx transform drops one or both runs of left side bearings and says
// which in a flag byte, and the fonts in testdata drop both — so which run is
// which is not visible in them. It is here, because a decoder that read the
// flags the other way round would take the bearings of the glyphs that share an
// advance from the outlines and the rest from the file, and every letter in the
// font would be in the wrong place by a small amount.
func TestTheBearingsThatWereKeptAreTheOnesTheFlagsName(t *testing.T) {
	// Four glyphs, three of which have an advance of their own. The bearings
	// stored in the file and the left edges of the outlines are deliberately
	// different, so which one came out is visible.
	f := &woff2Font{numGlyphs: 4, numHMetrics: 3, xMins: []int16{-1, -2, -3, -4}}
	widths := []byte{0, 10, 0, 20, 0, 30}
	stored := func(vs ...int16) []byte {
		var b []byte
		for _, v := range vs {
			b = binary.BigEndian.AppendUint16(b, uint16(v))
		}
		return b
	}
	for _, tc := range []struct {
		what     string
		flags    byte
		rest     []byte
		bearings []int16
	}{
		// Bit 0 set means the bearings of the glyphs with their own advance
		// are gone; the ones after them are still in the file.
		{"the proportional run dropped", 0x01, stored(70),
			[]int16{-1, -2, -3, 70}},
		// Bit 1 set means the other way round.
		{"the run that shares an advance dropped", 0x02, stored(40, 50, 60),
			[]int16{40, 50, 60, -4}},
		{"both dropped", 0x03, nil, []int16{-1, -2, -3, -4}},
	} {
		src := append([]byte{tc.flags}, append(append([]byte(nil), widths...), tc.rest...)...)
		out, _, err := reconstructHmtx(nil, src, f)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if len(out) != 2*4+2*3 {
			t.Errorf("%s: %d bytes, want %d", tc.what, len(out), 2*4+2*3)
			continue
		}
		// The table interleaves an advance and a bearing for the first three
		// glyphs and then bearings alone.
		at := 0
		for i := 0; i < 4; i++ {
			if i < 3 {
				if got := binary.BigEndian.Uint16(out[at:]); got != uint16(10*(i+1)) {
					t.Errorf("%s: glyph %d advances %d, want %d", tc.what, i, got, 10*(i+1))
				}
				at += 2
			}
			if got := int16(binary.BigEndian.Uint16(out[at:])); got != tc.bearings[i] {
				t.Errorf("%s: glyph %d bears %d, want %d", tc.what, i, got, tc.bearings[i])
			}
			at += 2
		}
	}

	// A table that drops neither run is not transformed, and one that says it
	// is transformed while dropping nothing is describing itself wrongly. The
	// bytes are all there — three advances and four bearings — so refusing it
	// is about what it claims and not about running out.
	whole := append([]byte{0x00}, widths...)
	whole = append(whole, stored(1, 2, 3, 4)...)
	if _, _, err := reconstructHmtx(nil, whole, f); err == nil {
		t.Error("an hmtx that drops neither run was accepted as transformed")
	}
	// The reserved bits mean something this does not implement.
	for _, flags := range []byte{0x04, 0x08, 0x40, 0x80, 0xfc} {
		src := append([]byte{flags | 0x03}, widths...)
		if _, _, err := reconstructHmtx(nil, src, f); err == nil {
			t.Errorf("an hmtx with reserved bit pattern %08b was accepted", flags)
		}
	}
}

// TestAComponentIsMeasuredByItsFlags.
//
// A composite glyph's components are copied through untouched, so the only
// thing to get right is where they end — and that is stated by flags inside
// them. The transform is what makes it matter: the components arrive in one
// stream with no lengths, so a component measured wrong takes the next glyph's
// with it.
//
// Two of the shapes below appear in no font in any corpus this is measured
// against, which is exactly why they are here.
func TestAComponentIsMeasuredByItsFlags(t *testing.T) {
	// One component: flags, a glyph index, then arguments and a transform
	// according to the flags. The bytes after it must not be read.
	component := func(flags uint16, extra int) []byte {
		b := binary.BigEndian.AppendUint16(nil, flags)
		b = binary.BigEndian.AppendUint16(b, 7) // the component's glyph
		return append(b, make([]byte, extra)...)
	}
	for _, tc := range []struct {
		what  string
		flags uint16
		extra int
	}{
		{"byte arguments and no transform", 0, 2},
		{"word arguments and no transform", compArgsAreWords, 4},
		{"one scale", compHaveScale, 2 + 2},
		{"a scale on each axis", compHaveXYScale, 2 + 4},
		{"a two-by-two matrix", compHave2x2, 2 + 8},
		{"words and a two-by-two matrix", compArgsAreWords | compHave2x2, 4 + 8},
	} {
		body := component(tc.flags, tc.extra)
		// Something after it that must not be counted.
		s := &stream{b: append(append([]byte(nil), body...), 0xde, 0xad, 0xbe, 0xef)}
		size, haveInstructions, err := sizeOfComposite(s)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if size != len(body) {
			t.Errorf("%s: measured %d bytes, want %d", tc.what, size, len(body))
		}
		if haveInstructions {
			t.Errorf("%s: reported instructions where the flags name none", tc.what)
		}
		if s.at != 0 {
			t.Errorf("%s: measuring consumed %d bytes, and it must consume none",
				tc.what, s.at)
		}
	}

	// Several components, and the flag that says instructions follow the last.
	two := append(component(compMoreComponents, 2), component(compHaveInstrs, 2)...)
	s := &stream{b: append(two, 1, 2, 3)}
	size, haveInstructions, err := sizeOfComposite(s)
	if err != nil {
		t.Fatal(err)
	}
	if size != len(two) {
		t.Errorf("two components measured %d bytes, want %d", size, len(two))
	}
	if !haveInstructions {
		t.Error("the instruction flag on the last component was not noticed")
	}
	// A list that says another component follows and then stops.
	cut := &stream{b: component(compMoreComponents, 2)}
	if _, _, err := sizeOfComposite(cut); err == nil {
		t.Error("a component list that ends in the middle was measured anyway")
	}
}

// TestBytesAfterTheLastBlockAreRefused. A WOFF 2's blocks sit one after another
// and the last runs to the end of the file. Anything past it is something the
// header did not describe, and reading the font anyway is how a file with
// payload hidden in it passes for an ordinary one.
func TestBytesAfterTheLastBlockAreRefused(t *testing.T) {
	built, _ := syntheticWOFF2(t, fonttest.WOFF2Options{Trailing: []byte("hidden away")})
	if _, err := DecodeWOFF(built); err == nil {
		t.Error("a file with bytes after its last block was accepted")
	}
	// The control, so this is not refusing every fixture the builder makes.
	ok, _ := syntheticWOFF2(t, fonttest.WOFF2Options{})
	if _, err := DecodeWOFF(ok); err != nil {
		t.Errorf("the control fixture was refused: %v", err)
	}
}

// TestALongRunOfIdenticalFlagsIsBrokenAtTwoHundredAndFiftySix.
//
// TrueType writes a repeated point flag once with a count after it, and the
// count is a byte — so a glyph whose points run on identically for longer than
// that needs the flag written again. No glyph in any font in any corpus this is
// measured against does, which is why it is here: the case is real, it is
// reachable by any font with a long enough smooth curve, and nothing else would
// find it.
func TestALongRunOfIdenticalFlagsIsBrokenAtTwoHundredAndFiftySix(t *testing.T) {
	// Points a step apart on a diagonal: every one has the same flag, because
	// every delta is +1 on both axes.
	points := func(n int) []point {
		out := make([]point, n)
		for i := range out {
			out[i] = point{x: int32(i + 1), y: int32(i + 1), onCurve: true}
		}
		return out
	}
	const flag = glyfOnCurve | glyfXShort | glyfXSameOrUp | glyfYShort | glyfYSameOrUp

	for _, tc := range []struct {
		n     int
		flags []byte
	}{
		{1, []byte{flag}},
		{2, []byte{flag | glyfRepeat, 1}},
		{256, []byte{flag | glyfRepeat, 255}},
		// One past what a count can say: the flag is written again.
		{257, []byte{flag | glyfRepeat, 255, flag}},
		{258, []byte{flag | glyfRepeat, 255, flag | glyfRepeat, 1}},
		{300, []byte{flag | glyfRepeat, 255, flag | glyfRepeat, 43}},
		// And twice over.
		{513, []byte{flag | glyfRepeat, 255, flag | glyfRepeat, 255, flag}},
	} {
		pts := points(tc.n)
		got := appendPoints(nil, pts, false)
		// The flags come first, then one byte of x per point and one of y.
		if len(got) != len(tc.flags)+2*tc.n {
			t.Errorf("%d points came to %d bytes, want %d flag bytes and %d of "+
				"coordinates", tc.n, len(got), len(tc.flags), 2*tc.n)
			continue
		}
		if string(got[:len(tc.flags)]) != string(tc.flags) {
			t.Errorf("%d points wrote flags % x, want % x", tc.n, got[:len(tc.flags)], tc.flags)
			continue
		}
		// Every delta is one, and a short delta is stored as its magnitude.
		for i := len(tc.flags); i < len(got); i++ {
			if got[i] != 1 {
				t.Errorf("%d points: byte %d of the coordinates is %d, want 1",
					tc.n, i-len(tc.flags), got[i])
				break
			}
		}
	}

	// The overlap bit goes on the first point and nowhere else, which is what
	// makes it a statement about the glyph rather than about a point.
	got := appendPoints(nil, points(3), true)
	if got[0]&glyfOverlap == 0 {
		t.Errorf("the overlap bit is not on the first flag: %08b", got[0])
	}
	if got[0]&glyfRepeat != 0 {
		t.Errorf("the first flag was folded into the run, and it differs from the rest")
	}
	if got[1]&glyfOverlap != 0 {
		t.Errorf("the overlap bit reached a second flag: %08b", got[1])
	}
}

// rebuiltWithFault takes the WOFF 2 in testdata, which has a transformed glyf
// and a transformed hmtx, and writes it back with one thing changed.
//
// It exists because the faults below can only occur in a font whose glyf is
// transformed, and nothing here can build one of those: the transform is a
// re-encoding of every outline, and writing it would be a second copy of the
// hardest part of the decoder. So the transform in the file is carried over
// untouched and only the directory around it is made wrong.
func rebuiltWithFault(t *testing.T, mutate func(tables []fonttest.WOFF2Table)) []byte {
	t.Helper()
	return rebuiltFrom(t, "HasubiMono-hmtx-dropped.woff2", mutate)
}

func rebuiltFrom(t *testing.T, name string, mutate func(tables []fonttest.WOFF2Table)) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "woff2", name))
	if err != nil {
		t.Fatal(err)
	}
	sfnt, err := DecodeWOFF2(data)
	if err != nil {
		t.Fatal(err)
	}
	tabs := SFNTTables(sfnt)

	r := &woff2Reader{b: data, at: woff2HeaderSize}
	n := int(binary.BigEndian.Uint16(data[12:]))
	dir, err := readWOFF2Directory(r, n)
	if err != nil {
		t.Fatal(err)
	}
	compLen := binary.BigEndian.Uint32(data[20:])
	last := dir[len(dir)-1]
	body, err := brotliDecodeForTest(data[r.at:r.at+int(compLen)], int(last.srcOffset+last.srcLength))
	if err != nil {
		t.Fatal(err)
	}

	var list []fonttest.WOFF2Table
	for _, e := range dir {
		tag := string([]byte{byte(e.tag >> 24), byte(e.tag >> 16), byte(e.tag >> 8), byte(e.tag)})
		w := fonttest.WOFF2Table{Tag: tag, Data: tabs[tag]}
		if e.transformed {
			w.Transformed = true
			w.Transform = body[e.srcOffset : e.srcOffset+e.srcLength]
		}
		list = append(list, w)
	}
	// The control: rebuilt with nothing changed, it must still be the same font.
	if mutate == nil {
		return fonttest.WOFF2(fonttest.WOFF2Options{
			Flavor: binary.BigEndian.Uint32(data[4:]), Tables: list,
		})
	}
	mutate(list)
	return fonttest.WOFF2(fonttest.WOFF2Options{
		Flavor: binary.BigEndian.Uint32(data[4:]), Tables: list,
	})
}

func find(tables []fonttest.WOFF2Table, tag string) *fonttest.WOFF2Table {
	for i := range tables {
		if tables[i].Tag == tag {
			return &tables[i]
		}
	}
	return nil
}

// TestADirectoryThatContradictsItselfIsRefused.
//
// None of these is a font that draws wrongly; each is a font whose directory
// says two things that cannot both be true, and each was found by removing the
// check that catches it and seeing that everything else still passed.
func TestADirectoryThatContradictsItselfIsRefused(t *testing.T) {
	// The control first, so that a failure below is about the fault and not
	// about the rebuilding.
	if _, err := DecodeWOFF2(rebuiltWithFault(t, nil)); err != nil {
		t.Fatalf("rebuilt with nothing changed: %v", err)
	}

	for _, tc := range []struct {
		what   string
		mutate func([]fonttest.WOFF2Table)
	}{
		{"a transformed loca that carries bytes", func(ts []fonttest.WOFF2Table) {
			// Every byte of loca comes out of the glyf stream, so a
			// transformed one has nothing of its own to carry.
			find(ts, "loca").Transform = []byte{1, 2, 3, 4}
		}},
		{"a loca too short for the glyphs", func(ts []fonttest.WOFF2Table) {
			loca := find(ts, "loca")
			loca.Data = loca.Data[:len(loca.Data)-4]
		}},
		{"a loca too long for the glyphs", func(ts []fonttest.WOFF2Table) {
			loca := find(ts, "loca")
			loca.Data = append(append([]byte(nil), loca.Data...), 0, 0, 0, 0)
		}},
		{"an hmtx whose stated size is not the one it rebuilds to",
			func(ts []fonttest.WOFF2Table) {
				hmtx := find(ts, "hmtx")
				hmtx.Data = hmtx.Data[:len(hmtx.Data)-2]
			}},
		{"an hhea too short to hold its metric count", func(ts []fonttest.WOFF2Table) {
			hhea := find(ts, "hhea")
			hhea.Data = hhea.Data[:20]
		}},
		// glyf writes loca as it goes, so naming loca first asks for a table
		// before the thing that produces it. The reference decoder accepts
		// this one and writes a font whose loca record is offset zero and
		// length zero — every outline present and no way to reach one.
		{"loca named before the glyf it comes out of", func(ts []fonttest.WOFF2Table) {
			var gi, li int
			for i, w := range ts {
				switch w.Tag {
				case "glyf":
					gi = i
				case "loca":
					li = i
				}
			}
			ts[gi], ts[li] = ts[li], ts[gi]
		}},
	} {
		if _, err := DecodeWOFF2(rebuiltWithFault(t, tc.mutate)); err == nil {
			t.Errorf("%s was accepted", tc.what)
		}
	}
}
