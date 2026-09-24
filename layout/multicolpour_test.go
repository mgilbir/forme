package layout

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// The pour, held to the literal one it replaced, and to the shape of its cost.

// randomColumnContent makes a fragment tree of the kind a pour is given, and a
// few kinds it is not: lines and boxes out of order, boxes whose content
// overflows them, boxes of no height, and boxes that refuse to be cut. The
// literal pour is the definition, so whatever it does with them is what the
// linear one has to do.
func randomColumnContent(r *rand.Rand, depth int, box *Box, refuser *Box) *Fragment {
	u := func(lo, hi int) style.Unit { return style.Unit(lo + r.Intn(hi-lo+1)) }
	f := &Fragment{Box: box}
	if depth > 0 && r.Intn(3) == 0 {
		f.Box = refuser
	}
	f.Border = Edges{Top: u(0, 2) * 32, Bottom: u(0, 2) * 32}
	f.Padding = Edges{Top: u(0, 2) * 32}
	var y style.Unit
	nLines := r.Intn(6)
	for i := 0; i < nLines; i++ {
		h := u(1, 4) * 64
		if r.Intn(8) == 0 {
			// Out of order: a line above the one before it, which a poured
			// multicol nested in another produces.
			f.Lines = append(f.Lines, LineFragment{Rect: Rect{Y: u(0, 8) * 64, H: h}})
			continue
		}
		f.Lines = append(f.Lines, LineFragment{Rect: Rect{Y: y, H: h}})
		y = y.Add(h)
	}
	if depth < 3 {
		nKids := r.Intn(4)
		for i := 0; i < nKids; i++ {
			c := randomColumnContent(r, depth+1, box, refuser)
			c.BorderRect.Y = y.Add(u(-1, 2) * 64)
			if r.Intn(6) == 0 {
				c.BorderRect.Y = u(0, 10) * 64
			}
			f.Children = append(f.Children, c)
			y = c.BorderRect.Bottom()
		}
	}
	inner := style.Unit(0)
	for _, line := range f.Lines {
		inner = style.Max(inner, line.Rect.Bottom())
	}
	for _, c := range f.Children {
		inner = style.Max(inner, c.BorderRect.Bottom())
	}
	h := inner.Add(f.Border.Vertical()).Add(f.Padding.Vertical())
	switch r.Intn(5) {
	case 0:
		h = 0 // overflowing a box of no height
	case 1:
		h = h.Div(2) // overflowing its box
	}
	f.BorderRect.H = h
	return f
}

// TestThePourIsTheLiteralPour compares the linear pour with the literal one on
// thousands of random trees, at every height a balanced pour could choose and
// at several column counts.
func TestThePourIsTheLiteralPour(t *testing.T) {
	plain := &Box{Style: style.Initial()}
	refuser := &Box{Style: style.Initial().With("box-decoration-break", "clone")}
	r := rand.New(rand.NewSource(1))
	compared, refused, forcedCompared := 0, 0, 0
	for trial := 0; trial < 800; trial++ {
		src := randomColumnContent(r, 0, plain, refuser)
		breaks := sortedBreaks(columnBreaks(src, 0, nil))
		heights := append([]style.Unit{0, 32, 100}, breaks...)
		for _, n := range []int{1, 2, 3, 7, 1000} {
			for _, h := range heights {
				c := columns{n: n, width: style.Unit(640), gap: style.Unit(64)}
				want := cloneForTest(src)
				got := cloneForTest(src)
				wantOK := fillColumnsByCopy(want, c, h)
				gotOK, _ := fillColumns(got, c, h)
				if wantOK != gotOK {
					t.Fatalf("trial %d, %d columns at %d: the literal pour said %v and this "+
						"one %v", trial, n, h, wantOK, gotOK)
				}
				if !wantOK {
					refused++
					continue
				}
				if d := fragmentDiff("pour", want, got); d != "" {
					t.Fatalf("trial %d, %d columns at %d: %s", trial, n, h, d)
				}
				compared++
			}
			if bh, ok := balancedHeight(breaks, nil, n); true {
				wh, wok := balancedHeightByScan(breaks, nil, n)
				if ok != wok || bh != wh {
					t.Fatalf("trial %d, %d columns: the balanced height is %d, %v by "+
						"halving and %d, %v by trying each", trial, n, bh, ok, wh, wok)
				}
			}
			// And with forced breaks: a random few of the breakpoints, the
			// last included now and then. The balanced height against the
			// scan over every height a column can have, and the pour at it —
			// each column ending at a forced break or at the last breakpoint
			// that fits — against the literal pour told the same.
			var forced []style.Unit
			for _, b := range breaks {
				if r.Intn(5) == 0 {
					forced = append(forced, b)
				}
			}
			bh, ok := balancedHeight(breaks, forced, n)
			wh, wok := balancedHeightByScan(breaks, forced, n)
			if ok != wok || bh != wh {
				t.Fatalf("trial %d, %d columns, forced at %v: the balanced height is %d, "+
					"%v by halving and %d, %v by trying each", trial, n, forced, bh, ok, wh, wok)
			}
			for _, h := range append([]style.Unit{bh}, heights...) {
				c := columns{n: n, width: style.Unit(640), gap: style.Unit(64)}
				snap := []style.Unit(nil)
				if h == bh && ok {
					snap = breaks
				}
				want, got := cloneForTest(src), cloneForTest(src)
				wantOK := fillColumnsByCopyWith(want, c, h, forced, snap)
				gotOK, _ := fillColumnsWith(got, c, h, &columnEnds{forced: forced, breaks: snap})
				if wantOK != gotOK {
					t.Fatalf("trial %d, %d columns at %d, forced at %v: the literal pour "+
						"said %v and this one %v", trial, n, h, forced, wantOK, gotOK)
				}
				if !wantOK {
					continue
				}
				if d := fragmentDiff("pour", want, got); d != "" {
					t.Fatalf("trial %d, %d columns at %d, forced at %v: %s",
						trial, n, h, forced, d)
				}
				forcedCompared++
			}
		}
	}
	// A comparison that never compared a successful pour, or never a refused
	// one, has not tested both halves.
	if compared < 500 || refused < 500 || forcedCompared < 500 {
		t.Fatalf("compared %d pours, %d refusals and %d pours with forced breaks; the "+
			"generator is not producing all three", compared, refused, forcedCompared)
	}
}

