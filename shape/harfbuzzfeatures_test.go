package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The font-variant family checked against HarfBuzz.
//
// harfbuzz_test.go compares what a font turns on by itself. This compares what a
// *declaration* turns on, which is a different question and the one CSS Fonts 4
// §6.5 through §6.9 answer: which features a value asks for, and — where a value
// asks for more than one, or two properties are written together — what happens
// when their rules meet over the same glyphs.
//
// # Why it is an oracle and not a mirror
//
// Which OpenType features a CSS value asks for is stated twice: in this package,
// and again in testdata/harfbuzz/shapefeatures.py. That second copy is the whole
// point. A test that read this package's own table would agree with itself
// whatever the table said — "oldstyle-nums is 'lnum'" would pass — where the
// generator's table is written from the specification and HarfBuzz applies *it*.
//
// The corpus names CSS values rather than tags for the same reason: what is
// being compared is the chain from a declaration to a page, and a corpus of tags
// would start halfway along it.
//
// # And the order, which nothing states
//
// Neither side says an order. HarfBuzz is handed a set and walks the font's
// lookups in index order; applyRequestedFeatures exists to reproduce that, and
// before it did, "oldstyle-nums diagonal-fractions" set a line of oldstyle
// digits where a fraction was asked for. The rows that hold that down are the
// ones whose spec names two values whose lookups meet — see the corpus.
//
// # What it does not cover, and how that is known
//
// Six defects were planted against it and five failed. The two it cannot see are
// worth naming, because a corpus that finds nothing is indistinguishable from
// one that checks nothing until somebody tries:
//
//   - font-variant-east-asian, because the bundled face has no CJK in it and
//     every font here is checked in on purpose. §6.9's nine features are
//     compared against HarfBuzz in eastasian_test.go instead, over the fetched
//     Noto Sans JP and by hand rather than over a corpus.
//   - A lookup two features name, which applyRequestedFeatures runs once and
//     which running twice would apply to its own output. No font here states one
//     that way, so the case lives in a font built for it — see
//     TestALookupTwoFeaturesNameIsRunOnce. Removing the deduplication leaves
//     every case below passing.
//
// The five it does see are the ones this corpus exists for: the features applied
// tag by tag instead of in the font's lookup order (74 cases), a tag swapped in
// a property's table (91), a value that asks for two features forgetting one
// (9), a whole property dropped from what a run asks for (172), and the two
// positions exchanged (121).

// TestFeatureShapingAgreesWithHarfBuzz.
func TestFeatureShapingAgreesWithHarfBuzz(t *testing.T) {
	specs, corpus := readFeatureCorpus(t, "features.txt")
	expected, header := readFeatureExpectations(t, "features.expected.txt")
	if len(corpus) != len(expected) {
		t.Fatalf("%d cases in the corpus and %d lines of expectations; run "+
			"`make hbshaping`", len(corpus), len(expected))
	}
	if len(corpus) == 0 {
		t.Fatal("the corpus is empty, so this proves nothing")
	}

	data := notoSansBytes(t)
	sum := sha256.Sum256(data)
	if got, want := hex.EncodeToString(sum[:]), header["font-sha256"]; got != want {
		t.Fatalf("the expectations were generated against font %s and this one "+
			"is %s.\nRun `make hbshaping` to regenerate them.", want, got)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading the font: %v", err)
	}

	var differing int
	for i, text := range corpus {
		got, _ := f.ShapeGlyphsInContext(text, "", "", featuresOfSpec(t, specs[i]))
		same, why := sameAsHarfBuzz(f, got, expected[i])
		if same {
			continue
		}
		differing++
		if differing <= 20 {
			t.Errorf("%s  with %s\n  %s\n  forme     %s\n  harfbuzz %s",
				describeRunes(text), specs[i], why,
				describeGlyphs(got), describeExpected(f, expected[i]))
		}
	}
	if differing > 20 {
		t.Errorf("... and %d more", differing-20)
	}
	t.Logf("%d of %d agree (harfbuzz %s)", len(corpus)-differing, len(corpus),
		header["harfbuzz"])
}

// TestTheFeatureOracleHasTeeth.
//
// An oracle that cannot fail is decoration, and this one has two ways of not
// failing that a passing run cannot tell apart: a corpus where no spec asks for
// anything, and a comparison that ignores the features it was handed. Both are
// planted here rather than trusted.
func TestTheFeatureOracleHasTeeth(t *testing.T) {
	specs, corpus := readFeatureCorpus(t, "features.txt")
	expected, _ := readFeatureExpectations(t, "features.expected.txt")
	f, err := Load(notoSansBytes(t))
	if err != nil {
		t.Fatalf("loading the font: %v", err)
	}

	// The corpus really does ask for things, and really does ask for more than
	// one at a time: a file of "-" would pass against any implementation of
	// this whole family.
	var asked, several int
	for _, spec := range specs {
		want := featuresOfSpec(t, spec)
		switch n := len(want.adds()); {
		case n > 1:
			several++
			asked++
		case n == 1:
			asked++
		}
	}
	if asked < len(specs)/2 {
		t.Errorf("%d of %d cases ask a face for anything at all", asked, len(specs))
	}
	if several < 100 {
		t.Errorf("only %d cases ask for more than one feature, and the order "+
			"between two is what this file is for", several)
	}

	// And shaping *without* the features disagrees with the expectations, in a
	// large share of the corpus. If it did not, the comparison above would pass
	// whether or not the features reached the shaper at all.
	var wouldDiffer int
	for i, text := range corpus {
		if len(featuresOfSpec(t, specs[i]).adds()) == 0 {
			continue
		}
		plain, _ := f.ShapeGlyphsInContext(text, "", "", Features{})
		if same, _ := sameAsHarfBuzz(f, plain, expected[i]); !same {
			wouldDiffer++
		}
	}
	if wouldDiffer < 200 {
		t.Errorf("only %d cases come out differently when the features are "+
			"dropped; the rest of the corpus cannot tell an engine that "+
			"applies them from one that does not", wouldDiffer)
	}
}

