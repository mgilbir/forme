package layout

import (
	"testing"
)

// TestAGridItemKeepsTheSizeItDeclared is §6.6's clause the sizing had nowhere.
//
// "stretch" — which is what justify-self and align-self start at — sizes an
// item to its grid area only where the item's own size in that axis is auto. It
// was applied unconditionally: an item asking to be fifty by twenty in a
// two-hundred-by-hundred cell came out two hundred by a hundred, and flex's own
// stretchesAcross has always asked this question.
func TestAGridItemKeepsTheSizeItDeclared(t *testing.T) {
	const doc = `<div id="g"><div id="i">x</div></div>`
	for _, tc := range []struct {
		name         string
		css          string
		wantW, wantH float64
	}{
		{"a width and a height",
			`#i { width: 50px; height: 20px }`, 50, 20},
		{"a width alone",
			`#i { width: 50px }`, 50, 100},
		{"a height alone",
			`#i { height: 20px }`, 200, 20},
		{"neither, which is what stretch is for",
			``, 200, 100},
	} {
		css := noDefaults + `#g { display: grid; grid-template-columns: 200px;
			grid-template-rows: 100px; width: 200px }` + tc.css
		got, ok := gridRect(t, doc, css, "i")
		if !ok {
			t.Errorf("%s: the item generated no fragment", tc.name)
			continue
		}
		if got.W.Px() != tc.wantW || got.H.Px() != tc.wantH {
			t.Errorf("%s: the item is %.0f x %.0f, want %.0f x %.0f",
				tc.name, got.W.Px(), got.H.Px(), tc.wantW, tc.wantH)
		}
	}
}

// TestAGridItemsPercentageIsOfItsArea. §6.6 makes the grid area the containing
// block for an item's percentages, and half of a two-hundred-pixel cell is a
// hundred pixels. It came out at two hundred, because the percentage was never
// read at all and the stretch took the cell.
func TestAGridItemsPercentageIsOfItsArea(t *testing.T) {
	const doc = `<div id="g"><div id="i">x</div></div>`
	css := noDefaults + `#g { display: grid; grid-template-columns: 200px;
		grid-template-rows: 100px; width: 200px }
		#i { width: 50%; height: 25% }`
	got, ok := gridRect(t, doc, css, "i")
	if !ok {
		t.Fatal("the item generated no fragment")
	}
	if got.W.Px() != 100 || got.H.Px() != 25 {
		t.Errorf("the item is %.0f x %.0f, want 100 x 25", got.W.Px(), got.H.Px())
	}
}

// TestAPercentageRowIsOfTheHeight is the axis the track sizing read off the
// wrong one.
//
// A percentage in a track list is of the container's size on that track's own
// axis. Every list was resolved against the container's *width*, and always as
// though it were definite: two rows of "50%" in a four-hundred-pixel-tall box
// put the second at a hundred — half the width — and in a box with no height
// they made two rows of that same number out of ten pixels of content.
func TestAPercentageRowIsOfTheHeight(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`
	css := noDefaults + `#g { display: grid; grid-template-rows: 50% 50%;
		width: 200px; height: 400px }
		#a, #b { background: red }`
	a, ok := gridRect(t, doc, css, "a")
	if !ok {
		t.Fatal("the first item generated no fragment")
	}
	b, _ := gridRect(t, doc, css, "b")
	if a.H.Px() != 200 {
		t.Errorf("the first row is %.0f tall, want 200 — half of the height", a.H.Px())
	}
	if b.Y.Sub(a.Y).Px() != 200 {
		t.Errorf("the second row starts %.0f below the first, want 200",
			b.Y.Sub(a.Y).Px())
	}
}

// TestAPercentageRowWithNoHeightIsAuto is what CSS says to do when there is no
// definite size to take a percentage of: §7.2.1 makes such a track behave as
// "auto". Resolving it against the width instead invented a size out of
// nothing — two two-hundred-pixel rows in a box holding two lines of text.
func TestAPercentageRowWithNoHeightIsAuto(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`
	pct := noDefaults + `#g { display: grid; grid-template-rows: 50% 50%; width: 200px }`
	auto := noDefaults + `#g { display: grid; grid-template-rows: auto auto; width: 200px }`

	got, ok := gridRect(t, doc, pct, "g")
	if !ok {
		t.Fatal("the container generated no fragment")
	}
	want, _ := gridRect(t, doc, auto, "g")
	if got.H != want.H {
		t.Errorf("with percentage rows the container is %.0f tall and with automatic "+
			"rows %.0f; a percentage of no height is not a height",
			got.H.Px(), want.H.Px())
	}
}

// TestAPercentageRowGapIsOfTheHeight is the same question about the space
// between the tracks, which Box Alignment §8 answers the same way.
func TestAPercentageRowGapIsOfTheHeight(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`
	css := noDefaults + `#g { display: grid; grid-template-rows: 100px 100px;
		row-gap: 10%; width: 400px; height: 200px }
		#a, #b { background: red }`
	a, ok := gridRect(t, doc, css, "a")
	if !ok {
		t.Fatal("the first item generated no fragment")
	}
	b, _ := gridRect(t, doc, css, "b")
	if gap := b.Y.Sub(a.Y.Add(a.H)).Px(); gap != 20 {
		t.Errorf("the gap between the rows is %.0f, want 20 — a tenth of the height "+
			"rather than of the width", gap)
	}
}

// TestAPercentageColumnIsStillOfTheWidth is the axis that was right, kept
// right: a change that took every percentage off the block size would break the
// commoner half of this.
func TestAPercentageColumnIsStillOfTheWidth(t *testing.T) {
	const doc = `<div id="g"><div id="a">a</div><div id="b">b</div></div>`
	css := noDefaults + `#g { display: grid; grid-template-columns: 25% 75%; width: 400px }
		#a, #b { background: red }`
	a, ok := gridRect(t, doc, css, "a")
	if !ok {
		t.Fatal("the first item generated no fragment")
	}
	b, _ := gridRect(t, doc, css, "b")
	if a.W.Px() != 100 {
		t.Errorf("the first column is %.0f wide, want 100", a.W.Px())
	}
	if b.X.Sub(a.X).Px() != 100 {
		t.Errorf("the second column starts %.0f across, want 100", b.X.Sub(a.X).Px())
	}
}