// cloneForTest is a deep copy, so that each pour is given its own tree.
func cloneForTest(f *Fragment) *Fragment {
	c := fragmentCloner{seen: map[*Fragment]*Fragment{}}
	return c.clone(f)
}

// fragmentDiff describes the first difference between two fragment trees, or
// is empty when there is none. Boxes, faces and images are compared by
// identity, which is what two layouts of one box tree share; everything else
// is compared by value, field by field.
func fragmentDiff(path string, a, b *Fragment) string {
	return diffValue(path, reflect.ValueOf(a), reflect.ValueOf(b), map[[2]uintptr]bool{})
}

var fragmentPtrType = reflect.TypeOf(&Fragment{})

func diffValue(path string, a, b reflect.Value, seen map[[2]uintptr]bool) string {
	if a.Type() != b.Type() {
		return fmt.Sprintf("%s: %v against %v", path, a.Type(), b.Type())
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return fmt.Sprintf("%s: nil on one side only", path)
			}
			return ""
		}
		if a.Pointer() == b.Pointer() {
			return ""
		}
		// Two pointers to different things are compared by what they point
		// at: a colour resolved twice is two allocations of one value, and a
		// box a layout makes for itself — a ::first-line piece — is made again
		// by the next one.
		key := [2]uintptr{a.Pointer(), b.Pointer()}
		if seen[key] {
			return ""
		}
		seen[key] = true
		return diffValue(path, a.Elem(), b.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if d := diffValue(path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i), seen); d != "" {
				return d
			}
		}
		return ""
	case reflect.Slice:
		if a.IsNil() != b.IsNil() && (a.Len() != 0 || b.Len() != 0) {
			return fmt.Sprintf("%s: nil against empty", path)
		}
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: %d against %d", path, a.Len(), b.Len())
		}
		for i := 0; i < a.Len(); i++ {
			if d := diffValue(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i), seen); d != "" {
				return d
			}
		}
		return ""
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				return fmt.Sprintf("%s: nil on one side only", path)
			}
			return ""
		}
		return diffValue(path, a.Elem(), b.Elem(), seen)
	case reflect.Array:
		for i := 0; i < a.Len(); i++ {
			if d := diffValue(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i), seen); d != "" {
				return d
			}
		}
		return ""
	case reflect.Map, reflect.Func, reflect.Chan:
		if a.Pointer() != b.Pointer() {
			return fmt.Sprintf("%s: a different %v", path, a.Type())
		}
		return ""
	default:
		if !valuesEqual(a, b) {
			return fmt.Sprintf("%s: %v against %v", path, printable(a), printable(b))
		}
		return ""
	}
}

func valuesEqual(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.Bool:
		return a.Bool() == b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() == b.Uint()
	case reflect.Float32, reflect.Float64:
		return a.Float() == b.Float()
	case reflect.String:
		return a.String() == b.String()
	}
	// A kind this does not know how to compare is a difference rather than a
	// match: a comparison that waves through what it cannot read is the kind
	// of oracle that passes everything.
	return false
}

func printable(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprint(v.Int())
	case reflect.String:
		return fmt.Sprintf("%q", v.String())
	case reflect.Bool:
		return fmt.Sprint(v.Bool())
	}
	return v.Kind().String()
}

