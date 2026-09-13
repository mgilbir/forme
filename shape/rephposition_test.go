package shape

import "testing"

// Where each script draws its reph.
//
// A reph is the Ra at the head of a syllable, turned into a stroke over it.
// Every Indic script has one and they disagree about where along the syllable it
// sits, which is what the reph_pos field is for: Oriya draws it straight after
// the main consonant, Bengali after the subjoined forms, Gurmukhi before them,
// Devanagari and Gujarati before the post-base forms, and Tamil, Telugu and
// Kannada after everything.
//
// Nothing had checked the disagreement. Only Devanagari has a font in the
// corpora, so five of the nine configurations had never been exercised, and
// indicAfterReph — the test the after-subjoined class makes — was at 0% coverage
// across every unit test and all 6253 reftest documents.
//
// The function is a pure one over a syllable's position classes, so it is asked
// directly rather than through a font. That is the level the disagreement is
// stated at, and it is the only level five of these scripts can be asked at
// without a font for each.

// rephSyllable is one syllable with a glyph in every class the reph could stop
// at, so that the answers are distinguishable indices.
//
// It is the shape a syllable has *when this runs*, which is not the shape it was
// typed in: the Ra's own virama has already been consumed by the time the reph's
// position is chosen, so the Ra stands alone at the head with posRaToBecomeReph
// and the base follows it directly. Taken from a dump of Noto Sans Devanagari
// shaping र्क, र्क्य, र्कि, र्कों and र्क्रा rather than reasoned out — the first
// attempt at this fixture put the virama back and every script answered 1.
func rephSyllable() []indicInfo {
	return []indicInfo{
		{cat: catRa, pos: posRaToBecomeReph},   // 0: the Ra, virama already gone
		{cat: catConsonant, pos: posBaseC},     // 1: the base
		{cat: catConsonant, pos: posAfterMain}, // 2
		{cat: catConsonant, pos: posBelowC},    // 3: a subjoined form
		{cat: catConsonant, pos: posAfterSub},  // 4
		{cat: catConsonant, pos: posPostC},     // 5: a post-base form
		{cat: catMatra, pos: posSMVD},          // 6: a modifier, drawn last
	}
}

// planFor is a plan carrying one script's configuration and nothing else, which
// is all the reph positioning reads.
func planFor(t *testing.T, tag string) *indicPlan {
	t.Helper()
	cfg, ok := indicConfigs[tag]
	if !ok {
		t.Fatalf("%q is not a script this file reorders", tag)
	}
	return &indicPlan{cfg: cfg}
}

func TestEachScriptDrawsItsRephWhereItSaysItDoes(t *testing.T) {
	info := rephSyllable()
	const start, base = 0, 1
	end := len(info)

	for _, c := range []struct {
		tag  string
		want int
		why  string
	}{
		// The two classes with a case of their own stop partway.
		{"ory2", 2, "Oriya draws it after the main consonant"},
		{"mlm2", 2, "Malayalam the same, which the doc comment used to leave out"},
		{"bng2", 4, "Bengali draws it after the subjoined forms"},

		// The rest fall through to the end of the syllable, and stop inside the
		// modifiers: an anusvara is drawn over the syllable and the reph belongs
		// under it, which a font can only make one glyph of in that order.
		{"dev2", 5, "Devanagari draws it before the post-base forms"},
		{"gjr2", 5, "Gujarati the same"},
		{"gur2", 5, "Gurmukhi draws it before the subjoined forms"},
		{"tml2", 5, "Tamil draws it after everything"},
		{"tel2", 5, "Telugu the same"},
		{"knd2", 5, "Kannada the same"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			got := indicRephPosition(info, planFor(t, c.tag), start, end, base)
			if got != c.want {
				t.Errorf("%s: the reph went to %d (position class %d), want %d "+
					"(class %d) — %s", c.tag, got, info[got].pos, c.want,
					info[c.want].pos, c.why)
			}
		})
	}

	// Oriya and Bengali are the two the switch has cases for, and they must
	// differ: if they ever agree, one of the cases has stopped being reached and
	// the table above would go on passing with the wrong reason.
	ory := indicRephPosition(info, planFor(t, "ory2"), start, end, base)
	bng := indicRephPosition(info, planFor(t, "bng2"), start, end, base)
	if ory >= bng {
		t.Errorf("Oriya put the reph at %d and Bengali at %d; after the main "+
			"consonant is earlier than after the subjoined forms", ory, bng)
	}
}

// A virama still standing between the reph and the base takes it, whatever the
// script says — and it is the first step for every script but the after-post
// ones, which check it last.
//
// This is the step the specification calls the usual answer for a modern font,
// and over Noto Sans Devanagari it is never the answer at all: the half-form
// feature substitutes the consonant and its virama into one glyph, so nothing is
// left standing. It is reached when a font declines to make that form.
func TestAStandingViramaTakesTheReph(t *testing.T) {
	info := []indicInfo{
		{cat: catRa, pos: posRaToBecomeReph},
		{cat: catConsonant, pos: posPreC},
		{cat: catHalant, pos: posPreC}, // 2: the half form the font did not make
		{cat: catConsonant, pos: posBaseC},
		{cat: catConsonant, pos: posAfterSub},
		{cat: catMatra, pos: posSMVD},
	}
	for _, tag := range []string{"dev2", "bng2", "ory2", "gur2", "gjr2",
		"tml2", "tel2", "knd2", "mlm2"} {
		t.Run(tag, func(t *testing.T) {
			got := indicRephPosition(info, planFor(t, tag), 0, len(info), 3)
			if got != 2 {
				t.Errorf("%s put the reph at %d; a virama still standing before "+
					"the base takes it, which is index 2 here, so that the reph "+
					"sits on what the half form made", tag, got)
			}
		})
	}

	// And a joiner after that virama goes with it: the joiner asked for the form
	// the reph is to sit on, so the reph goes past both.
	withJoiner := []indicInfo{
		{cat: catRa, pos: posRaToBecomeReph},
		{cat: catConsonant, pos: posPreC},
		{cat: catHalant, pos: posPreC},
		{cat: catZWJ, pos: posPreC}, // 3
		{cat: catConsonant, pos: posBaseC},
		{cat: catMatra, pos: posSMVD},
	}
	if got := indicRephPosition(withJoiner, planFor(t, "dev2"), 0, len(withJoiner), 4); got != 3 {
		t.Errorf("with a joiner after the virama the reph went to %d, want 3; "+
			"the joiner asked for the form the reph is to sit on, so the reph "+
			"goes past both", got)
	}
}

// indicAfterReph's own three classes, since it is what the after-subjoined walk
// stops on: a class added to it or taken from it moves the stroke.
func TestWhatARephMustBeDrawnBefore(t *testing.T) {
	for _, p := range []indicPos{posPostC, posAfterPost, posSMVD} {
		if !indicAfterReph(p) {
			t.Errorf("position class %d is one the reph is drawn before, and "+
				"indicAfterReph says it is not", p)
		}
	}
	for _, p := range []indicPos{posStart, posRaToBecomeReph, posPreM, posPreC,
		posBaseC, posAfterMain, posAboveC, posBeforeSub, posBelowC, posAfterSub,
		posBeforePost, posFinalC, posEnd} {
		if indicAfterReph(p) {
			t.Errorf("position class %d is one the reph may be drawn after, and "+
				"indicAfterReph says it must come first", p)
		}
	}
}
