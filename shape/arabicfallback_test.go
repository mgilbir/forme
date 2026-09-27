package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// arabicFallbackGlyphs is a face that maps the Arabic letters it has and
// their presentation forms, and states no joining forms of its own: the face
// HarfBuzz's Arabic fallback is for. Glyph i+1 is entry i, and each has an
// advance of its own, so that a glyph can be told by its advance too.
func arabicFallbackGlyphs() []fonttest.Glyph {
	var g []fonttest.Glyph
	for i, r := range []rune{
		0x0628, 0xFE8F, 0xFE90, 0xFE91, 0xFE92, // beh, and its isolated, final, initial and medial forms
		0x0644, 0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0, // lam
		0x0627, 0xFE8D, 0xFE8E, // alef, isolated and final
		0xFEFB, 0xFEFC, // lam-alef, isolated and final
		0x0647, 0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC, // heh
		0xF201, // the private-use lellah ligature
	} {
		g = append(g, fonttest.Glyph{Rune: r, Advance: 300 + 10*i, HasShape: true})
	}
	for _, r := range []rune{0x064E, 0x0651, 0xFC60} { // fatha, shadda, shadda with fatha
		g = append(g, fonttest.Glyph{Rune: r, Advance: 0, HasShape: true})
	}
	return g
}

// withoutRune is a fixture's glyphs with one character's glyph drawn as
// nothing the character map reaches: it stays in place, so the others keep
// their numbers.
func withoutRune(g []fonttest.Glyph, r rune) []fonttest.Glyph {
	for i := range g {
		if g[i].Rune == r {
			g[i].Rune = 0xE000
		}
	}
	return g
}

// The fixture's glyphs, by what they are.
const (
	afBeh = 1 + iota
	afBehIsol
	afBehFina
	afBehInit
	afBehMedi
	afLam
	afLamIsol
	afLamFina
	afLamInit
	afLamMedi
	afAlef
	afAlefIsol
	afAlefFina
	afLamAlefIsol
	afLamAlefFina
	afHeh
	afHehIsol
	afHehFina
	afHehInit
	afHehMedi
	afLellah
	afFatha
	afShadda
	afShaddaFatha
)

// windows1256High is the upper half of the Windows-1256 code page, 0x80 to
// 0xFF: the character each byte is, from the WHATWG Encoding Standard's index
// (Python's cp1256 codec agrees with it byte for byte).
var windows1256High = [128]rune{
	0x20AC, 0x067E, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, // 0x80
	0x02C6, 0x2030, 0x0679, 0x2039, 0x0152, 0x0686, 0x0698, 0x0688, // 0x88
	0x06AF, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, // 0x90
	0x06A9, 0x2122, 0x0691, 0x203A, 0x0153, 0x200C, 0x200D, 0x06BA, // 0x98
	0x00A0, 0x060C, 0x00A2, 0x00A3, 0x00A4, 0x00A5, 0x00A6, 0x00A7, // 0xA0
	0x00A8, 0x00A9, 0x06BE, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x00AF, // 0xA8
	0x00B0, 0x00B1, 0x00B2, 0x00B3, 0x00B4, 0x00B5, 0x00B6, 0x00B7, // 0xB0
	0x00B8, 0x00B9, 0x061B, 0x00BB, 0x00BC, 0x00BD, 0x00BE, 0x061F, // 0xB8
	0x06C1, 0x0621, 0x0622, 0x0623, 0x0624, 0x0625, 0x0626, 0x0627, // 0xC0
	0x0628, 0x0629, 0x062A, 0x062B, 0x062C, 0x062D, 0x062E, 0x062F, // 0xC8
	0x0630, 0x0631, 0x0632, 0x0633, 0x0634, 0x0635, 0x0636, 0x00D7, // 0xD0
	0x0637, 0x0638, 0x0639, 0x063A, 0x0640, 0x0641, 0x0642, 0x0643, // 0xD8
	0x00E0, 0x0644, 0x00E2, 0x0645, 0x0646, 0x0647, 0x0648, 0x00E7, // 0xE0
	0x00E8, 0x00E9, 0x00EA, 0x00EB, 0x0649, 0x064A, 0x00EE, 0x00EF, // 0xE8
	0x064B, 0x064C, 0x064D, 0x064E, 0x00F4, 0x064F, 0x0650, 0x00F7, // 0xF0
	0x0651, 0x00F9, 0x0652, 0x00FB, 0x00FC, 0x200E, 0x200F, 0x06D2, // 0xF8
}

// win1256Glyphs is a face laid out as an Arabic font made for Windows was:
// glyph c is the character byte c is in Windows-1256, so that alef is glyph
// 199 and lam 225, and the joining forms HarfBuzz's table substitutes are at
// glyphs the code page gives to other characters or to none. The printable
// ASCII bytes are themselves; the control bytes, which a font does not map,
// map the private use area, as fonttest needs every glyph to map something.
// Each glyph advances
// by its own amount, 300 and its number, so that the glyph can be told by its
// advance too; the Arabic marks advance by nothing.
func win1256Glyphs() []fonttest.Glyph {
	g := make([]fonttest.Glyph, 255)
	for c := 1; c <= 255; c++ {
		var r rune
		switch {
		case c >= 0x80:
			r = windows1256High[c-0x80]
		case c >= 0x20 && c < 0x7F:
			r = rune(c)
		default:
			r = 0xE000 + rune(c)
		}
		adv := 300 + c
		if r >= 0x064B && r <= 0x0652 {
			adv = 0
		}
		g[c-1] = fonttest.Glyph{Rune: r, Advance: adv, HasShape: true}
	}
	return g
}

