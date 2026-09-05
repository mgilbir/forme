package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Color 4 §3.1, opacity. See layout/opacity.go for why the alpha is folded
// into each mark rather than expressed as a group, and for the two cases where
// folding *is* the group.
//
// Every assertion below names an alpha. "The box came out lighter" is true for
// several wrong reasons — a colour that half-parsed, a background painted twice,
// a fill that was dropped — so each test says which mark it expects and what
// alpha it carries, and the ones about the report say which box it names.

// alphasOf returns the alpha of every fill of a colour, in paint order, ignoring
// what alpha the fill carries. Matching on the colour alone is the point: the
// test is about the alpha, so it must not be part of finding the mark.
func alphasOf(ops []Op, rgb style.RGBA) []float64 {
	var out []float64
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() &&
			r.Color.R == rgb.R && r.Color.G == rgb.G && r.Color.B == rgb.B {
			out = append(out, r.Color.A)
		}
	}
	return out
}

// soleAlpha requires exactly one fill of a colour and returns its alpha.
func soleAlpha(t *testing.T, ops []Op, rgb style.RGBA, what string) float64 {
	t.Helper()
	got := alphasOf(ops, rgb)
	if len(got) != 1 {
		t.Fatalf("%s: %d fills of %v, want 1\n%s", what, len(got), rgb, sketchClips(ops))
	}
	return got[0]
}

const box100 = `#b { width: 100px; height: 100px; background-color: #008000 }`

func TestAHalfOpaqueBoxPaintsAHalfOpaqueMark(t *testing.T) {
	ops := paintOf(t, `<div id="b"></div>`, noDefaults+box100+`#b { opacity: 0.5 }`)
	if got := soleAlpha(t, ops, green, "a half-opaque box"); got != 0.5 {
		t.Errorf("the box painted its background at alpha %v, want 0.5", got)
	}
	// The same document without the declaration, so that the number above is
	// the property's doing and not the colour's.
	full := paintOf(t, `<div id="b"></div>`, noDefaults+box100)
	if got := soleAlpha(t, full, green, "an opaque box"); got != 1 {
		t.Errorf("a box with no opacity painted at alpha %v, want 1", got)
	}
}

// TestOpacityOnAnInlineReachesTheBlockItWasSplitAround is
// css/CSS2/stacking-context/opacity-affects-block-in-inline, written out.
//
// §9.2.1.1 lifts the block out of the span and makes it a sibling of the span's
// two halves, so nothing in the box tree says it is inside the span any more.
// The opacity still covers it, and splitFrom is what carries that. Without the
// splitFrom clause in dimming the block paints fully opaque and this document
// disagrees with a reference that is one half-opaque square.
func TestOpacityOnAnInlineReachesTheBlockItWasSplitAround(t *testing.T) {
	ops := paintOf(t,
		`<span id="s"><div id="b"></div></span>`,
		noDefaults+box100+`#s { opacity: 0.5 }`)
	if got := soleAlpha(t, ops, green, "a block inside a half-opaque span"); got != 0.5 {
		t.Errorf("the block painted at alpha %v, want the span's 0.5", got)
	}
}

// TestNestedOpacitiesMultiply: a group inside a group is composited twice, and
// the marks in it carry the product. A fold that took the innermost opacity, or
// the outermost, gives 0.5 here and both are wrong.
func TestNestedOpacitiesMultiply(t *testing.T) {
	ops := paintOf(t,
		`<div id="outer"><div id="b"></div></div>`,
		noDefaults+box100+`#outer { opacity: 0.5 } #b { opacity: 0.5 }`)
	if got := soleAlpha(t, ops, green, "a half-opaque box in a half-opaque box"); got != 0.25 {
		t.Errorf("the nested box painted at alpha %v, want 0.5 * 0.5", got)
	}
}

// TestOpacityZeroPaintsNothing. Zero is the case the fold is exact for whatever
// the group holds, because everything in it is dropped — so the mark is gone
// rather than present and invisible, and nothing downstream has to know that a
// fill of no alpha is a fill of nothing.
func TestOpacityZeroPaintsNothing(t *testing.T) {
	ops := paintOf(t, `<div id="b"></div>`, noDefaults+box100+`#b { opacity: 0 }`)
	if got := alphasOf(ops, green); len(got) != 0 {
		t.Errorf("an invisible box painted %d green fills at %v", len(got), got)
	}
}

