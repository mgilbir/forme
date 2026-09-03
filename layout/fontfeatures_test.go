package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The two CSS Fonts properties that turn a font's own rules off, and CSS Text
// §8.2's rule that does the same thing for a different reason.
//
// A font states what it wants done to its glyphs and a document may overrule
// it. Nothing here adds a feature and nothing here asks a face whether it has
// one: a face that declares no ligatures is unaffected by a rule that turns
// ligatures off.
//
// The fixtures need a face that really ligates and really kerns, which is what
// the suite ships Lato-Medium-Liga.ttf for.

// ligatingFace is a face with an "ffi" ligature in it.
func ligatingFace(t *testing.T) *shape.Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(wptDir(t), "fonts", "Lato-Medium-Liga.ttf"))
	if err != nil {
		t.Skip("the checkout has no Lato-Medium-Liga.ttf")
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Skipf("Lato-Medium-Liga.ttf could not be read: %v", err)
	}
	return face
}

// shapedIn is the glyphs a face sets a string as, with some features off.
func shapedIn(t *testing.T, face *shape.Face, s string, off shape.Features) []shape.Glyph {
	t.Helper()
	gs, _ := face.ShapeGlyphsInContext(s, "", "", off)
	return gs
}

// TestOptionalLigaturesCanBeTurnedOff.
func TestOptionalLigaturesCanBeTurnedOff(t *testing.T) {
	face := ligatingFace(t)
	on := shapedIn(t, face, "office", shape.Features{})
	if len(on) != 4 {
		t.Skipf("this face set \"office\" as %d glyphs; the fixture needs a ligature",
			len(on))
	}
	off := shapedIn(t, face, "office", shape.Features{NoOptionalLigatures: true})
	if len(off) != 6 {
		t.Errorf("with the optional ligatures off the face set \"office\" as %d "+
			"glyphs, want 6 — one per letter", len(off))
	}
	// And the advance follows: a run measured with the ligature and drawn
	// without it is a line filled to one width and painted at another.
	withLig := face.MeasureShapedInContext("office", 20, "", "", true, shape.Features{})
	without := face.MeasureShapedInContext("office", 20, "", "", true,
		shape.Features{NoOptionalLigatures: true})
	if withLig == without {
		t.Error("the two measure the same; the ligature is narrower than the " +
			"letters it replaces, so the measurement has to follow the shaping")
	}
}

// TestKerningCanBeTurnedOff.
//
// Lato-Medium rather than the ligating face beside it: the ligature subset the
// suite ships has the pairs stripped out, so the face reports kerning and kerns
// nothing. That is a fixture that would have proved nothing while looking as
// though it proved something, which is why the pair is measured rather than the
// face asked.
func TestKerningCanBeTurnedOff(t *testing.T) {
	face := kerningFace(t)
	for _, pair := range []string{"AV", "To", "P."} {
		on := face.MeasureShapedInContext(pair, 100, "", "", true, shape.Features{})
		off := face.MeasureShapedInContext(pair, 100, "", "", true,
			shape.Features{NoKerning: true})
		if on == off {
			t.Errorf("this face sets %q %g wide either way; the fixture needs a "+
				"pair it kerns", pair, on)
			continue
		}
		if off < on {
			t.Errorf("%q is %g wide with kerning off and %g with it on; this face "+
				"kerns the pair closer, so turning it off can only widen it",
				pair, off, on)
		}
	}
	// And the boundary pair, which is kerned by a pass of its own: a run whose
	// neighbour is set in the same face kerns across the boundary between them,
	// and turning kerning off has to reach that pass too.
	const a, b = "A", "V"
	joined := face.MeasureShapedInContext(a, 100, "", b, true, shape.Features{})
	apart := face.MeasureShapedInContext(a, 100, "", b, true,
		shape.Features{NoKerning: true})
	if joined == apart {
		t.Error("the boundary kern is not applied at all, so turning it off " +
			"proves nothing")
	}
}