// win1256Fixtures are the Windows-1256 face, and faces that are almost it: one
// whose sukun is not at 250, which HarfBuzz does not take for Windows-1256;
// one that also maps one presentation form, and one that maps the ligature of
// shadda and fatha, from either of which the character map's fallback builds
// a lookup and so is used instead of the table — the second draws no joining
// form at all, as HarfBuzz draws none; and one that declares 'init', for which
// there is no fallback at all.
func win1256Fixtures() map[string][]byte {
	sukunElsewhere := win1256Glyphs()
	sukunElsewhere[250-1].Rune, sukunElsewhere[251-1].Rune = 0x00FB, 0x0652
	oneForm := win1256Glyphs()
	oneForm[4-1].Rune = 0xFE91 // beh's initial form, at the glyph the table gives it
	markLigature := win1256Glyphs()
	markLigature[5-1].Rune = 0xFC60 // shadda with fatha
	return map[string][]byte{
		"win1256": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWin1256", Glyphs: win1256Glyphs()}),
		"win1256-sukun-elsewhere": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWin1256SukunElsewhere",
			Glyphs: sukunElsewhere}),
		"win1256-one-form": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWin1256OneForm", Glyphs: oneForm}),
		"win1256-mark-ligature": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWin1256MarkLigature",
			Glyphs: markLigature}),
		"win1256-declares-init": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWin1256DeclaresInit",
			Glyphs: win1256Glyphs(),
			Extra:  map[string][]byte{"GSUB": fonttest.GSUBLookups(nil, map[string][]int{"init": {}})}}),
	}
}

// arabicFallbackFixtures are the face with nothing but a character map, and
// the same face with a GSUB that declares 'init' and names no lookup for it —
// which is enough for HarfBuzz to take the font at its word and make none of
// the forms.
func arabicFallbackFixtures() map[string][]byte {
	return map[string][]byte{
		"cmap-only": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicCmapOnly", Glyphs: arabicFallbackGlyphs()}),
		// The fallback's place among the font's own rules: its 'rlig' joins a
		// beh and a heh as letters, before the fallback makes forms of them,
		// and its 'calt' replaces the initial beh the fallback made.
		"with-rules": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicWithRules", Glyphs: arabicFallbackGlyphs(),
			Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
				{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
					{Components: []int{afBeh, afHeh}, Glyph: afLellah}})}},
				{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{afBehInit}, []int{afLamIsol})}},
			}, map[string][]int{"rlig": {0}, "calt": {1}})}}),
		// No final alef, so no ligature has one as a part: the lam-alef the
		// face maps is not made, even where something the face does not map
		// — a .notdef — stands where the alef would be.
		"no-final-alef": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicNoFinalAlef",
			Glyphs: withoutRune(arabicFallbackGlyphs(), 0xFE8E)}),
		"declares-init": fonttest.SFNT(fonttest.SFNTOptions{Name: "ArabicDeclaresInit", Glyphs: arabicFallbackGlyphs(),
			Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups(nil, map[string][]int{"init": {}})}}),
	}
}

// TestArabicFallsBackToPresentationForms: a face with no joining forms of its
// own draws them out of its character map, as HarfBuzz does — the four forms,
// the lam-alef and three-part ligatures under 'rlig' stepping over marks, and
// the shadda ligature — and a face that declares a form draws none. Every
// answer is HarfBuzz 14.5.0's for the same face, from the pinned uharfbuzz
// (TestWriteArabicFallbackFixtures writes the faces out to ask it).
func TestArabicFallsBackToPresentationForms(t *testing.T) {
	fonts := arabicFallbackFixtures()
	adv := func(gid int) float64 { return float64(300 + 10*(gid-1)) }
	g := func(gids ...int) []shapedAs {
		var out []shapedAs
		for _, gid := range gids {
			a := adv(gid)
			if gid >= afFatha {
				a = 0
			}
			out = append(out, shapedAs{gid, a, 0, 0})
		}
		return out
	}
	for _, c := range []struct {
		font, text string
		want       []shapedAs
	}{
		// Drawn left to right, so the last letter comes first.
		{"cmap-only", "\u0628\u0628\u0628", g(afBehFina, afBehMedi, afBehInit)},
		{"cmap-only", "\u0628", g(afBehIsol)},
		{"cmap-only", "\u0644\u0627", g(afLamAlefIsol)},
		{"cmap-only", "\u0628\u0644\u0627", g(afLamAlefFina, afBehInit)},
		{"cmap-only", "\u0644\u0644\u0647", g(afLellah)},
		// A heh that joins on is medial, so the ligature, which wants it
		// final, is not made.
		{"cmap-only", "\u0644\u0644\u0647\u0628", g(afBehFina, afHehMedi, afLamMedi, afLamInit)},
		// The forms and the lam-alef step over a mark, which the face places
		// by its ink, having no positioning of its own.
		{"cmap-only", "\u0628\u064E\u0628", []shapedAs{{afBehFina, adv(afBehFina), 0, 0},
			{afFatha, 0, -235, 862}, {afBehInit, adv(afBehInit), 0, 0}}},
		{"cmap-only", "\u0644\u064E\u0627", []shapedAs{{afFatha, 0, -77, 862},
			{afLamAlefIsol, adv(afLamAlefIsol), 0, 0}}},
		{"cmap-only", "\u0628\u0651\u064E", []shapedAs{{afShaddaFatha, 0, -245, 862},
			{afBehIsol, adv(afBehIsol), 0, 0}}},
		{"with-rules", "\u0628\u0647", g(afLellah)},
		{"with-rules", "\u0628\u0628", g(afBehFina, afLamIsol)},
		{"no-final-alef", "\u0628\u0644\u062C", []shapedAs{{0, 0, 0, 0},
			{afLamMedi, adv(afLamMedi), 0, 0}, {afBehInit, adv(afBehInit), 0, 0}}},
		{"declares-init", "\u0628\u0628\u0628", g(afBeh, afBeh, afBeh)},
		{"declares-init", "\u0644\u0627", g(afAlef, afLam)},
	} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatalf("%s: %v", c.font, err)
		}
		got, _ := f.ShapeGlyphs(c.text)
		checkShaped(t, fmt.Sprintf("%s %+q", c.font, c.text), got, c.want)
	}
}