// TestOpacityIsClampedAndReadsPercentages: §3.1's value is a number or a
// percentage, clamped rather than rejected, and anything unreadable leaves the
// box alone.
func TestOpacityIsClampedAndReadsPercentages(t *testing.T) {
	for _, tc := range []struct {
		decl string
		want float64
	}{
		{"0.25", 0.25},
		{"25%", 0.25},
		{" 0.25 ", 0.25},
		{"1", 1},
		{"2", 1},
		{"-1", 0},
		{"120%", 1},
		{"", 1},
		{"half", 1},
		{"0.5px", 1},
	} {
		if got := opacityOf(style.ComputedStyle{"opacity": tc.decl}); got != tc.want {
			t.Errorf("opacity: %q read as %v, want %v", tc.decl, got, tc.want)
		}
	}
}

// TestOneMarkAloneIsNotReported is the exactness claim the whole design rests
// on. A group with one mark in it *is* the group — there is nothing under the
// mark to be hidden — so reporting it would be reporting an approximation that
// was not made, and the reftest suite would count a document that came out
// right as one that had not.
func TestOneMarkAloneIsNotReported(t *testing.T) {
	got := composeOf(t, `<div id="b"></div>`, Options{}, noDefaults+box100+`#b { opacity: 0.5 }`)
	for _, f := range got.Findings {
		if f.Property == "opacity" {
			t.Errorf("a group of one mark was reported: %s", f.Message)
		}
	}
	if n := len(alphasOf(got.Ops, green)); n != 1 {
		t.Fatalf("the document painted %d green fills, so it is not the one-mark "+
			"case this test is about", n)
	}
}

// TestAGroupThatOverlapsItselfIsReported is the other half, and the one that
// keeps the engine honest: a background with a border over it is two marks that
// lie on each other, the fold puts the lower one through the upper, and an
// author has no other way to learn that the box is not the one a browser draws.
func TestAGroupThatOverlapsItselfIsReported(t *testing.T) {
	got := composeOf(t, `<div id="b"></div>`, Options{}, noDefaults+box100+
		`#b { opacity: 0.5; border: 10px solid #ff0000 }`)
	var said string
	for _, f := range got.Findings {
		if f.Property == "opacity" {
			said = f.Message
			if f.Rule != RuleUnsupportedValue {
				t.Errorf("the report was raised as %q, want %q", f.Rule, RuleUnsupportedValue)
			}
			if !strings.Contains(f.Path, "div") {
				t.Errorf("the report names %q, which does not say which box it is about", f.Path)
			}
		}
	}
	if said == "" {
		t.Fatalf("a background under a border at opacity 0.5 was not reported\n%s",
			sketchClips(got.Ops))
	}
	if !strings.Contains(said, "over each other") {
		t.Errorf("the report says %q, which does not say what went wrong", said)
	}
	// And the marks are still painted, dimmed. A report is not a refusal.
	if a := alphasOf(got.Ops, green); len(a) != 1 || a[0] != 0.5 {
		t.Errorf("the reported box painted %v, want one green fill at 0.5", a)
	}
}

// TestAPictureCannotTakeAnAlphaAndSaysSo. A DrawImage carries pixels, so there
// is no colour in it to multiply and the fold cannot touch it — the box comes
// out opaque, which is a page the author did not ask for.
func TestAPictureCannotTakeAnAlphaAndSaysSo(t *testing.T) {
	pic := DrawImage{Rect: Rect{W: 1, H: 1}}
	ops, marks := dimOps([]Op{pic}, 0, 0.5)
	if len(ops) != 1 {
		t.Fatalf("the picture was dropped at alpha 0.5, and half a picture is "+
			"still a picture: %v", ops)
	}
	g := group{box: &Box{}, alpha: 0.5, marks: marks}
	why := g.unfaithful()
	if !strings.Contains(why, "picture") {
		t.Errorf("a group holding a picture was called %q, want it named the picture", why)
	}

	// At zero there is an exact answer and it is taken.
	ops, marks = dimOps([]Op{pic}, 0, 0)
	if len(ops) != 0 {
		t.Errorf("an invisible picture was still painted: %v", ops)
	}
	if why := (group{box: &Box{}, alpha: 0, marks: marks}).unfaithful(); why != "" {
		t.Errorf("a group at opacity 0 was reported as %q, and zero is exact", why)
	}
}

