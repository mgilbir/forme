package layout

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// Emphasis marks, CSS Text Decoration 3 §3.
//
// Ahem makes the arithmetic whole: ascent 0.8em, descent 0.2em, no line gap,
// and every glyph an em square — "•" among them — so at 20px a character is 20
// wide and reaches 16 above its baseline and 4 below, and its 10px mark reaches
// 8 above the mark's baseline and 2 below. With "line-height: 1" there is no
// leading, and a line of marked text is 20 + 10 tall.

const emphasisCSS = noDefaults + `p { font: 20px/1 Ahem; text-emphasis: dot }`

// emphasisPage lays a document out in Ahem and paints it.
func emphasisPage(t *testing.T, htmlSrc, cssSrc string) (*Fragment, []Op, []Finding) {
	t.Helper()
	frag, findings := layoutWith(t, loadAhem(t), htmlSrc, cssSrc)
	return frag, Paint(frag), findings
}

// marksIn is every emphasis mark a display list draws, in order.
func marksIn(ops []Op) []DrawText {
	var out []DrawText
	flat, _ := flattenGroups(ops, "")
	for _, op := range flat {
		if m, ok := op.(DrawEmphasisMark); ok {
			out = append(out, m.Mark)
		}
	}
	return out
}

func markXs(marks []DrawText) []float64 {
	out := make([]float64, len(marks))
	for i, m := range marks {
		out[i] = m.At.X.Px()
	}
	return out
}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAMarkIsCentredOverEachCharacter: one mark per character that takes one,
// half the size, centred on the character's advance, and sitting on the text's
// ascent — and the line grown to hold it.
func TestAMarkIsCentredOverEachCharacter(t *testing.T) {
	frag, ops, _ := emphasisPage(t, `<p id="p">ab c</p>`, emphasisCSS)
	marks := marksIn(ops)
	// a is 0 to 20, b 20 to 40, the space 40 to 60 and takes no mark, c 60 to 80.
	if got, want := markXs(marks), []float64{5, 25, 65}; !sameFloats(got, want) {
		t.Fatalf("the marks' pens are at x=%v, want %v", got, want)
	}
	for _, m := range marks {
		if m.Text != "\u2022" || m.Size.Px() != 10 || m.Upright || m.Sideways {
			t.Errorf("a mark is %q at %gpx (upright %v, sideways %v), want U+2022 at 10px lying along the line",
				m.Text, m.Size.Px(), m.Upright, m.Sideways)
		}
		// The line's baseline is at 26: the text reaches 16 above it and its
		// marks 10 more. The mark's baseline is the text's ascent less the
		// mark's descent, 26 - 16 - 2.
		if m.At.Y.Px() != 8 {
			t.Errorf("a mark's baseline is at y=%g, want 8", m.At.Y.Px())
		}
	}
	line := find(t, frag, "p").Lines[0]
	if line.Rect.H.Px() != 30 || line.Baseline.Px() != 26 {
		t.Errorf("the line is %gpx tall with its baseline at %g, want 30 and 26: a line-height "+
			"of 1 leaves no room for the marks, so the line grows by their 10px",
			line.Rect.H.Px(), line.Baseline.Px())
	}
}

