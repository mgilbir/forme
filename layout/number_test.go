package layout

import (
	"math"
	"strings"
	"testing"
)

// TestGoNumberSyntaxIsNotCSSNumberSyntax is the whole of this: four spellings
// strconv.ParseFloat accepts that no stylesheet and no SVG attribute can mean.
//
// The tokenizer reads every one of them as an identifier, so a declaration
// holding one is invalid and dropped — but these readers never see a token.
// They started from a string the cascade kept, handed it to ParseFloat, and got
// a number back for text that was never a number. They read CSS's own <number>
// now, through the reader line-height already used; see paragraph.ParseNumber
// for why it is CSS's reader and not Go's.
//
// It is tested here rather than beside that reader because here is where the
// six callers are, and what this is guarding is that they go through it.
func TestGoNumberSyntaxIsNotCSSNumberSyntax(t *testing.T) {
	for _, s := range []string{
		"nan", "NaN",
		"inf", "Inf", "infinity", "+inf", "-inf",
		"0x1p3", // Go's hexadecimal float
		"1_0",   // Go's digit separator
		"1.",    // a number and then a delimiter, not one number
		"1e",    // a number and then an identifier
		"1e+",   //
		".",     //
		"", " ", "+", "-",
		"1 2", "1,2", "12px", "12%",
	} {
		if v, ok := parseNumber(s); ok {
			t.Errorf("parseNumber(%q) = %v, and CSS has no way to write it", s, v)
		}
	}
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"0", 0}, {"16", 16}, {"+16", 16}, {"-16", -16},
		{"0.5", 0.5}, {".5", 0.5}, {"-.5", -0.5},
		{"1e3", 1000}, {"1E3", 1000}, {"1e+3", 1000}, {"1e-3", 0.001},
		{"1.5e2", 150},
	} {
		if v, ok := parseNumber(c.in); !ok || v != c.want {
			t.Errorf("parseNumber(%q) = %v, %v; want %v, true", c.in, v, ok, c.want)
		}
	}
	// An exponent whose answer does not exist is refused rather than clamped:
	// 10^400 is an infinity, and an infinite multiplier reaches arithmetic that
	// clamps it to the largest length there is, so the page comes out set on a
	// number nobody wrote. Refused where the number is past the end and not
	// where its spelling looks it: "9e308" has a small exponent and is.
	for _, s := range []string{"1e400", "1e401", "-1e400", "9e308"} {
		if v, ok := parseNumber(s); ok {
			t.Errorf("parseNumber(%q) = %v; the answer is not a number this can "+
				"hold, and every length operation would clamp it out of sight", s, v)
		}
	}
	// And the largest one that does exist is still read, and read exactly: the
	// conversion is rounded once, from the decimal, to the nearest float64.
	// It used to accumulate the digits and multiply by a power of ten, which
	// came within an ulp of 1e308 — and, at the far ends, to NaN. See
	// TestAZeroUnderAnyExponentIsZero.
	if v, ok := parseNumber("1e308"); !ok || v != 1e308 {
		t.Errorf("parseNumber(%q) = %v, %v; want 1e308", "1e308", v, ok)
	}
}

// TestANaNPassesEveryGuardWrittenAgainstANumber is why the spellings above are
// worth a reader of their own rather than an extra condition at each site.
//
// Each of these readers already refuses a nonsense number: "wn <= 0" for a
// ratio, "n < 0" for a percentage, "v <= 0" for an SVG dimension. None of them
// stops a NaN, because a NaN is neither greater than nor less than anything and
// every one of those comparisons is false for it. So the one value that most
// needs refusing is the one value that walks through.
func TestANaNPassesEveryGuardWrittenAgainstANumber(t *testing.T) {
	nan := math.NaN()
	if nan <= 0 || nan < 0 || nan > 0 {
		t.Fatal("this Go does not have IEEE comparisons and the premise is wrong")
	}
	if _, ok := aspectRatioOf("nan"); ok {
		t.Error(`aspect-ratio: nan was read as a ratio`)
	}
	if _, ok := aspectRatioOf("1 / nan"); ok {
		t.Error(`aspect-ratio: 1 / nan was read as a ratio`)
	}
	if v, ok := percentValue("nan%"); ok {
		t.Errorf("percentValue(%q) = %v; a text-fit of that size is not a size", "nan%", v)
	}
	if v, ok := svgPercent("nan%"); ok {
		t.Errorf("svgPercent(%q) = %v; it would scale an area by a NaN", "nan%", v)
	}
	if v, ok := svgCoord("nan"); ok {
		t.Errorf("svgCoord(%q) = %v; it is a coordinate nothing can be drawn at", "nan", v)
	}
	if v, ok := svgViewBoxAll("0 0 nan nan"); ok {
		t.Errorf("svgViewBoxAll(%q) = %v; the zero and negative test below it is "+
			"false for a NaN, so the viewBox scale would be one", "0 0 nan nan", v)
	}
}