// TestTwoMarksThatMissEachOtherAreExact. The check is overlap and not count: two
// fills side by side hide nothing from each other, so folding is the group and
// there is nothing to report.
func TestTwoMarksThatMissEachOtherAreExact(t *testing.T) {
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	left := FillRect{Rect: Rect{u(0), u(0), u(10), u(10)}, Color: green}
	right := FillRect{Rect: Rect{u(20), u(0), u(10), u(10)}, Color: red}
	_, marks := dimOps([]Op{left, right}, 0, 0.5)
	if why := (group{box: &Box{}, alpha: 0.5, marks: marks}).unfaithful(); why != "" {
		t.Errorf("two fills that do not touch were reported as %q", why)
	}

	over := FillRect{Rect: Rect{u(5), u(0), u(10), u(10)}, Color: red}
	_, marks = dimOps([]Op{left, over}, 0, 0.5)
	if why := (group{box: &Box{}, alpha: 0.5, marks: marks}).unfaithful(); why == "" {
		t.Error("two fills that overlap were not reported, so the check is not " +
			"about overlap at all")
	}
}

// TestAnOutlineIsDimmedWithTheBoxItRings. An outline is painted in a pass of its
// own, after every stacking context — see painter.outlines — so it is the one
// mark of a box that a fold applied while the box was being painted would miss,
// and a half-opaque box with a solid ring round it is a page nobody asked for.
func TestAnOutlineIsDimmedWithTheBoxItRings(t *testing.T) {
	ops := paintOf(t, `<div id="b"></div>`, noDefaults+box100+
		`#b { opacity: 0.5; outline: 5px solid #ff0000 }`)
	ring := alphasOf(ops, red)
	if len(ring) == 0 {
		t.Fatalf("no outline was painted, so this test asserts nothing\n%s", sketchClips(ops))
	}
	for i, a := range ring {
		if a != 0.5 {
			t.Errorf("outline band %d painted at alpha %v, want the box's 0.5", i, a)
		}
	}
}

// TestACollapsedGridIsDimmedWithItsTable. The grid lines of a collapsing table
// are neither its background nor its content — they are painted by a step of
// their own, between the two — so they are the second mark a fold could miss.
func TestACollapsedGridIsDimmedWithItsTable(t *testing.T) {
	ops := paintOf(t,
		`<table id="t"><tr><td>a</td><td>b</td></tr></table>`,
		noDefaults+`#t { border-collapse: collapse; opacity: 0.5 }
		 #t td { border: 2px solid #ff0000 }`)
	lines := alphasOf(ops, red)
	if len(lines) == 0 {
		t.Fatalf("no grid lines were painted, so this test asserts nothing\n%s", sketchClips(ops))
	}
	for i, a := range lines {
		if a != 0.5 {
			t.Errorf("grid line %d painted at alpha %v, want the table's 0.5", i, a)
		}
	}
}

// TestTwoGroupsUnderOneGroupAreEachTheirOwn. Every box that asks for opacity
// keeps its own account of what it painted, and the accounts must not run into
// each other. Here the left box overlaps itself and the right one does not, so
// the left is reported and the right is not — and the wrapper round both is
// reported as well, because the left box's two marks are marks of the wrapper's
// group too.
//
// The mistake this is aimed at is crediting a mark to the innermost box that
// asked for opacity and stopping there. The wrapper would then be told about
// nothing at all — its group is made entirely of marks that belong to a box
// inside it — and a page whose whole content is composited twice would report
// only the half of it that a browser and this engine already agree about.
func TestTwoGroupsUnderOneGroupAreEachTheirOwn(t *testing.T) {
	got := composeOf(t, `<div id="w"><div id="l"></div><div id="r"></div></div>`, Options{},
		noDefaults+`
		#w { opacity: 0.5 }
		#l { width: 50px; height: 50px; background-color: #008000; opacity: 0.5;
		     border: 5px solid #ff0000 }
		#r { width: 50px; height: 50px; background-color: #008000; opacity: 0.5 }`)

	said := map[string]bool{}
	for _, f := range got.Findings {
		if f.Property == "opacity" {
			said[f.Path] = true
		}
	}
	for _, want := range []string{"html > body > div#w", "html > body > div#w > div#l"} {
		if !said[want] {
			t.Errorf("%s paints marks that lie over each other and was not reported", want)
		}
		delete(said, want)
	}
	for path := range said {
		t.Errorf("%s paints one mark and was reported anyway", path)
	}

	// Both boxes are inside #w, so both backgrounds are dimmed twice over.
	green := alphasOf(got.Ops, green)
	if len(green) != 2 {
		t.Fatalf("%d green fills, want one for each of the two boxes", len(green))
	}
	for i, a := range green {
		if a != 0.25 {
			t.Errorf("green fill %d painted at alpha %v, want 0.5 * 0.5", i, a)
		}
	}
}