// TestAMarkIsCentredOnTheCharacterNotItsSpacing is letter-spacing-211's
// sentence: "Emphasis marks are centered on characters, not characters +
// spacing".
func TestAMarkIsCentredOnTheCharacterNotItsSpacing(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>abc</p>`, emphasisCSS+`p { letter-spacing: 20px }`)
	// a is 0 to 20 and the spacing after it 20 to 40; b 40 to 60; c 80 to 100.
	if got, want := markXs(marksIn(ops)), []float64{5, 45, 85}; !sameFloats(got, want) {
		t.Errorf("the marks' pens are at x=%v, want %v", got, want)
	}
}

// TestAMarkUnderTheText: text-emphasis-position: under puts the mark's line
// below the text's descent, and grows the line below.
func TestAMarkUnderTheText(t *testing.T) {
	frag, ops, _ := emphasisPage(t, `<p id="p">ab</p>`, emphasisCSS+`p { text-emphasis-position: under right }`)
	line := find(t, frag, "p").Lines[0]
	if line.Rect.H.Px() != 30 || line.Baseline.Px() != 16 {
		t.Errorf("the line is %gpx tall with its baseline at %g, want 30 and 16", line.Rect.H.Px(), line.Baseline.Px())
	}
	for _, m := range marksIn(ops) {
		// 16 + the text's descent 4 + the mark's ascent 8.
		if m.At.Y.Px() != 28 {
			t.Errorf("a mark's baseline is at y=%g, want 28", m.At.Y.Px())
		}
	}
	if n := len(marksIn(ops)); n != 2 {
		t.Errorf("%d marks, want 2", n)
	}
}

// TestMarksGrowTheLineOnlyWhereTheLeadingIsShort is CSS Ruby 1 §3.6, which
// §3.4 defers to: leading is added only where the line-height leaves the marks
// too little room in all, and then so that identical lines would not overlap.
func TestMarksGrowTheLineOnlyWhereTheLeadingIsShort(t *testing.T) {
	for _, c := range []struct {
		lineHeight     string
		height, baseln float64
	}{
		// 24px: 2 of leading either side and the marks need 10 over, so 6 is
		// missing. The side under the text has more than the marks ask of it
		// there, which is none, so the missing leading goes over: 8 over and 2
		// under, and the line is 30 with its baseline at 16 + 8.
		{"1.2", 30, 24},
		// 30px: 5 either side, which is the marks' 10 in all. Nothing is
		// added: a mark reaches 5 into the line above's leading and no further.
		{"1.5", 30, 21},
		// 60px: room to spare, and nothing changes.
		{"3", 60, 36},
	} {
		frag, _, _ := emphasisPage(t, `<p id="p">ab</p>`, emphasisCSS+`p { line-height: `+c.lineHeight+` }`)
		line := find(t, frag, "p").Lines[0]
		if line.Rect.H.Px() != c.height || line.Baseline.Px() != c.baseln {
			t.Errorf("line-height %s: the line is %gpx with its baseline at %g, want %g and %g",
				c.lineHeight, line.Rect.H.Px(), line.Baseline.Px(), c.height, c.baseln)
		}
	}
	// Under the text, the same arithmetic the other way up: the 6 missing go
	// under, and the baseline is where the half-leading put it.
	frag, _, _ := emphasisPage(t, `<p id="p">ab</p>`, emphasisCSS+`p { line-height: 1.2; text-emphasis-position: under }`)
	if line := find(t, frag, "p").Lines[0]; line.Rect.H.Px() != 30 || line.Baseline.Px() != 18 {
		t.Errorf("marks under: the line is %gpx with its baseline at %g, want 30 and 18",
			line.Rect.H.Px(), line.Baseline.Px())
	}
	// A line-height below the font's own leaves negative leading, and the
	// marks' side takes the whole band.
	frag, _, _ = emphasisPage(t, `<p id="p">ab</p>`, emphasisCSS+`p { line-height: 0.5 }`)
	if line := find(t, frag, "p").Lines[0]; line.Rect.H.Px() != 30 || line.Baseline.Px() != 26 {
		t.Errorf("line-height 0.5: the line is %gpx with its baseline at %g, want 30 and 26",
			line.Rect.H.Px(), line.Baseline.Px())
	}
	// And the marks are the box's: a marked span grows the line of a block
	// that has none, and the same line without the span's marks is not grown.
	frag, _, _ = emphasisPage(t, `<div id="d">a<span>b</span></div>`,
		noDefaults+`div { font: 20px/1 Ahem } span { text-emphasis: dot }`)
	if h := find(t, frag, "d").Lines[0].Rect.H.Px(); h != 30 {
		t.Errorf("a line with one marked span is %gpx, want 30", h)
	}
	frag, _, _ = emphasisPage(t, `<div id="d">a<span>b</span></div>`,
		noDefaults+`div { font: 20px/1 Ahem }`)
	if h := find(t, frag, "d").Lines[0].Rect.H.Px(); h != 20 {
		t.Errorf("a line with no marks is %gpx, want 20", h)
	}
}

// TestTheMarksColour: text-emphasis-color, and the text's own colour where it
// is currentcolor — the colour of the box the text is in.
func TestTheMarksColour(t *testing.T) {
	blue := style.RGBA{B: 255, A: 1}
	_, ops, _ := emphasisPage(t, `<p>a<span>b</span></p>`,
		emphasisCSS+`p { color: rgb(0, 128, 0) } span { color: rgb(0, 0, 255) }`)
	marks := marksIn(ops)
	if len(marks) != 2 || marks[0].Color != green || marks[1].Color != blue {
		t.Errorf("the marks are %v, want one in the paragraph's green and one in the span's blue", marks)
	}
	_, ops, _ = emphasisPage(t, `<p>a</p>`, emphasisCSS+`p { text-emphasis-color: rgb(255, 0, 0) }`)
	if marks := marksIn(ops); len(marks) != 1 || marks[0].Color != red {
		t.Errorf("the marks are %v, want one in red", marks)
	}
	// The shorthand resets the colour it does not name.
	_, ops, _ = emphasisPage(t, `<p>a</p>`, noDefaults+
		`p { font: 20px/1 Ahem; color: rgb(0, 128, 0); text-emphasis-color: rgb(255, 0, 0); text-emphasis: dot }`)
	if marks := marksIn(ops); len(marks) != 1 || marks[0].Color != green {
		t.Errorf("the marks are %v, want one in the text's green", marks)
	}
}

// TestMarksAreTheBoxsOwn: the properties inherit, so a span saying none has
// none, where it could not take a paragraph's underline off.
func TestMarksAreTheBoxsOwn(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a<span>b</span>c</p>`, emphasisCSS+`span { text-emphasis: none }`)
	if got, want := markXs(marksIn(ops)), []float64{5, 45}; !sameFloats(got, want) {
		t.Errorf("the marks are at x=%v, want %v: over a and c and not the span's b", got, want)
	}
	_, ops, _ = emphasisPage(t, `<p>a<span>b</span>c</p>`, emphasisCSS)
	if n := len(marksIn(ops)); n != 3 {
		t.Errorf("%d marks, want 3: the span inherits the paragraph's", n)
	}
	// The paragraph's own strut is its root inline box, whose text's marks
	// are its own: the line holds them though the only text on it has none.
	frag, ops, _ := emphasisPage(t, `<p id="p"><span>b</span></p>`, emphasisCSS+`span { text-emphasis: none }`)
	if line := find(t, frag, "p").Lines[0]; len(marksIn(ops)) != 0 || line.Rect.H.Px() != 30 || line.Baseline.Px() != 26 {
		t.Errorf("a marked paragraph's line of unmarked text is %gpx with its baseline at %g "+
			"and %d marks, want 30, 26 and none", line.Rect.H.Px(), line.Baseline.Px(), len(marksIn(ops)))
	}
}