// TestAWindows1256FaceIsShapedByHarfBuzzsTable: a face laid out in the order
// of Windows-1256, with no presentation form in its character map, draws its
// joining forms, lam-alefs and shadda ligatures from HarfBuzz's table for that
// encoding; faces that are almost it do not. Every answer in win1256Shaped is
// HarfBuzz 14.5.0's, from the pinned uharfbuzz 0.56.2: the faces written out
// by TestWriteArabicFallbackFixtures, each string shaped with its segment
// properties guessed, as hb-shape does.
//
// The strings are every Arabic letter the code page has, isolated, initial,
// medial and final beside a beh; each lam-alef isolated and medial, with a
// mark between its parts or not; each shadda ligature, and a mark the forms
// step over. Out of tree, forty thousand random strings of those letters and
// marks, tatweel, the joiners and space were shaped through both engines on
// every face here. Every string that differs on the Windows-1256 face and not
// on the one whose sukun is elsewhere — the table's difference, and no other —
// is a mark placed on a lam-alef the table made, and neither cause is the
// table's. A shadda ligature made of marks inside the lam-alef loses the
// ligature record its first mark had, which HarfBuzz's ligate_input keeps, so
// it is placed on the ligature's last part rather than on the lam; and a mark
// after a zero width joiner the ligature stepped over is placed around the
// joiner's glyph by HarfBuzz and around the ligature here.
func TestAWindows1256FaceIsShapedByHarfBuzzsTable(t *testing.T) {
	fonts := win1256Fixtures()
	faces := map[string]*Face{}
	font := ""
	cases := 0
	for _, line := range strings.Split(strings.TrimSpace(win1256Shaped), "\n") {
		text, answer, ok := strings.Cut(line, "\t")
		if !ok {
			font = line
			continue
		}
		f := faces[font]
		if f == nil {
			var err error
			if f, err = Load(fonts[font]); err != nil {
				t.Fatalf("%s: %v", font, err)
			}
			faces[font] = f
		}
		s, err := strconv.Unquote(`"` + text + `"`)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		var want []shapedAs
		for _, g := range strings.Split(answer, "|") {
			var sa shapedAs
			if _, err := fmt.Sscanf(g, "%d,%g,%g,%g", &sa.gid, &sa.adv, &sa.dx, &sa.dy); err != nil {
				t.Fatalf("%s: %q: %v", text, g, err)
			}
			want = append(want, sa)
		}
		got, _ := f.ShapeGlyphs(s)
		checkShaped(t, font+" "+text, got, want)
		cases++
	}
	if cases != 263 {
		t.Fatalf("read %d cases; the table holds 263", cases)
	}
}

// TestTheWindows1256FaceFollowsTheNeighbours: the table draws the forms of a
// Windows-1256 face as the character map's lookups draw those of another, so
// the questions a line breaker asks say the forms depend on the letters either
// side, and the face has joining forms. The faces that are almost it answer
// as their own fallback does: the one that maps a presentation form has forms
// from its character map, and the other three have none — the one that maps a
// ligature of marks included, whose fallback is the character map's and
// builds that ligature alone.
func TestTheWindows1256FaceFollowsTheNeighbours(t *testing.T) {
	fonts := win1256Fixtures()
	for _, c := range []struct {
		font string
		want bool
	}{{"win1256", true}, {"win1256-one-form", true}, {"win1256-sukun-elsewhere", false},
		{"win1256-mark-ligature", false}, {"win1256-declares-init", false}} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatal(err)
		}
		if got := f.FormsFollowNeighbours("\u0628", Features{}); got != c.want {
			t.Errorf("%s: FormsFollowNeighbours = %v, want %v", c.font, got, c.want)
		}
		if got := f.HasJoiningForms(); got != c.want {
			t.Errorf("%s: HasJoiningForms = %v, want %v", c.font, got, c.want)
		}
	}
}

