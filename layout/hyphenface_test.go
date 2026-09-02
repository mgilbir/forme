package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Two things a hyphen is, and both were being asked of the wrong face.
//
// It is text, so §8.1's fallback finds a face for it where the declared one has
// no glyph: U+2010 HYPHEN is the character "hyphenate-character: auto" wants and
// a great many faces do not have it — Courier is one, so every monospaced
// document here was hyphenated with U+002D instead.
//
// And it is a run, so §10.8.1 measures it against the font it is *in*. A hyphen
// borrowed from another face reaches as far as that face does, which is what the
// suite's hyphens-manual references draw: they write the character into the text
// by hand, where it goes through the ordinary fallback and takes the ordinary
// leading.
//
// And a float has words of its own. hyphenPointsIn skipped an out-of-flow box's
// whole subtree, so a floated box never got a hyphenation point however loudly it
// asked — which is what hyphens-vs-float-clearance-001 is four of.

// hyphenatedRuns is the text a fixture draws, run by run.
func hyphenatedRuns(t *testing.T, htmlSrc string) []string {
	t.Helper()
	var out []string
	for _, op := range paintOf(t, htmlSrc, "") {
		if v, ok := op.(DrawText); ok {
			out = append(out, v.Text)
		}
	}
	return out
}

// hyphenatedWithFallback is hyphenatedRuns against a font set that has faces to
// fall back to, which is what the character question needs: the standard
// fourteen alone have no U+2010 anywhere, so "auto" is right to choose U+002D
// there and the rule cannot be seen at all.
func hyphenatedWithFallback(t *testing.T, htmlSrc, css string) []string {
	t.Helper()
	if len(fallbackFacesInUse()) == 0 {
		t.Skip("no fallback faces; set NOTO_FONTS")
	}
	built := Build(Input{
		HTML: htmlSrc, CSS: []Stylesheet{{Source: css}}, Fonts: fontSetForWPT(),
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(400)
	var out []string
	for _, op := range Paint(Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))) {
		if v, ok := op.(DrawText); ok {
			out = append(out, v.Text)
		}
	}
	return out
}

const hyphenCSS = `width:6ch;font:16px monospace;hyphens:auto`

func TestAFloatedBoxIsHyphenated(t *testing.T) {
	plain := hyphenatedRuns(t, `<div lang=en style="`+hyphenCSS+`">hyphenate!</div>`)
	floated := hyphenatedRuns(t, `<div lang=en style="`+hyphenCSS+`;float:left">hyphenate!</div>`)
	if len(plain) < 3 {
		t.Fatalf("the plain box was not hyphenated at all: %q", plain)
	}
	if strings.Join(floated, "") != strings.Join(plain, "") {
		t.Errorf("floated the word is set as %q and in flow as %q; a float is a "+
			"formatting context of its own and its words are words", floated, plain)
	}
}

func TestAnOutOfFlowBoxStillDoesNotDivideAWord(t *testing.T) {
	// The other half of the same rule, and the reason the float's own text is
	// gathered aside rather than flushed: a box hung off the middle of a word
	// does not end the word around it.
	whole := hyphenatedRuns(t, `<div lang=en style="`+hyphenCSS+`">hyphenate!</div>`)
	split := hyphenatedRuns(t,
		`<div lang=en style="`+hyphenCSS+`">hyphen<span style="position:absolute">`+
			`</span>ate!</div>`)
	if strings.Join(split, "") != strings.Join(whole, "") {
		t.Errorf("with an absolutely positioned box inside it the word is set as "+
			"%q and without one as %q", split, whole)
	}
}

func TestTheHyphenIsSetInAFaceThatHasIt(t *testing.T) {
	got := hyphenatedWithFallback(t, `<div lang=en id=d>hyphenate!</div>`,
		`#d { `+hyphenCSS+` }`)
	found := false
	for _, r := range got {
		if r == "‐" {
			found = true
		}
		if r == "-" {
			t.Errorf("the word was broken with U+002D: %q — U+2010 is what "+
				"\"auto\" asks for and the fallback can set it", got)
		}
	}
	if !found {
		t.Errorf("the word was set as %q with no U+2010 in it", got)
	}
}

func TestTheHyphensLineIsAsTallAsTheFaceThatDrewIt(t *testing.T) {
	// §10.8.1 against the font the run is in. A hyphen borrowed from another
	// face reaches as far as that face does, so the line holding it is as tall
	// as the same hyphen written into the text by hand.
	auto := lineTopsWithFallback(t, `<div lang=en id=d>hyphenate!</div>`,
		`#d { `+hyphenCSS+` }`)
	byHand := lineTopsWithFallback(t,
		`<div lang=en id=d>hy`+"‐"+`<br>phen`+"‐"+`<br>ate!</div>`,
		`#d { `+hyphenCSS+`; hyphens: none }`)
	if len(auto) < 2 || len(byHand) < 2 {
		t.Fatalf("the fixtures drew %d and %d lines, want at least two each",
			len(auto), len(byHand))
	}
	if got, want := auto[1].Sub(auto[0]), byHand[1].Sub(byHand[0]); got != want {
		t.Errorf("the automatic hyphen makes lines %v apart and the same "+
			"character written by hand makes them %v apart", got, want)
	}
}

// lineTopsWithFallback is the baseline of each line a fixture draws, against a
// font set that has faces to fall back to.
//
// The fallback set is the point: with the standard fourteen alone the hyphen is
// U+002D in the word's own face, so the leading question cannot arise and a test
// that asked it there would pass however the code answered.
func lineTopsWithFallback(t *testing.T, htmlSrc, css string) []style.Unit {
	t.Helper()
	if len(fallbackFacesInUse()) == 0 {
		t.Skip("no fallback faces; set NOTO_FONTS")
	}
	built := Build(Input{
		HTML: htmlSrc, CSS: []Stylesheet{{Source: css}}, Fonts: fontSetForWPT(),
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(400)
	var out []style.Unit
	seen := map[style.Unit]bool{}
	for _, op := range Paint(Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))) {
		v, ok := op.(DrawText)
		if !ok || seen[v.At.Y] {
			continue
		}
		seen[v.At.Y] = true
		out = append(out, v.At.Y)
	}
	return out
}

func TestAStatedHyphenCharacterIsStillObeyed(t *testing.T) {
	// The property names a string and the string is printed as it stands,
	// wherever it has to be found a face.
	got := hyphenatedRuns(t,
		`<div lang=en style="`+hyphenCSS+`;hyphenate-character:'='">hyphenate!</div>`)
	if !strings.Contains(strings.Join(got, ""), "=") {
		t.Errorf("the word was set as %q, want the \"=\" the document asked for", got)
	}
}