// TestWhichCharactersAreMarked is §3.1's list, through a document: punctuation
// is not marked, and the symbols it names are.
func TestWhichCharactersAreMarked(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a,b#c.%</p>`, emphasisCSS)
	// a 0, ',' 20, b 40, # 60, c 80, '.' 100, % 120.
	if got, want := markXs(marksIn(ops)), []float64{5, 45, 65, 85, 125}; !sameFloats(got, want) {
		t.Errorf("the marks are at x=%v, want %v", got, want)
	}
}

// TestTakesEmphasisMark is §3.1's exclusions one class at a time.
func TestTakesEmphasisMark(t *testing.T) {
	for unit, want := range map[string]bool{
		"a": true, "\u6F22": true, "1": true, "+": true, "\u00E9": true, "e\u0301": true,
		"\uE000": true, // private use: not among the exclusions
		" ":      false, "\u00A0": false, "\u3000": false, "\u2028": false, "\u2029": false,
		" \u0301": true, // "a space that combines with any combining characters"
		",":       false, ".": false, "\u3001": false, "\u300C": false, "(": false, "-": false,
		"\u1361": false, // ETHIOPIC WORDSPACE, a word separator
		"#":      true, "%": true, "&": true, "@": true, "\u00A7": true, "\u00B6": true,
		"\u2030": true, "\u303D": true, "\uFF03": true, "\uFE6B": true,
		"\t": false, "\u200D": false, "\u00AD": false, "\u0378": false,
		"": false,
	} {
		if got := takesEmphasisMark(unit); got != want {
			t.Errorf("takesEmphasisMark(%+q) = %v, want %v", unit, got, want)
		}
	}
}