// TestARatioNobodyCanWriteLeavesTheBoxAlone is the same fact at the far end,
// where it is visible: a document, a box, and the height it comes out.
//
// "aspect-ratio: nan" made the box thirty-three million pixels tall. That
// number is style.Unit saturating — the lengths are defended and a NaN never
// reached the geometry — but nothing refused the declaration, so the box was
// sized by a value the author had no way of writing. It should be ignored, and
// the box left the height its content gives it.
func TestARatioNobodyCanWriteLeavesTheBoxAlone(t *testing.T) {
	line := ratioHeightOfFilledBox(t, ``)
	if line <= 0 {
		t.Fatalf("a box with a line in it is %gpx tall; the fixture is wrong", line)
	}
	for _, decl := range []string{
		`aspect-ratio: nan`,
		`aspect-ratio: 1 / nan`,
		`aspect-ratio: nan / 1`,
		`aspect-ratio: auto nan`,
		`aspect-ratio: inf`,
		`aspect-ratio: infinity`,
		`aspect-ratio: 0x1p3`,
		`aspect-ratio: 1_0`,
		`aspect-ratio: 1.`,
	} {
		if got := ratioHeightOfFilledBox(t, decl); got != line {
			t.Errorf("%s made the box %gpx tall and a line is %gpx; CSS has no way "+
				"to write that value, so the declaration is invalid and the box "+
				"keeps the height its content gives it", decl, got, line)
		}
	}
	// And the control: a ratio that *is* one still works, or the test above
	// passes on a reader that refuses everything.
	if got := ratioHeightOfFilledBox(t, `aspect-ratio: 16/9`); got != 180 {
		t.Errorf("a ratio that is one made the box %gpx tall, want 180", got)
	}
}

// TestAZeroUnderAnyExponentIsZero is audit C44 on the page: "opacity: 0e400"
// is nought, which CSS says it is, and it came out as NaN — the reader
// multiplied nought by 10^400 — which is neither at most nought nor at least
// one, so it passed both of opacityOf's guards and reached the fill.
//
// The rest are the other spellings the same arithmetic got wrong: a mantissa
// too long for a float64 brought back to one by its exponent, and a number
// below the smallest float64, which is nought and not a refusal.
func TestAZeroUnderAnyExponentIsZero(t *testing.T) {
	for _, decl := range []string{"0e400", "-0e400", "0e99999999999999999999", "1e-400"} {
		ops := paintOf(t, `<div id="b"></div>`, noDefaults+box100+`#b { opacity: `+decl+` }`)
		if got := alphasOf(ops, green); len(got) != 0 {
			t.Errorf("opacity: %s painted %d green fills at alphas %v; it is nought",
				decl, len(got), got)
		}
	}
	for _, decl := range []string{strings.Repeat("9", 400) + "e-400", "1e0"} {
		ops := paintOf(t, `<div id="b"></div>`, noDefaults+box100+`#b { opacity: `+decl+` }`)
		name := decl
		if len(name) > 12 {
			name = name[:6] + "…" + name[len(name)-5:]
		}
		if got := soleAlpha(t, ops, green, "opacity: "+name); got != 1 {
			t.Errorf("opacity: %s painted at alpha %v; it is one", name, got)
		}
	}
	for _, s := range []string{"0e400", "-0e400", strings.Repeat("9", 310) + "e-310"} {
		if v, ok := parseNumber(s); !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("parseNumber(%.20q) = %v, %v", s, v, ok)
		}
	}
}