// TestPouringIsLinearInTheLines is audit C23: one paragraph of a word per line
// in a multicol container, at n and four times n lines. Every column's cut
// copied everything below it, and a balanced height tried every breakpoint
// with a fit that scanned for the breakpoint before each one: 32,000 lines in
// a million columns took seventy-six seconds.
//
// The million columns are the copying's shape. Two columns are the balancing's,
// and a whole layout does not show it at any size the suite can afford: the
// balancing of before f6c6437, copied back in, read 6.8 to 7.4 here at a
// thousand lines against a bound of 8, and 10.8 at four thousand, where one
// layout of the larger side takes half a second. The lines are laid out in
// time linear in them either way, and at these sizes that is most of what is
// timed. So the case is kept for what it does hold — that two columns pour in
// time linear in the lines — and the balancing is held on its own by
// TestBalancingIsNotQuadraticInTheBreaks, which the same copy fails.
func TestPouringIsLinearInTheLines(t *testing.T) {
	for _, cols := range []string{"2", "1000000"} {
		doc := func(n int) Built {
			return Build(Input{HTML: `<div style="column-count:` + cols +
				`;width:600px"><p style="width:10px;margin:0">` +
				strings.Repeat("w ", n) + `</p></div>`})
		}
		small, large := doc(1000), doc(4000)
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(100000)
		var smallLines, largeLines int
		c := costtest.Time(t, "pouring n lines into column-count "+cols, func() {
			smallLines = pouredLines(Layout(small.Root, Size{W: w, H: h}, nil, nil))
		}, func() {
			largeLines = pouredLines(Layout(large.Root, Size{W: w, H: h}, nil, nil))
		})
		// The fixture has to be what it says: every word a line of its own, and
		// all of them poured. A fixture that fell back to one column would be
		// timing the fallback.
		if smallLines != 1000 || largeLines != 4000 {
			t.Fatalf("column-count %s: %d and %d lines were poured; the fixture is "+
				"meant to pour 1000 and 4000", cols, smallLines, largeLines)
		}
		if c.Ratio > 8 {
			t.Errorf("column-count %s: four times the lines took %.1f times as long "+
				"(%v against %v); a linear pour is about four", cols, c.Ratio, c.Large, c.Small)
		}
	}
}

// pouredLines counts the lines of a laid-out document whose multicol container
// was divided into columns — every line in every piece of the paragraph — and
// is zero where the pour was refused and the content laid out in one column.
func pouredLines(root *Fragment) int {
	var n, pieces int
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		if len(f.Lines) > 0 {
			n += len(f.Lines)
			pieces++
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if pieces < 2 {
		return 0
	}
	return n
}

// TestAPourIsBoundedInPieces is maxPourPieces firing: a pour that would make
// more fragments than the bound is refused, reported, and laid out in one
// column, which is what every other refused pour gets.
func TestAPourIsBoundedInPieces(t *testing.T) {
	defer func(n int) { maxPourPieces = n }(maxPourPieces)
	maxPourPieces = 50
	built := Build(Input{HTML: `<div style="column-count:4;width:400px"><p>` +
		strings.Repeat("w<br>", 200) + `</p></div>`})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(400)
	root := Layout(built.Root, Size{W: w, H: w}, nil, rec)
	said := false
	for _, f := range rec.Findings() {
		if strings.Contains(f.Message, "pieces") {
			said = true
		}
	}
	if !said {
		t.Errorf("a pour of 200 lines against a bound of 50 pieces said nothing: %v",
			findingList(rec.Findings()))
	}
	if n := pouredLines(root); n != 0 {
		t.Errorf("%d lines were poured; the pour was over the bound and is refused", n)
	}
}

// TestBalancingIsNotQuadraticInTheBreaks is the other half of C23. A balanced
// height is the first breakpoint the content fits under, and trying them in
// turn is a fit per breakpoint, each a walk of the breakpoints: quadratic in
// the lines before the pour itself begins. At the sizes above that is lost in
// the layout, so it is measured here on its own, where a paragraph of ten
// thousand lines in two columns shows it.
func TestBalancingIsNotQuadraticInTheBreaks(t *testing.T) {
	breaks := func(n int) []style.Unit {
		out := make([]style.Unit, n)
		for i := range out {
			out[i] = style.Unit((i + 1) * 64)
		}
		return out
	}
	small, large := breaks(10000), breaks(40000)
	var hs, hl style.Unit
	c := costtest.Time(t, "balancing n breakpoints in two columns",
		func() { hs, _ = balancedHeight(small, nil, 2) },
		func() { hl, _ = balancedHeight(large, nil, 2) })
	if hs != small[len(small)/2-1] || hl != large[len(large)/2-1] {
		t.Fatalf("two columns of equal lines balance at half of them: got %d and %d", hs, hl)
	}
	if c.Ratio > 8 {
		t.Errorf("four times the breakpoints took %.1f times as long (%v against %v); "+
			"a search by halving is about four", c.Ratio, c.Large, c.Small)
	}
}