// TestMarkedPunctuationIsTheNFKDList derives §3.1's punctuation from
// UnicodeData.txt: every P* character whose full compatibility decomposition
// is exactly one of the fifteen symbols the section lists.
func TestMarkedPunctuationIsTheNFKDList(t *testing.T) {
	path := filepath.Join("..", "testdata", "ucd", "UnicodeData.txt")
	f, err := os.Open(path)
	if err != nil {
		if os.Getenv("TABLE_INPUTS") == "required" {
			t.Fatalf("%s is not here, and TABLE_INPUTS=required says the database was fetched", path)
		}
		t.Skipf("%s is not in this checkout; `make ucd` fetches it", path)
	}
	defer f.Close()
	type row struct{ cat, decomp string }
	rows := map[rune]row{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ";")
		if len(fields) < 6 {
			continue
		}
		cp, err := strconv.ParseUint(fields[0], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		rows[rune(cp)] = row{fields[2], fields[5]}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	var nfkd func(r rune, depth int) []rune
	nfkd = func(r rune, depth int) []rune {
		d := rows[r].decomp
		if d == "" || depth > 20 {
			return []rune{r}
		}
		parts := strings.Fields(d)
		if strings.HasPrefix(parts[0], "<") {
			parts = parts[1:]
		}
		var out []rune
		for _, p := range parts {
			cp, _ := strconv.ParseUint(p, 16, 32)
			out = append(out, nfkd(rune(cp), depth+1)...)
		}
		return out
	}
	listed := map[rune]bool{}
	for _, r := range "#%\u2030\u2031\u066A\u0609\u060A&\u204A@\u00A7\u00B6\u204B\u2053\u303D" {
		listed[r] = true
	}
	derived := map[rune]bool{}
	for r, row := range rows {
		if !strings.HasPrefix(row.cat, "P") {
			continue
		}
		if d := nfkd(r, 0); len(d) == 1 && listed[d[0]] {
			derived[r] = true
		}
	}
	if len(derived) < len(listed) {
		t.Fatalf("only %d characters derived from %s; the file was not read", len(derived), path)
	}
	for r := range derived {
		if !markedPunctuation[r] {
			t.Errorf("U+%04X decomposes to a listed symbol and is not in markedPunctuation", r)
		}
	}
	for r := range markedPunctuation {
		if !derived[r] {
			t.Errorf("U+%04X is in markedPunctuation and does not decompose to a listed symbol", r)
		}
	}
}

// TestTheMarkShapes is §3.1's table, and what a lone fill keyword means in
// each typographic mode.
func TestTheMarkShapes(t *testing.T) {
	l := &layouter{rec: NewRecorder(nil), reportedOnce: map[string]bool{}}
	b := &Box{Style: style.Initial()}
	for _, c := range []struct {
		value    string
		vertical bool
		want     string
	}{
		{"dot", false, "\u2022"}, {"filled dot", false, "\u2022"}, {"open dot", false, "\u25E6"},
		{"circle", false, "\u25CF"}, {"open circle", false, "\u25CB"},
		{"double-circle", false, "\u25C9"}, {"open double-circle", false, "\u25CE"},
		{"triangle", false, "\u25B2"}, {"triangle open", false, "\u25B3"},
		{"sesame", false, "\uFE45"}, {"open sesame", true, "\uFE46"},
		{"filled", false, "\u25CF"}, {"open", false, "\u25CB"},
		{"filled", true, "\uFE45"}, {"open", true, "\uFE46"},
		{"'x'", false, "x"}, {"'xy'", false, "x"}, {"'e\u0301z'", true, "e\u0301"},
		{"''", false, ""},
	} {
		if got := l.emphasisMark(b, c.value, c.vertical); got != c.want {
			t.Errorf("%q (vertical %v) is %+q, want %+q", c.value, c.vertical, got, c.want)
		}
	}
}

// TestAMarkStringIsBounded: the first grapheme cluster is drawn, and one longer
// than the bound is not drawn at all and is reported — however long the string
// behind it.
func TestAMarkStringIsBounded(t *testing.T) {
	at := strings.Repeat("\u0301", (maxEmphasisMarkBytes-1)/2) // a + marks, within the bound
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"abc", "a", true},
		{"a" + at, "a" + at, true},
		{"a" + at + "b", "a" + at, true},
		{"a" + at + "\u0301", "", false},
		{"a" + strings.Repeat("\u0301", 100000), "", false},
		{"\U0001F469\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466x", "\U0001F469\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466", true},
	} {
		got, ok := firstCluster(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("firstCluster(%d bytes) = %d bytes, %v; want %d bytes, %v",
				len(c.in), len(got), ok, len(c.want), c.ok)
		}
	}
	// Through a document: reported, and no marks, and the line not grown for
	// marks it does not have.
	frag, ops, findings := emphasisPage(t, `<p id="p">ab</p>`,
		emphasisCSS+`p { text-emphasis: 'a`+strings.Repeat("\u0301", 1000)+`' }`)
	if n := len(marksIn(ops)); n != 0 {
		t.Errorf("%d marks drawn from a mark past the bound", n)
	}
	if len(findingsWith(findings, RuleLimit)) != 1 {
		t.Errorf("the bound was not reported once: %v", findings)
	}
	if h := find(t, frag, "p").Lines[0].Rect.H.Px(); h != 20 {
		t.Errorf("the line is %gpx, want 20", h)
	}
}

// TestAMarkNoFaceHasIsReported: a mark no face can draw is not drawn as a
// missing-glyph box beside every character, and the author is told.
func TestAMarkNoFaceHasIsReported(t *testing.T) {
	// Neither Ahem nor the standard faces have a sesame.
	frag, ops, findings := emphasisPage(t, `<p id="p">ab</p>`, emphasisCSS+`p { text-emphasis: sesame }`)
	if n := len(marksIn(ops)); n != 0 {
		t.Errorf("%d marks drawn with a glyph no face has", n)
	}
	missing := findingsWith(findings, RuleGlyphMissing)
	if len(missing) != 1 || !strings.Contains(missing[0].Message, "emphasis mark") {
		t.Errorf("the missing mark was not reported: %v", findings)
	}
	if h := find(t, frag, "p").Lines[0].Rect.H.Px(); h != 20 {
		t.Errorf("the line is %gpx, want 20: no marks are drawn, so none are made room for", h)
	}
}