// kerningFace is a face with kern pairs that actually fire.
func kerningFace(t *testing.T) *shape.Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(wptDir(t), "fonts", "Lato-Medium.ttf"))
	if err != nil {
		t.Skip("the checkout has no Lato-Medium.ttf")
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Skipf("Lato-Medium.ttf could not be read: %v", err)
	}
	return face
}

// TestTheTwoPropertiesTurnOffDifferentSets.
//
// CSS Fonts 4 §6.4 defines "font-variant-ligatures: none" to expand to
// "no-common-ligatures no-discretionary-ligatures no-historical-ligatures
// no-contextual", so it takes the contextual alternates with it. CSS Text §8.2
// turns off the optional *ligatures* and says nothing about alternates — an
// alternate is a different shape for one character, not two characters set as
// one, so a spacing between them has nothing to be between.
//
// Folding the two together would answer one rule's question with the other's
// set, so they are two fields and this is the check that they stay two.
func TestTheTwoPropertiesTurnOffDifferentSets(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      shape.Features
	}{
		{"nothing declared", "", shape.Features{}},
		{"ligatures none", "font-variant-ligatures: none",
			shape.Features{NoOptionalLigatures: true, NoContextualAlternates: true}},
		{"no common ligatures", "font-variant-ligatures: no-common-ligatures",
			shape.Features{NoOptionalLigatures: true}},
		{"no contextual", "font-variant-ligatures: no-contextual",
			shape.Features{NoContextualAlternates: true}},
		{"kerning none", "font-kerning: none", shape.Features{NoKerning: true}},
		{"kerning auto", "font-kerning: auto", shape.Features{}},
		{"letter-spacing", "letter-spacing: 1px",
			shape.Features{NoOptionalLigatures: true}},
		{"letter-spacing zero", "letter-spacing: 0", shape.Features{}},
		{"both", "font-kerning: none; font-variant-ligatures: none",
			shape.Features{NoOptionalLigatures: true, NoContextualAlternates: true,
				NoKerning: true}},
	} {
		if got := featuresOfBox(t, c.css); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.what, got, c.want)
		}
	}
}

// featuresOfBox is what a declaration turns off, read the way layout reads it.
func featuresOfBox(t *testing.T, css string) shape.Features {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">office</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-size: 20px; ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	l := &layouter{lengths: map[lengthKey]style.Length{}}
	var walk func(*Box) *Box
	walk = func(b *Box) *Box {
		if id, ok := boxAttr(b, "id"); ok && id == "d" {
			return b
		}
		for _, c := range b.Children {
			if got := walk(c); got != nil {
				return got
			}
		}
		return nil
	}
	d := walk(built.Root)
	if d == nil {
		t.Fatal("the document has no box with id d")
	}
	return l.featuresFor(d)
}

// TestSpacingSuppressesOptionalLigatures is CSS Text §8.2, and the reason is
// what a ligature is: two letters set as one glyph have no boundary between
// them for a spacing to be inserted at.
func TestSpacingSuppressesOptionalLigatures(t *testing.T) {
	const css = `body { margin: 0 }
	#d { font-size: 20px; width: 400px }`
	for _, c := range []struct {
		what, style string
		suppressed  bool
	}{
		{"nothing", "", false},
		{"letter-spacing", "letter-spacing: 5px", true},
		{"justified between characters",
			"text-align: justify; text-align-last: justify; text-justify: inter-character",
			true},
		{"justified between words",
			"text-align: justify; text-align-last: justify; text-justify: inter-word",
			false},
		{"inter-character but not justified", "text-justify: inter-character", false},
	} {
		got := featuresOfBox(t, c.style)
		if got.NoOptionalLigatures != c.suppressed {
			t.Errorf("with %s the optional ligatures are suppressed=%v, want %v",
				c.what, got.NoOptionalLigatures, c.suppressed)
		}
	}
}

