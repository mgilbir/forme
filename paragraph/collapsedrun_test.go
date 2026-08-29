package paragraph

import "testing"

// Boundary.Collapsed and the run that a node boundary falls inside.
//
// Phase I is defined over one node, and §4.1.1 is defined over the text of an
// inline formatting context — css-text-4 spells out that "intervening inline box
// boundaries must be ignored". The two are reconciled by telling a node whether
// the run it opens with was already opened, and by the node before it having
// already written the one space the run collapses to.

func TestEndsCollapsedSpaceReadsTheOutput(t *testing.T) {
	sp := WordSpaceTransform{Separator: ordinarySpace}
	ideo := WordSpaceTransform{Separator: ideographicSpace}
	for _, tc := range []struct {
		name      string
		collapsed string
		value     string
		wst       WordSpaceTransform
		want      bool
	}{
		{"a run that collapsed to a space", "a ", "collapse", WordSpaceTransform{}, true},
		{"a word", "ab", "collapse", WordSpaceTransform{}, false},
		{"nothing at all", "", "collapse", WordSpaceTransform{}, false},
		{"the property's own space", "a ", "collapse", sp, true},
		{"the property's ideographic space", "a" + ideographicSpace, "collapse", ideo, true},
		// An ideographic space nobody asked the property for is a character the
		// author wrote. §4.1 counts it among the other space separators, which
		// no rule collapses, so no run is open after it.
		{"an ideographic space the author wrote", "a" + ideographicSpace, "collapse", WordSpaceTransform{}, false},
		// Under "preserve" nothing collapses, so nothing is open.
		{"a preserved space", "a ", "preserve", WordSpaceTransform{}, false},
	} {
		if got := EndsCollapsedSpace(tc.collapsed, tc.value, tc.wst); got != tc.want {
			t.Errorf("%s: EndsCollapsedSpace(%q) = %v, want %v",
				tc.name, tc.collapsed, got, tc.want)
		}
	}
}

func TestARunThatCrossedTheBoundaryAddsNothing(t *testing.T) {
	open := Boundary{Last: ' ', Seen: ' ', Collapsed: true}
	sp := WordSpaceTransform{Separator: ordinarySpace}
	for _, tc := range []struct {
		name string
		text string
		wst  WordSpaceTransform
		want string
	}{
		{"a leading space", " b", WordSpaceTransform{}, "b"},
		{"a leading run", "   b", WordSpaceTransform{}, "b"},
		{"a leading separator", "​", sp, ""},
		{"a leading separator and a word", "​g", sp, "g"},
		// The bit says what the node follows and is spent on the first run, so
		// a second run in the same node is a run of this node's own and
		// collapses to a space like any other.
		{"a run after the one that crossed", "  b c", WordSpaceTransform{}, "b c"},
		{"a separator after the one that crossed", "​b​c", sp, "b c"},
		{"a node that opens with a word", "b c", WordSpaceTransform{}, "b c"},
		{"a run after a word", "b  c", WordSpaceTransform{}, "b c"},
	} {
		got := CollapseWhitespaceAfter(tc.text, "collapse", tc.wst, open, WritingSystemOther)
		if got != tc.want {
			t.Errorf("%s: %q after an open run collapsed to %q, want %q",
				tc.name, tc.text, got, tc.want)
		}
	}
}

func TestAClosedRunIsUntouched(t *testing.T) {
	// The same texts after a node that ended in a letter. The leading space is
	// kept, because the flattening is what collapses it across the boundary and
	// it keeps the break opportunity while doing so.
	shut := Boundary{Last: 'a', Seen: 'a'}
	got := CollapseWhitespaceAfter(" b", "collapse", WordSpaceTransform{}, shut, WritingSystemOther)
	if got != " b" {
		t.Errorf("%q after a letter collapsed to %q, want %q", " b", got, " b")
	}
}

func TestAPreservedBreakSurvivesAnOpenRun(t *testing.T) {
	// pre-line keeps segment breaks and collapses the spaces around them. A
	// break is not a space, so closing the run must not take it.
	open := Boundary{Last: ' ', Seen: ' ', Collapsed: true}
	got := CollapseWhitespaceAfter(" \n b", "preserve-breaks", WordSpaceTransform{},
		open, WritingSystemOther)
	if got != "\nb" {
		t.Errorf("%q after an open run collapsed to %q, want %q under pre-line: "+
			"the spaces go and the break stays", " \n b", got, "\nb")
	}
}

func TestAPreservedBreakBesideASeparatorSurvivesToo(t *testing.T) {
	// The other arm that keeps a break: a virtual word separator in a run that
	// also holds one. CSS Text 4 says not to expand a separator against a forced
	// break — a place a line *may* end beside a place it must — so what is left
	// is the break, and an open run coming into it takes nothing away.
	open := Boundary{Last: ' ', Seen: ' ', Collapsed: true}
	sp := WordSpaceTransform{Separator: ordinarySpace}
	got := CollapseWhitespaceAfter("​\n b", "preserve-breaks", sp, open, WritingSystemOther)
	if got != "\nb" {
		t.Errorf("a separator against a preserved break after an open run "+
			"collapsed to %q, want %q: the separator is not expanded and the "+
			"break is not a space", got, "\nb")
	}
}

func TestFreezesSpaceIsOnlyFullWidth(t *testing.T) {
	for _, tc := range []struct {
		kind TextTransform
		want bool
	}{
		{TransformFullWidth, true},
		{TransformFullWidth | TransformLowercase, true},
		{TransformNone, false},
		{TransformUppercase, false},
		{TransformLowercase, false},
		{TransformCapitalize, false},
	} {
		if got := FreezesSpace(tc.kind); got != tc.want {
			t.Errorf("FreezesSpace(%v) = %v, want %v: U+3000 is what "+
				"\"full-width\" makes of a space and no other value makes of one",
				tc.kind, got, tc.want)
		}
	}
}