// TestMarksArePaintedBetweenTheTextAndTheLineThrough is §5.1's order: under-
// and overlines, the text, the marks, the line-through.
func TestMarksArePaintedBetweenTheTextAndTheLineThrough(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a</p>`, emphasisCSS+`p { text-decoration: underline line-through;
		text-decoration-color: rgb(0, 128, 0) }`)
	var order []string
	for _, op := range ops {
		switch v := op.(type) {
		case DrawText:
			order = append(order, "text")
		case DrawEmphasisMark:
			order = append(order, "mark")
		case FillRect:
			if v.Color == green {
				order = append(order, "line")
			}
		}
	}
	if got, want := strings.Join(order, " "), "line text mark line"; got != want {
		t.Errorf("painted %s, want %s", got, want)
	}
}

// TestAMarkCastsAShadow: a text shadow shadows the marks too, as glyphs, above
// the text's own shadow and below the line-through's.
func TestAMarkCastsAShadow(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a</p>`, emphasisCSS+`p { text-shadow: rgb(255, 0, 0) 1px 2px }`)
	var shadows []DrawTextShadow
	for _, op := range ops {
		if s, ok := op.(DrawTextShadow); ok {
			shadows = append(shadows, s)
		}
	}
	marks := marksIn(ops)
	if len(shadows) != 2 || len(marks) != 1 {
		t.Fatalf("%d shadows and %d marks, want 2 and 1", len(shadows), len(marks))
	}
	s := shadows[1].Run
	if s.Text != marks[0].Text || s.Color != red || s.At != (Point{X: marks[0].At.X.Add(rpx(1)), Y: marks[0].At.Y.Add(rpx(2))}) {
		t.Errorf("the mark's shadow is %+v, want the mark moved by 1,2 in red", s)
	}
	// A transparent mark is not drawn and still casts its shadow, as a
	// transparent decoration line does: the shadow is of its shape.
	_, ops, _ = emphasisPage(t, `<p>a</p>`, emphasisCSS+`p { text-shadow: rgb(255, 0, 0) 1px 2px;
		text-emphasis-color: transparent }`)
	shadows = shadows[:0]
	for _, op := range ops {
		if s, ok := op.(DrawTextShadow); ok {
			shadows = append(shadows, s)
		}
	}
	if len(marksIn(ops)) != 0 || len(shadows) != 2 {
		t.Errorf("a transparent mark drew %d marks and %d shadows, want none and 2",
			len(marksIn(ops)), len(shadows))
	}
}

// TestFirstLineMarks: ::first-line applies the emphasis properties, which CSS
// Pseudo 4 §2.1.3 lists among "all text decoration properties".
func TestFirstLineMarks(t *testing.T) {
	frag, ops, _ := emphasisPage(t, `<p id="p">ab<br>cd</p>`, noDefaults+
		`p { font: 20px/1 Ahem } p::first-line { text-emphasis: dot }`)
	marks := marksIn(ops)
	if len(marks) != 2 || marks[0].At.Y.Px() != 8 || marks[1].At.Y.Px() != 8 {
		t.Errorf("the marks are %v, want two over the first line's a and b", marks)
	}
	lines := find(t, frag, "p").Lines
	if len(lines) != 2 || lines[0].Rect.H.Px() != 30 || lines[1].Rect.H.Px() != 20 {
		t.Errorf("the lines are not 30 and 20 tall: %v", lines)
	}
}

