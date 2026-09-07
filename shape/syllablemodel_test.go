package shape

import "testing"

// Two productions of the syllable grammars that were read as looser than the
// model, and are not.
//
// Both were raised as differences from what the model states, and neither could
// be settled from this repository: the corpora hold no string that distinguishes
// the two readings, so the oracle passes either way. They were settled by asking
// HarfBuzz directly, with a real Noto face for each script, and the numbers
// below are its answers. Pinned here so that the same claim cannot be raised a
// second time without new evidence.

// TestASyllableTakesEveryCantillationMarkWrittenOnIt.
//
// syllable_tail ends in VD*, not VD VD?. A Vedic line carries several accents
// over one syllable, and cutting after the second would start a broken syllable
// and show a dotted circle in the middle of correctly written text.
//
// HarfBuzz, with Noto Sans Devanagari: one Ka and one, two, three or four Vedic
// marks all come back as a single cluster with no dotted circle, and this engine
// gives the same glyphs for all four.
func TestASyllableTakesEveryCantillationMarkWrittenOnIt(t *testing.T) {
	const (
		udatta   = 0x0951 // DEVANAGARI STRESS SIGN UDATTA
		anudatta = 0x0952 // DEVANAGARI STRESS SIGN ANUDATTA
		vedicA   = 0x1CDA // VEDIC TONE DOUBLE SVARITA
		vedicB   = 0x1CDB // VEDIC TONE TRIPLE SVARITA
	)
	f := devaFace(t)
	marks := []rune{udatta, anudatta, vedicA, vedicB}
	for n := 1; n <= len(marks); n++ {
		s := str(append([]rune{devKa}, marks[:n]...)...)
		glyphs, _ := f.ShapeGlyphs(s)
		clusters := map[int]bool{}
		for _, g := range glyphs {
			clusters[g.Cluster] = true
			if g.GID == gidDotted {
				t.Errorf("%d Vedic marks on a Ka drew a dotted circle; they are "+
					"one syllable and HarfBuzz says so too", n)
			}
		}
		if len(clusters) != 1 {
			t.Errorf("%d Vedic marks on a Ka came out as %d clusters, want one",
				n, len(clusters))
		}
	}
	// The control: something that really is a second syllable comes out as two,
	// so the count above is not one for want of ever being more.
	two := str(devKa, devKa)
	glyphs, _ := f.ShapeGlyphs(two)
	clusters := map[int]bool{}
	for _, g := range glyphs {
		clusters[g.Cluster] = true
	}
	if len(clusters) != 2 {
		t.Errorf("two Ka came out as %d clusters, want two", len(clusters))
	}
}

// TestATonedSyllableTakesMoreThanOneToneGroup.
//
// complex_tail ends in tone_group*, not tone_group?. "Ka, pwo tone, visarga,
// pwo tone" is one syllable.
//
// HarfBuzz, with Noto Sans Myanmar: that sequence is one cluster with no dotted
// circle, and so are "Ka SM PT", "Ka SM SM" and "Ka PT PT". The same run of
// probes does show a dotted circle for a tone mark or a medial written on its
// own, so it can see a broken cluster when there is one.
func TestATonedSyllableTakesMoreThanOneToneGroup(t *testing.T) {
	const (
		myPwoTone = 0x1063 // MYANMAR TONE MARK SGAW KAREN HATHI
		myVisarga = 0x1038 // MYANMAR SIGN VISARGA, a syllable modifier
	)
	f := myanmarFace(t)
	for _, tc := range []struct {
		name  string
		runes []rune
	}{
		{"a tone, a modifier and a tone", []rune{myKa, myPwoTone, myVisarga, myPwoTone}},
		{"a modifier and a tone", []rune{myKa, myVisarga, myPwoTone}},
		{"two modifiers", []rune{myKa, myVisarga, myVisarga}},
		{"two tones", []rune{myKa, myPwoTone, myPwoTone}},
	} {
		glyphs, _ := f.ShapeGlyphs(str(tc.runes...))
		clusters := map[int]bool{}
		for _, g := range glyphs {
			clusters[g.Cluster] = true
			if g.GID == gidMyDotted {
				t.Errorf("%s drew a dotted circle; it is one syllable and "+
					"HarfBuzz says so too", tc.name)
			}
		}
		if len(clusters) != 1 {
			t.Errorf("%s came out as %d clusters, want one", tc.name, len(clusters))
		}
	}
	// The control: a tone mark with no letter to sit on is a broken syllable,
	// and is shown as one.
	glyphs, _ := f.ShapeGlyphs(str(myPwoTone))
	found := false
	for _, g := range glyphs {
		if g.GID == gidMyDotted {
			found = true
		}
	}
	if !found {
		t.Error("a tone mark on its own drew no dotted circle, so this test " +
			"cannot see a broken syllable at all")
	}
}