// TestTextIsDimmedWithTheBoxItIsIn. A box's content is painted in a step of its
// own — §E.2's step 6, against step 4 for its background — so text is the third
// mark a fold could miss, and a half-opaque paragraph whose words came out black
// is the commonest thing anyone would notice.
func TestTextIsDimmedWithTheBoxItIsIn(t *testing.T) {
	ops := paintOf(t, `<p id="b">hello</p>`, noDefaults+`#b { opacity: 0.5; color: #ff0000 }`)
	var runs int
	for _, op := range ops {
		r, ok := op.(DrawText)
		if !ok || r.Text == "" {
			continue
		}
		runs++
		if r.Color.A != 0.5 {
			t.Errorf("the run %q was painted at alpha %v, want 0.5", r.Text, r.Color.A)
		}
	}
	if runs == 0 {
		t.Fatalf("no text was painted, so this test asserts nothing\n%s", sketchClips(ops))
	}
}

// dimmedFillAt is where the sole fill of a colour falls in paint order, which
// is what decides which of two marks is the visible one. It matches on the
// colour and not the alpha, which is what separates it from indexOfFill next
// door: every fill in these documents has been dimmed.
func dimmedFillAt(t *testing.T, ops []Op, rgb style.RGBA, what string) int {
	t.Helper()
	at := -1
	for i, op := range ops {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() &&
			r.Color.R == rgb.R && r.Color.G == rgb.G && r.Color.B == rgb.B {
			if at >= 0 {
				t.Fatalf("%s: more than one fill of %v\n%s", what, rgb, sketchClips(ops))
			}
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("%s: no fill of %v\n%s", what, rgb, sketchClips(ops))
	}
	return at
}

// TestAnOpacityGroupSealsItsDescendants. §3.1: a box with an opacity below one
// is a stacking context, painted as though its z-index were zero. So a
// "z-index: -1" inside it is sealed in and goes behind its siblings and not
// behind the group's own background — which is the whole of what a stacking
// context is for, and is a change the alpha alone would not make.
//
// The same document without the opacity is the control. There the negative box
// is hoisted into the context around it and painted at step 3, before every
// block background — so the wrapper's green covers it. That is the picture the
// declaration changes.
func TestAnOpacityGroupSealsItsDescendants(t *testing.T) {
	doc := `<div id="g"><div id="neg"></div></div>`
	base := noDefaults + `
		#g { position: relative; width: 100px; height: 100px;
		     background-color: #008000 }
		#neg { position: absolute; top: 0; left: 0; width: 100px; height: 100px;
		       z-index: -1; background-color: #ff0000 }`

	loose := paintOf(t, doc, base)
	if dimmedFillAt(t, loose, red, "a hoisted negative box") >
		dimmedFillAt(t, loose, green, "the wrapper's background") {
		t.Fatal("without an opacity the negative box was painted after the " +
			"wrapper's background, so this test's control is not the picture " +
			"the declaration is meant to change")
	}

	sealed := paintOf(t, doc, base+`#g { opacity: 0.5 }`)
	if dimmedFillAt(t, sealed, red, "a sealed negative box") <
		dimmedFillAt(t, sealed, green, "the group's background") {
		t.Error("a box with an opacity did not seal its descendants: a " +
			"\"z-index: -1\" inside it was painted behind its own background")
	}
}