// TestTheWindows1256TableIsHarfBuzzs holds the table's lookups to the bytes of
// hb-ot-shaper-arabic-win1256.hh, subtable by subtable. The shaping test
// above sees what they do; this sees that a subtable's coverage is the list
// HarfBuzz writes, in its order, and that the two it shares are shared.
func TestTheWindows1256TableIsHarfBuzzs(t *testing.T) {
	words := func(b []byte) []int {
		var w []int
		for i := 0; i+1 < len(b); i += 2 {
			w = append(w, int(b[i])<<8|int(b[i+1]))
		}
		return w
	}
	// SingleSubstFormat2 with its format 1 coverage after it.
	single := func(from, to []int) []int {
		w := []int{2, 6 + 2*len(to), len(to)}
		w = append(w, to...)
		w = append(w, 1, len(from))
		return append(w, from...)
	}
	initMedi := single(
		[]int{198, 200, 201, 202, 203, 204, 205, 206, 211, 212, 213, 214, 223, 225, 227, 228, 236, 237},
		[]int{162, 4, 5, 5, 6, 7, 9, 11, 13, 14, 15, 26, 140, 141, 142, 143, 154, 154})
	lamAlef := single([]int{165, 178, 180, 252}, []int{170, 179, 185, 255})
	want := [5]struct {
		kind, flags int
		subs        [][]int
	}{
		{4, flagIgnoreMarks, nil},
		{1, flagIgnoreMarks, [][]int{initMedi,
			single([]int{218, 219, 221, 222, 229}, []int{27, 30, 128, 131, 144})}},
		{1, flagIgnoreMarks, [][]int{initMedi,
			single([]int{218, 219, 221, 222, 229}, []int{28, 31, 129, 138, 149}), lamAlef}},
		{1, flagIgnoreMarks, [][]int{single(
			[]int{194, 195, 197, 198, 199, 201, 204, 205, 206, 218, 219, 229, 236, 237},
			[]int{2, 1, 3, 181, 0, 159, 8, 10, 12, 29, 127, 152, 160, 156}), lamAlef}},
		{4, 0, nil},
	}
	for i, lk := range arabicWin1256Lookups() {
		if lk.kind != want[i].kind || lk.flags != want[i].flags || lk.markSet != -1 {
			t.Errorf("lookup %d is of kind %d with flags %#x and set %d", i, lk.kind, lk.flags, lk.markSet)
		}
		if want[i].subs == nil {
			continue
		}
		if len(lk.subs) != len(want[i].subs) {
			t.Errorf("lookup %d has %d subtables, want %d", i, len(lk.subs), len(want[i].subs))
			continue
		}
		for k, sub := range lk.subs {
			if got := words(sub); fmt.Sprint(got) != fmt.Sprint(want[i].subs[k]) {
				t.Errorf("lookup %d subtable %d is %v, want %v", i, k, got, want[i].subs[k])
			}
		}
	}
	// The ligatures: one set each, under the lam and the shadda.
	for i, c := range []struct {
		first int
		ligs  [][]int // the ligature glyph, then the second part
	}{
		{225, [][]int{{165, 199}, {178, 195}, {180, 194}, {252, 197}}},
		{248, [][]int{{172, 243}, {173, 245}, {175, 246}}},
	} {
		lk := arabicWin1256Lookups()[4*i]
		if len(lk.subs) != 1 {
			t.Fatalf("ligature lookup %d has %d subtables", 4*i, len(lk.subs))
		}
		sub := lk.subs[0]
		w := words(sub)
		if w[0] != 1 || w[2] != 1 {
			t.Fatalf("ligature lookup %d: format %d with %d sets", 4*i, w[0], w[2])
		}
		if cov := words(sub[w[1]:]); fmt.Sprint(cov) != fmt.Sprint([]int{1, 1, c.first}) {
			t.Errorf("ligature lookup %d covers %v, want %d alone in a list", 4*i, cov, c.first)
		}
		set := sub[w[3]:]
		sw := words(set)
		if sw[0] != len(c.ligs) {
			t.Fatalf("ligature lookup %d: %d ligatures, want %d", 4*i, sw[0], len(c.ligs))
		}
		for k, l := range c.ligs {
			lw := words(set[sw[1+k]:])
			if lw[0] != l[0] || lw[1] != 2 || lw[2] != l[1] {
				t.Errorf("ligature lookup %d, ligature %d: %v, want %v from %d and %d",
					4*i, k, lw[:3], l[0], c.first, l[1])
			}
		}
	}
}