// TestAMarkOnAVerticalLineStandsUpright: in vertical-rl a mark goes on the
// right of the column and stands upright, centred on its character's advance
// along it — for text turned with the page and for text set upright.
func TestAMarkOnAVerticalLineStandsUpright(t *testing.T) {
	const box = noDefaults + `#d { font: 20px/1 Ahem; writing-mode: vertical-rl;
		width: 90px; height: 100px; text-emphasis: dot }`
	for _, c := range []struct {
		name, css string
		// right is how far right of the run's pen the middle of the mark's
		// column is: past the text's reach over its baseline and half the
		// marks' band.
		right float64
	}{
		// Turned: the text reaches its ascent, 16, and the band is 10.
		{"turned", box, 21},
		// Upright: the text is hung from the middle of the line, half an em
		// either side of it.
		{"upright", box + `#d { text-orientation: upright }`, 15},
		// Left is the line's under side: past the text's descent, 4, and
		// half the band.
		{"on the left", box + `#d { text-emphasis-position: over left }`, -9},
	} {
		_, ops, _ := emphasisPage(t, `<div id="d">ab</div>`, c.css)
		var text DrawText
		for _, op := range ops {
			if v, ok := op.(DrawText); ok && v.Text == "ab" {
				text = v
			}
		}
		marks := marksIn(ops)
		if len(marks) != 2 || text.Face == nil {
			t.Fatalf("%s: %d marks and the run %+v", c.name, len(marks), text)
		}
		for i, m := range marks {
			if !m.Upright || !m.Sideways || m.Anticlockwise {
				t.Errorf("%s: a mark is not an upright run on a vertical line: %+v", c.name, m)
			}
			// Ahem states no vertical metrics, so an upright mark is one em
			// of its 10px along the line and across it, hung from its middle:
			// its pen is 5 before the character's middle, 10 and 30.
			want := Point{X: text.At.X.Add(rpx(c.right)), Y: text.At.Y.Add(rpx(float64(5 + 20*i)))}
			if m.At != want {
				t.Errorf("%s: mark %d is at %v, want %v", c.name, i, m.At, want)
			}
		}
	}
}

// TestAMarkOnASidewaysLineLiesAlongIt: the sideways modes are horizontal
// typographic modes, so the marks are over the text and turned with it — to
// its right in sideways-rl, which turns clockwise, and to its left in
// sideways-lr, which turns the other way.
func TestAMarkOnASidewaysLineLiesAlongIt(t *testing.T) {
	for _, c := range []struct {
		mode          string
		anticlockwise bool
		// dx and dy are the mark's pen from the run's, for the first mark.
		dx, dy float64
	}{
		// The mark's baseline is 18 over the run's (16 + 2), which is to the
		// right; its pen 5 along the line, which is down.
		{"sideways-rl", false, 18, 5},
		// Anticlockwise: over is to the left, and along is up.
		{"sideways-lr", true, -18, -5},
	} {
		_, ops, _ := emphasisPage(t, `<div id="d">ab</div>`, noDefaults+`#d { font: 20px/1 Ahem;
			writing-mode: `+c.mode+`; width: 90px; height: 100px; text-emphasis: dot }`)
		var text DrawText
		for _, op := range ops {
			if v, ok := op.(DrawText); ok && v.Text == "ab" {
				text = v
			}
		}
		marks := marksIn(ops)
		if len(marks) != 2 || text.Face == nil {
			t.Fatalf("%s: %d marks and the run %+v", c.mode, len(marks), text)
		}
		m := marks[0]
		if m.Upright || !m.Sideways || m.Anticlockwise != c.anticlockwise {
			t.Errorf("%s: the mark is not turned with its line: %+v", c.mode, m)
		}
		if want := (Point{X: text.At.X.Add(rpx(c.dx)), Y: text.At.Y.Add(rpx(c.dy))}); m.At != want {
			t.Errorf("%s: the mark is at %v, want %v", c.mode, m.At, want)
		}
	}
}

// TestUnitSpansTileTheRun: every unit of a run has a span, the spans follow one
// another in the order the units are drawn, and together they are the run's
// advance — a ligature shared out, a right-to-left run from its right.
func TestUnitSpansTileTheRun(t *testing.T) {
	noto := loadNoto(t, "NotoSans-Regular.ttf")
	hebrew := loadHebrew(t)
	for _, rtl := range []bool{false, true} {
		if g, _ := ShapedGlyphs(DrawText{Text: "ffi", Face: noto, Size: rpx(20), RTL: rtl}); len(g) != 1 {
			t.Fatalf("the face set ffi as %d glyphs; the fixture needs a ligature", len(g))
		}
	}
	for _, v := range []DrawText{
		{Text: "office affix", Face: noto, Size: rpx(20)},
		{Text: "e\u0301te\u0301", Face: noto, Size: rpx(20)},
		{Text: "\u05E9\u05DC\u05D5\u05DD", Face: hebrew, Size: rpx(20), RTL: true},
		// The same ligature in a run drawn right to left, as a bidi override
		// makes one: the first f is the right-hand third of it.
		{Text: "office", Face: noto, Size: rpx(20), RTL: true},
	} {
		spans := unitSpans(v)
		var units []unitSpan
		for _, s := range spans {
			if s.text == "\u202E" {
				// The override ShapedText puts in front of a right-to-left run
				// draws nothing.
				continue
			}
			if !s.have {
				t.Errorf("%q: the unit %+q has no span", v.Text, s.text)
			}
			units = append(units, s)
		}
		if len(units) != segmentCount(v.Text) {
			t.Fatalf("%q: %d spans for %d units", v.Text, len(units), segmentCount(v.Text))
		}
		// The advance the glyphs a backend draws come to, which is what the
		// spans are read from.
		width := shapedAdvance(v)
		for i, s := range units {
			if s.hi < s.lo {
				t.Errorf("%q: %+q spans %v to %v", v.Text, s.text, s.lo, s.hi)
			}
			if i == 0 {
				continue
			}
			prev := units[i-1]
			// Next to the one before, within a unit's rounding: after it, or
			// before it where the run reads right to left.
			gap := s.lo.Sub(prev.hi)
			if v.RTL {
				gap = prev.lo.Sub(s.hi)
			}
			if gap < -1 || gap > 1 {
				t.Errorf("%q: %+q is %v from %+q", v.Text, s.text, gap, prev.text)
			}
		}
		lo, hi := units[0].lo, units[len(units)-1].hi
		if v.RTL {
			lo, hi = units[len(units)-1].lo, units[0].hi
		}
		if lo > 1 || abs(hi.Sub(width)) > style.Unit(len(units)) {
			t.Errorf("%q: the spans cover %v to %v, want 0 to %v", v.Text, lo, hi, width)
		}
	}
}

