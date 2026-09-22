package style

import (
	"runtime"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// TestAComputedStyleCostsWhatItDeclares is audit C10: every element held all
// hundred and forty-eight registered properties in a map of its own, about
// 9,600 bytes whether it declared anything or not, so "<i></i>" repeated a
// quarter of a million times held 2.4 GB once styled.
//
// The bound is on bytes retained per element, and it is loose on purpose: 512
// is five to eight times what the cases below cost now (about 60 to 100 bytes,
// most of it the entry in Styled.Styles) and nearly twenty times less than the
// map cost. The three cases are the three ways an element can cost more than
// nothing:
//
//   - one that declares nothing shares its parent's inherited values outright;
//   - one that changes an inherited value — every <i> below makes its text
//     italic — needs a block of its own, and its siblings' blocks are the same
//     one, which is what the per-document interning is for (without it each of
//     these costs about a kilobyte, and this fails);
//   - one that sets non-inherited values, in em so that they are rewritten per
//     element, stores only those.
func TestAComputedStyleCostsWhatItDeclares(t *testing.T) {
	const n = 20000
	for _, tc := range []struct {
		name, markup, css string
	}{
		{"nothing declared", "<span></span>", ""},
		{"an inherited value changed", "<i></i>", "i { font-style: italic }"},
		{"non-inherited values in em", "<p></p>", "p { margin: 1em 0; text-indent: 2em }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, _, _ := html.Parse(strings.Repeat(tc.markup, n))
			var sheets []Sheet
			if tc.css != "" {
				sheets = append(sheets, author(t, tc.css))
			}
			before := heapInUse()
			got := Apply(doc, sheets)
			after := heapInUse()
			if len(got.Styles) < n {
				t.Fatalf("%d elements styled, want at least %d", len(got.Styles), n)
			}
			per := float64(after-before) / float64(len(got.Styles))
			if after < before {
				per = 0
			}
			if per > 512 {
				t.Errorf("each element retains %.0f bytes once styled, want at most "+
					"512: the computed style is proportional to the registry again "+
					"rather than to what the element declared", per)
			}
			runtime.KeepAlive(got)
			runtime.KeepAlive(doc)
		})
	}
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestAnElementThatDeclaresNothingSharesItsParentsValues is the structure the
// bound above rests on, asked directly: a child that changes no inherited value
// holds the very block its parent does, not a copy of it, and stores nothing of
// its own; and one that changes one has a block of its own that the parent's is
// not changed by.
func TestAnElementThatDeclaresNothingSharesItsParentsValues(t *testing.T) {
	doc := parseDoc(t, `<div id="d"><span id="s">x</span><em id="e">y</em></div>`)
	got := Apply(doc, []Sheet{author(t, "div { margin-left: 3px } em { color: red }")})
	d := got.Styles[elementFor(t, doc, "#d")]
	s := got.Styles[elementFor(t, doc, "#s")]
	e := got.Styles[elementFor(t, doc, "#e")]

	if s.inh != d.inh {
		t.Error("a child that declares nothing has a copy of its parent's inherited values")
	}
	if len(s.own) != 0 {
		t.Errorf("a child that declares nothing stores %d values of its own", len(s.own))
	}
	if d.Get("margin-left") != "3px" || s.Get("margin-left") != "0" {
		t.Errorf("margin-left is %q on the div and %q on the span, want 3px and 0: "+
			"a non-inherited value reached the child", d.Get("margin-left"), s.Get("margin-left"))
	}
	if e.inh == d.inh {
		t.Error("a child that changed color shares its parent's inherited values")
	}
	if e.Get("color") != "red" || d.Get("color") == "red" {
		t.Errorf("color is %q on the em and %q on the div", e.Get("color"), d.Get("color"))
	}
}

// TestComputedStyleReadsAsTheMapDid pins the accessor to the semantics the map
// had, for the things a caller could ask it.
func TestComputedStyleReadsAsTheMapDid(t *testing.T) {
	var zero ComputedStyle
	if !zero.IsZero() || zero.Get("color") != "" || zero.Len() != 0 {
		t.Error("the zero style is not empty")
	}
	if _, ok := zero.Lookup("color"); ok {
		t.Error("the zero style holds color")
	}
	for range zero.All() {
		t.Fatal("the zero style yields a property")
	}

	cs := Initial()
	if cs.Get("no-such-property") != "" {
		t.Error("an unregistered name reads as something")
	}
	if _, ok := cs.Lookup("margin-inline-start"); ok {
		t.Error("a logical name is in a computed style")
	}
	n := 0
	for name, v := range cs.All() {
		n++
		want := properties[name].initial
		if v != want || cs.Get(name) != want {
			t.Errorf("%s is %q in the initial style, want %q", name, v, want)
		}
	}
	if n != len(properties) || cs.Len() != len(properties) {
		t.Errorf("the initial style holds %d properties, want %d", n, len(properties))
	}

	// With is copy-on-write: the receiver is unchanged, for an inherited and a
	// non-inherited property both, and setting a value back to the initial
	// one stores nothing.
	red := cs.With("color", "red").With("margin-top", "1px")
	if cs.Get("color") == "red" || cs.Get("margin-top") == "1px" {
		t.Error("With changed the style it was called on")
	}
	if red.Get("color") != "red" || red.Get("margin-top") != "1px" {
		t.Errorf("With gave color %q and margin-top %q", red.Get("color"), red.Get("margin-top"))
	}
	back := red.With("margin-top", "0")
	if len(back.own) != 0 || red.Get("margin-top") != "1px" {
		t.Errorf("setting margin-top back to its initial value left %d own values, "+
			"and the style it came from reads %q", len(back.own), red.Get("margin-top"))
	}
	if inh := Inherited(red); inh.Get("color") != "red" || inh.Get("margin-top") != "0" {
		t.Errorf("Inherited gave color %q and margin-top %q", inh.Get("color"), inh.Get("margin-top"))
	}
	if Inherited(zero).Get("color") != properties["color"].initial {
		t.Error("the zero style does not inherit the initial values")
	}
}

// TestNoInitialValueHoldsAFontRelativeLength is what lets absolutiseLengths look
// only at the properties an element declared: a value it did not declare is its
// parent's, already rewritten, or an initial value, and an initial value in em
// would be one that the walk over every property used to rewrite and this does
// not.
func TestNoInitialValueHoldsAFontRelativeLength(t *testing.T) {
	size, _ := FromPx(20)
	for name, p := range properties {
		vals, errs := css.ParseComponentValues(p.initial)
		if len(errs) != 0 {
			continue
		}
		if absolutiseValues(vals, size, size) {
			t.Errorf("%s's initial value %q holds an em or a rem", name, p.initial)
		}
	}
}