// win1256Shaped is HarfBuzz's answer for each string, by face: a line naming
// the face, then a line for each string — the string, escaped, a tab, and each
// glyph HarfBuzz gives as its number, advance, and x and y offset.
const win1256Shaped = `
win1256
\u067e	129,429,0,0
\u067e\u0628	200,500,0,0|129,429,0,0
\u0628\u067e\u0628	200,500,0,0|129,429,0,0|4,304,0,0
\u0628\u067e	129,429,0,0|4,304,0,0
\u0679	138,438,0,0
\u0679\u0628	200,500,0,0|138,438,0,0
\u0628\u0679\u0628	200,500,0,0|138,438,0,0|4,304,0,0
\u0628\u0679	138,438,0,0|4,304,0,0
\u0686	141,441,0,0
\u0686\u0628	200,500,0,0|141,441,0,0
\u0628\u0686\u0628	200,500,0,0|141,441,0,0|4,304,0,0
\u0628\u0686	141,441,0,0|4,304,0,0
\u0698	142,442,0,0
\u0698\u0628	200,500,0,0|142,442,0,0
\u0628\u0698\u0628	200,500,0,0|142,442,0,0|4,304,0,0
\u0628\u0698	142,442,0,0|4,304,0,0
\u0688	143,443,0,0
\u0688\u0628	200,500,0,0|143,443,0,0
\u0628\u0688\u0628	200,500,0,0|143,443,0,0|4,304,0,0
\u0628\u0688	143,443,0,0|4,304,0,0
\u06af	144,444,0,0
\u06af\u0628	200,500,0,0|144,444,0,0
\u0628\u06af\u0628	200,500,0,0|144,444,0,0|4,304,0,0
\u0628\u06af	144,444,0,0|4,304,0,0
\u06a9	152,452,0,0
\u06a9\u0628	200,500,0,0|152,452,0,0
\u0628\u06a9\u0628	200,500,0,0|152,452,0,0|4,304,0,0
\u0628\u06a9	152,452,0,0|4,304,0,0
\u0691	154,454,0,0
\u0691\u0628	200,500,0,0|154,454,0,0
\u0628\u0691\u0628	200,500,0,0|154,454,0,0|4,304,0,0
\u0628\u0691	154,454,0,0|4,304,0,0
\u06ba	159,459,0,0
\u06ba\u0628	200,500,0,0|159,459,0,0
\u0628\u06ba\u0628	200,500,0,0|159,459,0,0|4,304,0,0
\u0628\u06ba	159,459,0,0|4,304,0,0
\u06be	170,470,0,0
\u06be\u0628	200,500,0,0|170,470,0,0
\u0628\u06be\u0628	200,500,0,0|170,470,0,0|4,304,0,0
\u0628\u06be	170,470,0,0|4,304,0,0
\u06c1	192,492,0,0
\u06c1\u0628	200,500,0,0|192,492,0,0
\u0628\u06c1\u0628	200,500,0,0|192,492,0,0|4,304,0,0
\u0628\u06c1	192,492,0,0|4,304,0,0
\u0621	193,493,0,0
\u0621\u0628	200,500,0,0|193,493,0,0
\u0628\u0621\u0628	200,500,0,0|193,493,0,0|200,500,0,0
\u0628\u0621	193,493,0,0|200,500,0,0
\u0622	194,494,0,0
\u0622\u0628	200,500,0,0|194,494,0,0
\u0628\u0622\u0628	200,500,0,0|2,302,0,0|4,304,0,0
\u0628\u0622	2,302,0,0|4,304,0,0
\u0623	195,495,0,0
\u0623\u0628	200,500,0,0|195,495,0,0
\u0628\u0623\u0628	200,500,0,0|1,301,0,0|4,304,0,0
\u0628\u0623	1,301,0,0|4,304,0,0
\u0624	196,496,0,0
\u0624\u0628	200,500,0,0|196,496,0,0
\u0628\u0624\u0628	200,500,0,0|196,496,0,0|4,304,0,0
\u0628\u0624	196,496,0,0|4,304,0,0
\u0625	197,497,0,0
\u0625\u0628	200,500,0,0|197,497,0,0
\u0628\u0625\u0628	200,500,0,0|3,303,0,0|4,304,0,0
\u0628\u0625	3,303,0,0|4,304,0,0
\u0626	198,498,0,0
\u0626\u0628	200,500,0,0|162,462,0,0
\u0628\u0626\u0628	200,500,0,0|162,462,0,0|4,304,0,0
\u0628\u0626	181,481,0,0|4,304,0,0
\u0627	199,499,0,0
\u0627\u0628	200,500,0,0|199,499,0,0
\u0628\u0627\u0628	200,500,0,0|0,0,0,0|4,304,0,0
\u0628\u0627	0,0,0,0|4,304,0,0
\u0628	200,500,0,0
\u0628\u0628	200,500,0,0|4,304,0,0
\u0628\u0628\u0628	200,500,0,0|4,304,0,0|4,304,0,0
\u0628\u0628	200,500,0,0|4,304,0,0
\u0629	201,501,0,0
\u0629\u0628	200,500,0,0|201,501,0,0
\u0628\u0629\u0628	200,500,0,0|159,459,0,0|4,304,0,0
\u0628\u0629	159,459,0,0|4,304,0,0
\u062a	202,502,0,0
\u062a\u0628	200,500,0,0|5,305,0,0
\u0628\u062a\u0628	200,500,0,0|5,305,0,0|4,304,0,0
\u0628\u062a	202,502,0,0|4,304,0,0
\u062b	203,503,0,0
\u062b\u0628	200,500,0,0|6,306,0,0
\u0628\u062b\u0628	200,500,0,0|6,306,0,0|4,304,0,0
\u0628\u062b	203,503,0,0|4,304,0,0
\u062c	204,504,0,0
\u062c\u0628	200,500,0,0|7,307,0,0
\u0628\u062c\u0628	200,500,0,0|7,307,0,0|4,304,0,0
\u0628\u062c	8,308,0,0|4,304,0,0
\u062d	205,505,0,0
\u062d\u0628	200,500,0,0|9,309,0,0
\u0628\u062d\u0628	200,500,0,0|9,309,0,0|4,304,0,0
\u0628\u062d	10,310,0,0|4,304,0,0
\u062e	206,506,0,0
\u062e\u0628	200,500,0,0|11,311,0,0
\u0628\u062e\u0628	200,500,0,0|11,311,0,0|4,304,0,0
\u0628\u062e	12,312,0,0|4,304,0,0
\u062f	207,507,0,0
\u062f\u0628	200,500,0,0|207,507,0,0
\u0628\u062f\u0628	200,500,0,0|207,507,0,0|4,304,0,0
\u0628\u062f	207,507,0,0|4,304,0,0
\u0630	208,508,0,0
\u0630\u0628	200,500,0,0|208,508,0,0
\u0628\u0630\u0628	200,500,0,0|208,508,0,0|4,304,0,0
\u0628\u0630	208,508,0,0|4,304,0,0
\u0631	209,509,0,0
\u0631\u0628	200,500,0,0|209,509,0,0
\u0628\u0631\u0628	200,500,0,0|209,509,0,0|4,304,0,0
\u0628\u0631	209,509,0,0|4,304,0,0
\u0632	210,510,0,0
\u0632\u0628	200,500,0,0|210,510,0,0
\u0628\u0632\u0628	200,500,0,0|210,510,0,0|4,304,0,0
\u0628\u0632	210,510,0,0|4,304,0,0
\u0633	211,511,0,0
\u0633\u0628	200,500,0,0|13,313,0,0
\u0628\u0633\u0628	200,500,0,0|13,313,0,0|4,304,0,0
\u0628\u0633	211,511,0,0|4,304,0,0
\u0634	212,512,0,0
\u0634\u0628	200,500,0,0|14,314,0,0
\u0628\u0634\u0628	200,500,0,0|14,314,0,0|4,304,0,0
\u0628\u0634	212,512,0,0|4,304,0,0
\u0635	213,513,0,0
\u0635\u0628	200,500,0,0|15,315,0,0
\u0628\u0635\u0628	200,500,0,0|15,315,0,0|4,304,0,0
\u0628\u0635	213,513,0,0|4,304,0,0
\u0636	214,514,0,0
\u0636\u0628	200,500,0,0|26,326,0,0
\u0628\u0636\u0628	200,500,0,0|26,326,0,0|4,304,0,0
\u0628\u0636	214,514,0,0|4,304,0,0
\u0637	216,516,0,0
\u0637\u0628	200,500,0,0|216,516,0,0
\u0628\u0637\u0628	200,500,0,0|216,516,0,0|4,304,0,0
\u0628\u0637	216,516,0,0|4,304,0,0
\u0638	217,517,0,0
\u0638\u0628	200,500,0,0|217,517,0,0
\u0628\u0638\u0628	200,500,0,0|217,517,0,0|4,304,0,0
\u0628\u0638	217,517,0,0|4,304,0,0
\u0639	218,518,0,0
\u0639\u0628	200,500,0,0|27,327,0,0
\u0628\u0639\u0628	200,500,0,0|28,328,0,0|4,304,0,0
\u0628\u0639	29,329,0,0|4,304,0,0
\u063a	219,519,0,0
\u063a\u0628	200,500,0,0|30,330,0,0
\u0628\u063a\u0628	200,500,0,0|31,331,0,0|4,304,0,0
\u0628\u063a	127,427,0,0|4,304,0,0
\u0641	221,521,0,0
\u0641\u0628	200,500,0,0|128,428,0,0
\u0628\u0641\u0628	200,500,0,0|129,429,0,0|4,304,0,0
\u0628\u0641	221,521,0,0|4,304,0,0
\u0642	222,522,0,0
\u0642\u0628	200,500,0,0|131,431,0,0
\u0628\u0642\u0628	200,500,0,0|138,438,0,0|4,304,0,0
\u0628\u0642	222,522,0,0|4,304,0,0
\u0643	223,523,0,0
\u0643\u0628	200,500,0,0|140,440,0,0
\u0628\u0643\u0628	200,500,0,0|140,440,0,0|4,304,0,0
\u0628\u0643	223,523,0,0|4,304,0,0
\u0644	225,525,0,0
\u0644\u0628	200,500,0,0|141,441,0,0
\u0628\u0644\u0628	200,500,0,0|141,441,0,0|4,304,0,0
\u0628\u0644	225,525,0,0|4,304,0,0
\u0645	227,527,0,0
\u0645\u0628	200,500,0,0|142,442,0,0
\u0628\u0645\u0628	200,500,0,0|142,442,0,0|4,304,0,0
\u0628\u0645	227,527,0,0|4,304,0,0
\u0646	228,528,0,0
\u0646\u0628	200,500,0,0|143,443,0,0
\u0628\u0646\u0628	200,500,0,0|143,443,0,0|4,304,0,0
\u0628\u0646	228,528,0,0|4,304,0,0
\u0647	229,529,0,0
\u0647\u0628	200,500,0,0|144,444,0,0
\u0628\u0647\u0628	200,500,0,0|149,449,0,0|4,304,0,0
\u0628\u0647	152,452,0,0|4,304,0,0
\u0648	230,530,0,0
\u0648\u0628	200,500,0,0|230,530,0,0
\u0628\u0648\u0628	200,500,0,0|230,530,0,0|4,304,0,0
\u0628\u0648	230,530,0,0|4,304,0,0
\u0649	236,536,0,0
\u0649\u0628	200,500,0,0|154,454,0,0
\u0628\u0649\u0628	200,500,0,0|154,454,0,0|4,304,0,0
\u0628\u0649	160,460,0,0|4,304,0,0
\u064a	237,537,0,0
\u064a\u0628	200,500,0,0|154,454,0,0
\u0628\u064a\u0628	200,500,0,0|154,454,0,0|4,304,0,0
\u0628\u064a	156,456,0,0|4,304,0,0
\u06d2	255,555,0,0
\u06d2\u0628	200,500,0,0|255,555,0,0
\u0628\u06d2\u0628	200,500,0,0|255,555,0,0|4,304,0,0
\u0628\u06d2	255,555,0,0|4,304,0,0
\u0644\u0627	165,465,0,0
\u0628\u0644\u0627	170,470,0,0|4,304,0,0
\u0644\u064e\u0627	243,0,-52,862|165,465,0,0
\u0628\u0644\u064f\u0627	245,0,-47,862|170,470,0,0|4,304,0,0
\u0644\u0623	178,478,0,0
\u0628\u0644\u0623	179,479,0,0|4,304,0,0
\u0644\u064e\u0623	243,0,-41,862|178,478,0,0
\u0628\u0644\u064f\u0623	245,0,-41,862|179,479,0,0|4,304,0,0
\u0644\u0625	252,552,0,0
\u0628\u0644\u0625	255,555,0,0|4,304,0,0
\u0644\u064e\u0625	243,0,14,862|252,552,0,0
\u0628\u0644\u064f\u0625	245,0,16,862|255,555,0,0|4,304,0,0
\u0644\u0622	180,480,0,0
\u0628\u0644\u0622	185,485,0,0|4,304,0,0
\u0644\u064e\u0622	243,0,-40,862|180,480,0,0
\u0628\u0644\u064f\u0622	245,0,-37,862|185,485,0,0|4,304,0,0
\u0628\u0651\u064e	172,0,-150,862|200,500,0,0
\u0628\u0651\u064e\u0628	200,500,0,0|172,0,-248,862|4,304,0,0
\u0628\u064e\u0651	172,0,-150,862|200,500,0,0
\u0628\u0651\u064f	173,0,-150,862|200,500,0,0
\u0628\u0651\u064f\u0628	200,500,0,0|173,0,-248,862|4,304,0,0
\u0628\u064f\u0651	173,0,-150,862|200,500,0,0
\u0628\u0651\u0650	175,0,-150,862|200,500,0,0
\u0628\u0651\u0650\u0628	200,500,0,0|175,0,-248,862|4,304,0,0
\u0628\u0650\u0651	175,0,-150,862|200,500,0,0
\u0628\u064e\u0628\u0652\u0628	200,500,0,0|250,0,-248,862|4,304,0,0|243,0,-248,862|4,304,0,0
\u0627\u0627	199,499,0,0|199,499,0,0
\u0628\u0627	0,0,0,0|4,304,0,0
\u0640\u0628	200,500,0,0|220,520,0,0
\u0628\u0640	220,520,0,0|4,304,0,0
\u0628 \u0628	200,500,0,0|32,332,0,0|200,500,0,0
win1256-sukun-elsewhere
\u0628\u0628\u0628	200,500,0,0|200,500,0,0|200,500,0,0
\u0644\u0627	199,499,0,0|225,525,0,0
\u0628\u0644\u0627	199,499,0,0|225,525,0,0|200,500,0,0
\u0628\u0651\u064e	243,0,-150,1724|248,0,-150,862|200,500,0,0
\u0628\u0627	199,499,0,0|200,500,0,0
\u0647\u0628	200,500,0,0|229,529,0,0
\u0628\u0647\u0628	200,500,0,0|229,529,0,0|200,500,0,0
\u0628\u0647	229,529,0,0|200,500,0,0
\u0643\u0628	200,500,0,0|223,523,0,0
\u0628\u0649	236,536,0,0|200,500,0,0
win1256-one-form
\u0628\u0628\u0628	200,500,0,0|200,500,0,0|4,304,0,0
\u0644\u0627	199,499,0,0|225,525,0,0
\u0628\u0644\u0627	199,499,0,0|225,525,0,0|4,304,0,0
\u0628\u0651\u064e	243,0,-150,1724|248,0,-150,862|200,500,0,0
\u0628\u0627	199,499,0,0|4,304,0,0
\u0647\u0628	200,500,0,0|229,529,0,0
\u0628\u0647\u0628	200,500,0,0|229,529,0,0|4,304,0,0
\u0628\u0647	229,529,0,0|4,304,0,0
\u0643\u0628	200,500,0,0|223,523,0,0
\u0628\u0649	236,536,0,0|4,304,0,0
win1256-mark-ligature
\u0628\u0628\u0628	200,500,0,0|200,500,0,0|200,500,0,0
\u0644\u0627	199,499,0,0|225,525,0,0
\u0628\u0644\u0627	199,499,0,0|225,525,0,0|200,500,0,0
\u0628\u0651\u064e	5,0,-150,862|200,500,0,0
\u0628\u0627	199,499,0,0|200,500,0,0
\u0647\u0628	200,500,0,0|229,529,0,0
\u0628\u0647\u0628	200,500,0,0|229,529,0,0|200,500,0,0
\u0628\u0647	229,529,0,0|200,500,0,0
\u0643\u0628	200,500,0,0|223,523,0,0
\u0628\u0649	236,536,0,0|200,500,0,0
win1256-declares-init
\u0628\u0628\u0628	200,500,0,0|200,500,0,0|200,500,0,0
\u0644\u0627	199,499,0,0|225,525,0,0
\u0628\u0644\u0627	199,499,0,0|225,525,0,0|200,500,0,0
\u0628\u0651\u064e	243,0,-150,1724|248,0,-150,862|200,500,0,0
\u0628\u0627	199,499,0,0|200,500,0,0
\u0647\u0628	200,500,0,0|229,529,0,0
\u0628\u0647\u0628	200,500,0,0|229,529,0,0|200,500,0,0
\u0628\u0647	229,529,0,0|200,500,0,0
\u0643\u0628	200,500,0,0|223,523,0,0
\u0628\u0649	236,536,0,0|200,500,0,0
`

