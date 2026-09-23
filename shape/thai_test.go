package shape

import (
	"slices"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestSaraAmIsTakenApartAndItsCircleMovedBack is audit C31. "น้ำ" — water —
// is NA, MAI THO and SARA AM, and SARA AM is drawn as a NIKHAHIT circle over the
// consonant and a SARA AA after it. The circle is stacked under the tone mark,
// so it is moved in front of it; set as written, the tone mark sat low and the
// circle floated over it. Where it moves, what it crossed becomes one cluster;
// where it does not, the two pieces keep the character they came from.
func TestSaraAmIsTakenApartAndItsCircleMovedBack(t *testing.T) {
	for _, c := range []struct {
		what        string
		text        string
		want        []rune
		wantOffsets []int
	}{
		{"after a tone mark", "น้ำ", []rune{0x0E19, 0x0E4D, 0x0E49, 0x0E32}, []int{0, 3, 3, 3}},
		{"after two above-base marks", "ปั่ำ", []rune{0x0E1B, 0x0E4D, 0x0E31, 0x0E48, 0x0E32}, []int{0, 3, 3, 3, 3}},
		{"with nothing to move across", "กำ", []rune{0x0E01, 0x0E4D, 0x0E32}, []int{0, 3, 3}},
		{"Lao", "ນ້ຳ", []rune{0x0E99, 0x0ECD, 0x0EC9, 0x0EB2}, []int{0, 3, 3, 3}},
		{"none at all", "กา", []rune{0x0E01, 0x0E32}, []int{0, 3}},
		// A NIKHAHIT written as itself is not moved: the rule is about the one
		// SARA AM brings.
		{"a written NIKHAHIT", "ก้ํา", []rune{0x0E01, 0x0E49, 0x0E4D, 0x0E32}, []int{0, 3, 6, 9}},
	} {
		runes := []rune(c.text)
		offsets := make([]int, 0, len(runes))
		for i := range c.text {
			offsets = append(offsets, i)
		}
		got, gotOffsets := thaiPreprocess(runes, offsets)
		if !slices.Equal(got, c.want) || !slices.Equal(gotOffsets, c.wantOffsets) {
			t.Errorf("%s: %U at %v, want %U at %v", c.what, got, gotOffsets, c.want, c.wantOffsets)
		}
	}
}

// TestSaraAmReachesTheFont is the same through a face: the glyphs come back in
// the stacked order.
func TestSaraAmReachesTheFont(t *testing.T) {
	const na, maiTho, nikhahit, saraAa = 1, 2, 3, 4
	f := planFace(t, []rune{0x0E19, 0x0E49, 0x0E4D, 0x0E32, 0x0E33}, nil)
	got, _ := f.ShapeGlyphs("น้ำ")
	if want := []int{na, nikhahit, maiTho, saraAa}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("glyphs %v, want %v", planGIDs(got), want)
	}
}

// TestAThaiFontWithNoThaiRulesStacksByPrivateUse. A Thai font from before
// OpenType draws its lowered and shifted marks at Private Use code points, and a
// shaper moves the marks to them: MAI THO on a consonant with no ascender is
// the lowered form, U+F70B in the Windows assignment. A font that states its own
// Thai rules is left to them.
func TestAThaiFontWithNoThaiRulesStacksByPrivateUse(t *testing.T) {
	const ko, maiTho, lowered = 1, 2, 3
	f := planFace(t, []rune{0x0E01, 0x0E49, 0xF70B}, nil)
	got, _ := f.ShapeGlyphs("ก้")
	if want := []int{ko, lowered}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("with no Thai rules: glyphs %v, want %v", planGIDs(got), want)
	}
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{99}, []int{98})}}},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}},
		onlyScript("thai", 1))
	f = planFace(t, []rune{0x0E01, 0x0E49, 0xF70B}, map[string][]byte{"GSUB": gsub})
	got, _ = f.ShapeGlyphs("ก้")
	if want := []int{ko, maiTho}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("with Thai rules of its own: glyphs %v, want %v", planGIDs(got), want)
	}
}
