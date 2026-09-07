package layout

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/mgilbir/forme/style"
)

// What a picture is, and what a span attribute says.

// TestAPNGWithSVGInItIsStillAPNG.
//
// The sniff was a search for "<svg" anywhere in the first kilobyte, which is a
// search for four bytes that occur in compressed data as often as any other
// four. A PNG with them in a chunk was read as a picture this engine cannot
// draw and refused, and nothing about the file says why.
func TestAPNGWithSVGInItIsStillAPNG(t *testing.T) {
	// A real PNG, with the bytes planted in a text chunk after its header.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	planted := append(append([]byte(nil), data[:33]...), []byte("<svg xmlns=")...)
	planted = append(planted, data[33:]...)

	if looksLikeSVG(planted) {
		t.Error("a PNG carrying the bytes \"<svg\" was read as an SVG")
	}
	if !looksLikeSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)) {
		t.Error("an SVG was not read as one")
	}
}

// TestWhatIsAndIsNotAnSVG is the shape of the sniff, both ways.
func TestWhatIsAndIsNotAnSVG(t *testing.T) {
	for _, tc := range []struct {
		data string
		want bool
		what string
	}{
		{`<svg width="1" height="1"/>`, true, "the root element and nothing before it"},
		{"  \n\t<svg/>", true, "white space in front of it"},
		{"\ufeff<svg/>", true, "a byte order mark in front of it"},
		{`<SVG/>`, true, "in capitals"},
		{`<?xml version="1.0"?><svg/>`, true, "an XML declaration first"},
		{"<!-- a comment -->\n<svg/>", true, "a comment first"},
		{`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN"><svg/>`, true, "a doctype first"},

		{"", false, "nothing at all"},
		{"\x89PNG\r\n\x1a\n", false, "a PNG signature"},
		{"\xff\xd8\xff\xe0", false, "a JPEG signature"},
		{"GIF89a", false, "a GIF signature"},
		{"<html><body>the word <svg is in this text</body></html>", false,
			"markup whose root element is not svg"},
		{"<?xml version=\"1.0\"?><rss><title>&lt;svg</title></rss>", false,
			"XML whose root element is not svg"},
		{"not markup at all, mentioning <svg in passing", false,
			"text that does not begin with markup"},
	} {
		if got := looksLikeSVG([]byte(tc.data)); got != tc.want {
			t.Errorf("%s: looksLikeSVG = %v, want %v", tc.what, got, tc.want)
		}
	}
}

// TestASpanAttributeIsParsedTheWayHTMLParsesOne.
//
// HTML's rules for parsing a non-negative integer read the digits at the front
// and stop. strconv.Atoi over the whole string refuses anything with a
// character after them, so "colspan=2.5" — a typo a person makes — was one
// column instead of two, and every column after it in the table was out by one.
func TestASpanAttributeIsParsedTheWayHTMLParsesOne(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
		ok   bool
	}{
		{"2", 2, true},
		{"+2", 2, true},
		{"2abc", 2, true},
		{"2.5", 2, true},
		{"0", 0, true},
		{"007", 7, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-2", 0, false},
		{".5", 0, false},
	} {
		got, ok := leadingNonNegative(tc.raw)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%q parses to (%d, %v), want (%d, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
	// And a value long enough to be arithmetic rather than a span stops where
	// no table could use it.
	long := ""
	for i := 0; i < 400; i++ {
		long += "9"
	}
	if got, ok := leadingNonNegative(long); !ok || got != maxSpanValue {
		t.Errorf("four hundred nines parse to (%d, %v), want (%d, true)",
			got, ok, maxSpanValue)
	}
}

// TestASpanWrittenWithATypoStillSpans is the same rule over a document, which
// is where it is felt: a cell that spans one column instead of two puts every
// column after it out by one for the rest of the table.
func TestASpanWrittenWithATypoStillSpans(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{"2", 2},
		{"2.5", 2},
		{"2abc", 2},
		{"+2", 2},
		{"abc", 1},
		{"-2", 1},
	} {
		l := &layouter{rec: NewRecorder(nil), lengths: map[lengthKey]style.Length{},
			grids: map[*Box]*tableGrid{}}
		built := Build(Input{HTML: `<table><tr><td colspan="` + tc.raw +
			`">a</td></tr><tr><td>b</td><td>c</td></tr></table>`})
		table := findBoxWhere(t, built.Root, func(b *Box) bool { return b.Inner == InnerTable })
		g := l.tableGridFor(table)
		if len(g.cells) == 0 {
			t.Fatalf("colspan=%q produced no cells", tc.raw)
		}
		if got := g.cells[0].colSpan; got != tc.want {
			t.Errorf("colspan=%q spans %d columns, want %d", tc.raw, got, tc.want)
		}
	}
}

// TestASpanningCellsPercentageReachesItsColumns.
//
// §17.5.2.2 is written for cells that occupy one column and says nothing about
// a spanning one, so its percentage was read and thrown away: "width: 40%" on a
// cell two columns wide did nothing at all, while the same declaration on the
// cell beside it decided a column.
func TestASpanningCellsPercentageReachesItsColumns(t *testing.T) {
	firstRow := func(markup string) []float64 {
		t.Helper()
		root := layoutOf(t, 600, markup, noDefaults+
			`table { border-collapse: collapse; width: 400px }
			 td { padding: 0; border: 0; font-family: Courier; font-size: 10px }`)
		var out []float64
		for _, row := range find(t, root, "t").Children {
			for _, cell := range row.Children {
				out = append(out, cell.BorderRect.W.Px())
			}
			break // the first row is the one that spans
		}
		return out
	}
	// Three columns, and a first row whose one cell spans the first two. The
	// third column is what the percentage takes room from, so the split between
	// the pair and it is the whole of what this measures.
	const rest = `<tr><td>a</td><td>b</td><td>c</td></tr></table>`
	plain := firstRow(`<table id=t><tr><td colspan=2>ab</td><td>c</td></tr>` + rest)
	wide := firstRow(`<table id=t><tr><td colspan=2 style="width: 80%">ab</td>` +
		`<td>c</td></tr>` + rest)
	if len(plain) < 2 || len(wide) < 2 {
		t.Fatalf("the first row laid out %d and %d cells, want two", len(plain), len(wide))
	}
	if wide[0] <= plain[0] {
		t.Errorf("the spanning cell is %gpx wide with the percentage and %gpx "+
			"without it; the declaration did nothing", wide[0], plain[0])
	}
	// Eighty per cent of four hundred, within a unit of rounding.
	if got := wide[0]; got < 315 || got > 325 {
		t.Errorf("the spanning cell is %gpx of a 400px table, want about 320", got)
	}
}