// TestFormsDrawnFromTheCharacterMapFollowTheNeighbours: a face whose forms
// the fallback draws is one whose forms depend on the letters either side, and
// the questions a line breaker asks before it shapes a word apart from its
// neighbours say so. They read the plan's lookups alone, so for this face they
// said the forms could not change, and a word cut at a line's end kept the
// forms it had had whole.
func TestFormsDrawnFromTheCharacterMapFollowTheNeighbours(t *testing.T) {
	fonts := arabicFallbackFixtures()
	for _, c := range []struct {
		font string
		want bool
	}{{"cmap-only", true}, {"declares-init", false}} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatal(err)
		}
		if got := f.FormsFollowNeighbours("\u0628", Features{}); got != c.want {
			t.Errorf("%s: FormsFollowNeighbours = %v, want %v", c.font, got, c.want)
		}
		if got := f.ContextCanChange("\u0628", Features{}); got != c.want {
			t.Errorf("%s: ContextCanChange = %v, want %v", c.font, got, c.want)
		}
		if got := f.FormsFollowNeighbours("\u064E", Features{}); got != c.want {
			t.Errorf("%s: FormsFollowNeighbours of a mark = %v, want %v", c.font, got, c.want)
		}
		if got := f.HasJoiningForms(); got != c.want {
			t.Errorf("%s: HasJoiningForms = %v, want %v", c.font, got, c.want)
		}
	}
}