// TestAnInlineBlockIsJustifiedOnItsOwnTerms.
//
// The walk that finds the justification stops at a box establishing its own
// formatting context, because such a box is justified on its own terms. The
// suite's letter-spacing-ligatures-001 writes exactly that — an inline-block
// with "text-justify: auto" inside a container justifying between characters —
// and asks for the ligature to survive inside it.
func TestAnInlineBlockIsJustifiedOnItsOwnTerms(t *testing.T) {
	built := Build(Input{
		HTML: `<div id="outer">o<span id="inner">ffi</span>ce</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `
			#outer { font-size: 20px; width: 400px; text-align: justify;
			         text-align-last: justify; text-justify: inter-character }
			#inner { display: inline-block; text-justify: auto }`}},
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	l := &layouter{lengths: map[lengthKey]style.Length{}}
	var find func(*Box, string) *Box
	find = func(b *Box, id string) *Box {
		if got, ok := boxAttr(b, "id"); ok && got == id {
			return b
		}
		for _, c := range b.Children {
			if got := find(c, id); got != nil {
				return got
			}
		}
		return nil
	}
	outer, inner := find(built.Root, "outer"), find(built.Root, "inner")
	if outer == nil || inner == nil {
		t.Fatal("the fixture did not produce both boxes")
	}
	if !l.featuresFor(outer).NoOptionalLigatures {
		t.Error("the container justifies between characters and keeps its ligatures")
	}
	if l.featuresFor(inner).NoOptionalLigatures {
		t.Error("the inline-block justifies on its own terms and lost its " +
			"ligatures anyway")
	}
}

// TestAValueThisEngineCannotReadIsNotGuessedAt.
//
// font-variant-ligatures takes four independent pairs and a document may write
// any of them in any order. What is read is what a document turning ligatures
// off writes; anything else leaves the font's rules alone, which is the value
// that changes nothing rather than a guess at what was meant.
func TestAValueThisEngineCannotReadIsNotGuessedAt(t *testing.T) {
	for _, css := range []string{
		"font-variant-ligatures: no-historical-ligatures",
		"font-variant-ligatures: common-ligatures no-contextual",
		"font-variant-ligatures: discretionary-ligatures",
	} {
		if got := featuresOfBox(t, css); got != (shape.Features{}) {
			t.Errorf("%q turned off %+v; a value this engine cannot read leaves "+
				"the font's own rules alone", css, got)
		}
	}
	if _, ok := ligaturesOf("no-historical-ligatures"); ok {
		t.Error("no-historical-ligatures reads as a value this engine understands")
	}
	if _, ok := ligaturesOf("none"); !ok {
		t.Error("none does not read as a value this engine understands")
	}
}

// TestTheRunCarriesWhatWasTurnedOff, so that the backend shapes it the way the
// engine measured it. A run measured with a ligature and drawn without one is a
// line filled to one width and painted at another.
func TestTheRunCarriesWhatWasTurnedOff(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      shape.Features
	}{
		{"nothing", "", shape.Features{}},
		{"ligatures off", "font-variant-ligatures: none",
			shape.Features{NoOptionalLigatures: true, NoContextualAlternates: true}},
		{"kerning off", "font-kerning: none", shape.Features{NoKerning: true}},
	} {
		var got shape.Features
		var found bool
		for _, op := range Paint(layoutOf(t, 400,
			`<div id="d" style="`+c.css+`">office</div>`,
			`body{margin:0} #d{font-size:20px}`)) {
			if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "off") {
				got, found = v.Features, true
			}
		}
		if !found {
			t.Fatalf("%s: the fixture drew no run", c.what)
		}
		if got != c.want {
			t.Errorf("%s: the run carries %+v, want %+v", c.what, got, c.want)
		}
	}
}

// boxAttr reads an attribute of the element a box came from.
func boxAttr(b *Box, name string) (string, bool) {
	if b == nil || b.Element == nil {
		return "", false
	}
	return b.Element.Attr(name)
}