func segmentCount(s string) int {
	n := 0
	for range s {
		n++
	}
	// Every fixture above is one unit a character but the combining marks.
	return n - strings.Count(s, "\u0301")
}

// TestAMarkIsHalfTheSizeItsTextIsSetAt: the marks are memoized per box, and by
// the size its text is set at, which text-fit changes line by line.
func TestAMarkIsHalfTheSizeItsTextIsSetAt(t *testing.T) {
	built := Build(Input{HTML: `<p>a</p>`, CSS: []Stylesheet{{Source: emphasisCSS}}})
	l := newLayouter(built.Root, Size{W: rpx(600), H: rpx(600)}, loadAhem(t), NewRecorder(nil))
	var p *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b.Element != nil && b.Element.Name == "p" {
			p = b
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if p == nil {
		t.Fatal("no paragraph")
	}
	small, large := l.emphasisOf(p, rpx(20)), l.emphasisOf(p, rpx(40))
	if small == nil || large == nil || small.size != rpx(10) || large.size != rpx(20) ||
		large.ascent != rpx(32) || large.band() != rpx(20) {
		t.Errorf("the marks at 20px and 40px are %+v and %+v", small, large)
	}
}

// TestAMarkOnFittedTextIsHalfItsFittedSize: css-text-5's text-fit scales the
// size a line's text is set at, and the marks with it — they are half the
// element's font as its text is set.
func TestAMarkOnFittedTextIsHalfItsFittedSize(t *testing.T) {
	frag, ops, _ := emphasisPage(t, `<div id="d">ab</div>`, noDefaults+`#d { font: 20px/normal Ahem;
		width: 80px; text-fit: grow per-line-all; text-emphasis: dot }`)
	marks := marksIn(ops)
	// "ab" is 40px at 20px, so it is set at 40px across the 80: a is 0 to 40
	// and its 20px mark's pen is at 10.
	if len(marks) != 2 || marks[0].Size.Px() != 20 || marks[0].At.X.Px() != 10 || marks[1].At.X.Px() != 50 {
		t.Fatalf("the marks are %+v, want two at 20px over the two 40px letters", marks)
	}
	// The line holds 40px of text and 20px of marks, and the marks sit on it.
	line := find(t, frag, "d").Lines[0]
	if line.Rect.H.Px() != 60 || line.Baseline.Px() != 52 || marks[0].At.Y.Px() != 16 {
		t.Errorf("the line is %gpx with its baseline at %g and the marks' at %g, want 60, 52 and 16",
			line.Rect.H.Px(), line.Baseline.Px(), marks[0].At.Y.Px())
	}
}

// TestAMarkIsGlyphsToTheComparison: the reftest comparison reads a mark as the
// ink it makes — the same as a run of its character there in its colour — and
// sees it move.
func TestAMarkIsGlyphsToTheComparison(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a</p>`, emphasisCSS)
	marks := marksIn(ops)
	if len(marks) != 1 {
		t.Fatalf("%d marks", len(marks))
	}
	m := marks[0]
	if !pictureEqual([]Op{DrawEmphasisMark{Mark: m}}, []Op{m}, picPage) {
		t.Error("a mark and a run of its character in the same place compare different")
	}
	moved := m
	moved.At.X = moved.At.X.Add(rpx(1))
	if pictureEqual([]Op{DrawEmphasisMark{Mark: m}}, []Op{DrawEmphasisMark{Mark: moved}}, picPage) {
		t.Error("a mark moved by a pixel compares the same")
	}
	recoloured := m
	recoloured.Color = red
	if pictureEqual([]Op{DrawEmphasisMark{Mark: m}}, []Op{DrawEmphasisMark{Mark: recoloured}}, picPage) {
		t.Error("a mark recoloured compares the same")
	}
	if normaliseOps([]Op{DrawEmphasisMark{Mark: m}}) == "" {
		t.Error("a page holding a mark reads as blank")
	}
}

// TestAMarkIsClippedDimmedAndBoundedAsText: the operations that narrow and
// fade what was painted treat a mark as the glyphs it is.
func TestAMarkIsClippedDimmedAndBoundedAsText(t *testing.T) {
	_, ops, _ := emphasisPage(t, `<p>a</p>`, emphasisCSS)
	m := DrawEmphasisMark{Mark: marksIn(ops)[0]}
	ink := textInk(m.Mark)
	// Wholly outside: dropped. Cut: carries the clip. Wholly inside: untouched.
	if got := clipOps([]Op{m}, 0, Clip{Active: true, Rect: Rect{X: rpx(500), Y: rpx(500), W: rpx(10), H: rpx(10)}}); len(got) != 0 {
		t.Errorf("a mark outside its clip was kept: %v", got)
	}
	cut := Rect{X: ink.X, Y: ink.Y, W: ink.W.Div(2), H: ink.H}
	got := clipOps([]Op{m}, 0, Clip{Active: true, Rect: cut})
	if len(got) != 1 || !got[0].(DrawEmphasisMark).Mark.Clip.Active {
		t.Errorf("a mark its clip cuts does not carry it: %v", got)
	}
	whole := Rect{X: rpx(-100), Y: rpx(-100), W: rpx(1000), H: rpx(1000)}
	if got := clipOps([]Op{m}, 0, Clip{Active: true, Rect: whole}); len(got) != 1 ||
		got[0].(DrawEmphasisMark).Mark.Clip.Active {
		t.Errorf("a mark inside its clip was changed: %v", got)
	}
	dimmed, groupMarks := dimOps([]Op{m}, 0, 0.5)
	if len(dimmed) != 1 || dimmed[0].(DrawEmphasisMark).Mark.Color.A != 0.5 || len(groupMarks) != 1 {
		t.Errorf("a mark at half opacity is %v", dimmed)
	}
	if r, ok := opBounds(m); !ok || r != textInkReserved(m.Mark) {
		t.Errorf("a mark's bounds are %v, %v", r, ok)
	}
}

// TestMarksAreLinearInTheText: a mark per character, each found by a walk of
// the run's glyphs, so marking a run four times as long costs four times as
// much. Timed, since nothing counts the work; the smaller run is two thousand
// characters, which is kilobytes.
func TestMarksAreLinearInTheText(t *testing.T) {
	frag, _, _ := emphasisPage(t, `<p>a</p>`, emphasisCSS)
	var e *runEmphasis
	var face DrawText
	for _, f := range allFragments(frag) {
		for _, line := range f.Lines {
			for _, r := range line.Runs {
				if r.emphasis != nil {
					e, face = r.emphasis, DrawText{Face: r.Face, Size: r.Size}
				}
			}
		}
	}
	if e == nil {
		t.Fatal("no marked run")
	}
	mark := func(n int) func() {
		text := face
		text.Text = strings.Repeat("ab, ", n)
		text.CharSpacing = rpx(1)
		if got := len(e.marks(unitSpans(text), text.At, false, red, runTurn{})); got != 2*n {
			t.Fatalf("%d marks over %d words, want %d", got, n, 2*n)
		}
		return func() { e.marks(unitSpans(text), text.At, false, red, runTurn{}) }
	}
	c := costtest.Time(t, "the marks of a run", mark(500), mark(2000))
	if c.Ratio > 8 {
		t.Errorf("marking 2000 words cost %s the 500", c)
	}
}

// allFragments is a fragment and everything under it.
func allFragments(f *Fragment) []*Fragment {
	out := []*Fragment{f}
	for _, c := range f.Children {
		out = append(out, allFragments(c)...)
	}
	return out
}

// TestAMarkInAFaceOfRectanglesIsRectangles: the comparison turns a run of
// rectangle glyphs into the rectangles it inks, and a mark in such a face is
// the same ink as the run of its character.
func TestAMarkInAFaceOfRectanglesIsRectangles(t *testing.T) {
	face := ahemFace(t)
	run := DrawText{Text: "•", Face: face, Size: rpx(10), Color: red, At: Point{X: rpx(5), Y: rpx(8)}}
	got := blockFills([]Op{DrawEmphasisMark{Mark: run}})
	if len(got) != 1 {
		t.Fatalf("the mark became %v", got)
	}
	if _, ok := got[0].(FillRect); !ok {
		t.Fatalf("the mark became %v, want a fill", got)
	}
	assertOps(t, got, blockFills([]Op{run}))
}