// featuresOfSpec reads a corpus line's spec into the value the shaper is handed.
//
// It names the CSS values and lets this package say which features they are,
// which is the half of the chain under test. The generator names the same values
// and says for itself which features they are; that is the other half, and where
// the two differ the test fails.
func featuresOfSpec(t *testing.T, spec string) Features {
	t.Helper()
	if spec == "-" {
		return Features{}
	}
	var out Features
	for _, group := range splitFeatureSpec(spec) {
		prop, values, ok := strings.Cut(group, "=")
		if !ok {
			t.Fatalf("the spec %q has a group with no property in it: %q", spec, group)
		}
		for _, value := range strings.Fields(values) {
			switch prop {
			case "caps":
				out.Caps = capsOfName(t, value)
			case "numeric":
				out.Numeric |= numericOfName(t, value)
			case "east-asian":
				out.EastAsian |= eastAsianOfName(t, value)
			case "position":
				out.Position = positionOfName(t, value)
			default:
				t.Fatalf("the spec %q names a property this test does not know: %q",
					spec, prop)
			}
		}
	}
	return out
}

// splitFeatureSpec cuts a spec into its groups.
//
// The separator is a space before a "<prop>=", which is what lets a value hold
// spaces: "numeric=ordinal slashed-zero" is one group naming two values, and
// "caps=small-caps position=super" is two groups naming one each. The generator
// splits it the same way and the two have to agree, which is what the first case
// of the corpus with a two-value group checks.
func splitFeatureSpec(spec string) []string {
	var out []string
	var cur []string
	for _, word := range strings.Fields(spec) {
		if strings.Contains(word, "=") && len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
		cur = append(cur, word)
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}

func capsOfName(t *testing.T, name string) Caps {
	t.Helper()
	switch name {
	case "small-caps":
		return CapsSmall
	case "all-small-caps":
		return CapsAllSmall
	case "petite-caps":
		return CapsPetite
	case "all-petite-caps":
		return CapsAllPetite
	case "unicase":
		return CapsUnicase
	case "titling-caps":
		return CapsTitling
	}
	t.Fatalf("font-variant-caps has no value %q", name)
	return CapsNormal
}

func numericOfName(t *testing.T, name string) Numeric {
	t.Helper()
	switch name {
	case "lining-nums":
		return NumericLining
	case "oldstyle-nums":
		return NumericOldstyle
	case "proportional-nums":
		return NumericProportional
	case "tabular-nums":
		return NumericTabular
	case "diagonal-fractions":
		return NumericDiagonalFractions
	case "stacked-fractions":
		return NumericStackedFractions
	case "ordinal":
		return NumericOrdinal
	case "slashed-zero":
		return NumericSlashedZero
	}
	t.Fatalf("font-variant-numeric has no value %q", name)
	return 0
}

func eastAsianOfName(t *testing.T, name string) EastAsian {
	t.Helper()
	switch name {
	case "jis78":
		return EastAsianJis78
	case "jis83":
		return EastAsianJis83
	case "jis90":
		return EastAsianJis90
	case "jis04":
		return EastAsianJis04
	case "simplified":
		return EastAsianSimplified
	case "traditional":
		return EastAsianTraditional
	case "full-width":
		return EastAsianFullWidth
	case "proportional-width":
		return EastAsianProportionalWidth
	case "ruby":
		return EastAsianRuby
	}
	t.Fatalf("font-variant-east-asian has no value %q", name)
	return 0
}

func positionOfName(t *testing.T, name string) Position {
	t.Helper()
	switch name {
	case "sub":
		return PositionSub
	case "super":
		return PositionSuper
	}
	t.Fatalf("font-variant-position has no value %q", name)
	return PositionNormal
}

// readFeatureCorpus reads the corpus, which is "<spec>\t<text>" a line with
// comments at the top.
//
// A reader of its own rather than readNonEmptyLines, because this corpus carries
// two columns and a header and that one carries neither.
func readFeatureCorpus(t *testing.T, name string) (specs, texts []string) {
	t.Helper()
	path := filepath.Join(harfbuzzDir, name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 0; sc.Scan(); line++ {
		text := sc.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		spec, body, ok := strings.Cut(text, "\t")
		if !ok {
			t.Fatalf("%s:%d: no tab in %q; a case is \"<spec>\\t<text>\"",
				path, line+1, text)
		}
		specs, texts = append(specs, spec), append(texts, body)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return specs, texts
}

// readFeatureExpectations reads what HarfBuzz produced, in the format shape.py
// and shapefeatures.py share.
func readFeatureExpectations(t *testing.T, name string) ([][]hbGlyph, map[string]string) {
	t.Helper()
	path := filepath.Join(harfbuzzDir, name)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbshaping` to generate it.", path, err)
	}
	defer file.Close()
	header := map[string]string{}
	var out [][]hbGlyph
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 0; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			if key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(text, "#")), " "); ok {
				header[key] = value
			}
			continue
		}
		glyphs, err := parseExpectedGlyphs(text)
		if err != nil {
			t.Fatalf("%s:%d: %v", path, line+1, err)
		}
		out = append(out, glyphs)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if header["font-sha256"] == "" {
		t.Fatalf("%s has no font-sha256 line, so there is nothing tying it to a font", path)
	}
	return out, header
}