// TestTheSynthesizedCoverageIsHarfBuzzs: a coverage built here is in the
// format HarfBuzz's Coverage::serialize would write it in — a list where that
// is no more than three times the runs of consecutive glyphs, the runs
// otherwise — and a glyph given twice is kept twice. The format is what
// decides, through the search each is read by, which of two letters sharing a
// glyph a lookup answers for.
func TestTheSynthesizedCoverageIsHarfBuzzs(t *testing.T) {
	for _, c := range []struct {
		glyphs []int
		want   []int // the table, sixteen bits at a time
	}{
		{[]int{3, 9, 20}, []int{1, 3, 3, 9, 20}},
		{[]int{4, 4, 5}, []int{1, 3, 4, 4, 5}},
		{[]int{1, 2, 3, 7}, []int{1, 4, 1, 2, 3, 7}},
		{[]int{1, 2, 3, 4, 5, 6, 7, 20}, []int{2, 2, 1, 7, 0, 20, 20, 7}},
		{[]int{5, 6, 7, 8, 8, 9, 10, 11}, []int{2, 2, 5, 8, 0, 8, 11, 4}},
	} {
		got := serializeCoverage(c.glyphs)
		var words []int
		for i := 0; i+1 < len(got); i += 2 {
			words = append(words, int(got[i])<<8|int(got[i+1]))
		}
		if fmt.Sprint(words) != fmt.Sprint(c.want) {
			t.Errorf("coverage of %v = %v, want %v", c.glyphs, words, c.want)
		}
		// A coverage is read at an offset from the subtable naming it.
		sub := append([]byte{0, 0}, got...)
		for i, g := range c.glyphs {
			if at, ok := coverageIndex(sub, 2, g); !ok || c.glyphs[at] != g {
				t.Errorf("coverage of %v: glyph %d (at %d) reads as index %d, %v", c.glyphs, g, i, at, ok)
			}
		}
	}
}

// TestWriteArabicFallbackFixtures writes the fixtures out for asking HarfBuzz,
// where FORME_ARABIC_FIXTURES names a directory; it does nothing otherwise.
func TestWriteArabicFallbackFixtures(t *testing.T) {
	dir := os.Getenv("FORME_ARABIC_FIXTURES")
	if dir == "" {
		t.Skip("FORME_ARABIC_FIXTURES is not set")
	}
	for _, fixtures := range []map[string][]byte{arabicFallbackFixtures(), win1256Fixtures()} {
		for name, data := range fixtures {
			if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
