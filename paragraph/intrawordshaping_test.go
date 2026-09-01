package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Text §5.4, shaping across an intra-word break.
//
//	When shaping scripts such as Arabic are allowed to break within words due
//	to hyphenation or [...] the characters must still be shaped as if the word
//	were not broken.
//
// A cut is the one thing in the whole pipeline that makes a boundary where the
// text has none: every other pass reads the items and leaves their text alone.
// So each half has to be told what the other is, or "عائلة" broken by
// overflow-wrap comes out as two words — the letter before the cut takes its
// final form and the one after it takes its initial.

// TestASplitItemKnowsWhatTheOtherHalfIs is the rule over the item.
func TestASplitItemKnowsWhatTheOtherHalfIs(t *testing.T) {
	br := NewBreaker(nil)
	item := Item{Text: "abcdef", Face: courier(t), Size: onePx}
	head, tail := br.SplitItem(item, 3)
	if head.PostContext != "def" {
		t.Errorf("the head's following context is %q, want the tail", head.PostContext)
	}
	if tail.PreContext != "abc" {
		t.Errorf("the tail's preceding context is %q, want the head", tail.PreContext)
	}
	// And neither loses the context the whole item had: what preceded the item
	// still precedes the head, and what followed it still follows the tail.
	if head.PreContext != "" || tail.PostContext != "" {
		t.Errorf("the outer contexts are %q and %q, want neither invented",
			head.PreContext, tail.PostContext)
	}
}

// TestTheOuterContextGoesOnTheOutsideOfEachHalf. The item may already stand
// between two others — §8.1's boundary shaping puts it there — and a cut inside
// it does not move either neighbour.
func TestTheOuterContextGoesOnTheOutsideOfEachHalf(t *testing.T) {
	br := NewBreaker(nil)
	item := Item{Text: "abcdef", PreContext: "XY", PostContext: "ZW",
		Face: courier(t), Size: onePx}
	head, tail := br.SplitItem(item, 3)
	if head.PreContext != "XY" || head.PostContext != "defZW" {
		t.Errorf("the head's contexts are %q and %q, want \"XY\" and \"defZW\"",
			head.PreContext, head.PostContext)
	}
	if tail.PreContext != "XYabc" || tail.PostContext != "ZW" {
		t.Errorf("the tail's contexts are %q and %q, want \"XYabc\" and \"ZW\"",
			tail.PreContext, tail.PostContext)
	}
}

// TestASplitHalfIsMeasuredInTheContextItIsDrawnIn is the half that makes the
// line right rather than merely the letters.
//
// A cursive letter's advance depends on the form it takes, so a head measured
// alone is measured to one width and painted at another — and the fill that
// chose the cut chose it against the wrong number.
func TestASplitHalfIsMeasuredInTheContextItIsDrawnIn(t *testing.T) {
	f := shapedFace(t, "NotoSansArabic-Regular.ttf")
	if !f.HasJoiningForms() {
		t.Skip("this face has no positional forms, so a cut cannot change one")
	}
	const size = 1000
	sz, _ := style.FromPx(size)
	br := NewBreaker(nil)
	const word = "بببب" // four behs, which join throughout

	item := Item{Text: word, Face: f, Size: sz}
	head, tail := br.SplitItem(item, len("بب"))

	want := func(s, before, after string) style.Unit {
		return br.MeasureSpacedInContext(f, s, sz, TextSpacing{}, before, after, true)
	}
	if head.Width != want(head.Text, "", head.PostContext) {
		t.Errorf("the head is %v wide and in its context it is %v",
			head.Width, want(head.Text, "", head.PostContext))
	}
	if tail.Width != want(tail.Text, tail.PreContext, "") {
		t.Errorf("the tail is %v wide and in its context it is %v",
			tail.Width, want(tail.Text, tail.PreContext, ""))
	}
	// And the two halves together are the whole word, which is what "shaped as
	// if the word were not broken" comes to as a measurement.
	whole := br.MeasureSpaced(f, word, sz, TextSpacing{})
	if got := head.Width.Add(tail.Width); got != whole {
		t.Errorf("the two halves are %v and the word is %v; a break inside a word "+
			"does not change how it is shaped", got, whole)
	}
	// The fixture has to be a word whose forms differ, or the equality above
	// holds for a reason that has nothing to do with the context.
	if apart := br.MeasureSpaced(f, head.Text, sz, TextSpacing{}).
		Add(br.MeasureSpaced(f, tail.Text, sz, TextSpacing{})); apart == whole {
		t.Fatalf("this face measures the halves the same joined and apart, so the "+
			"fixture asserts nothing: %v", whole)
	}
}

// onePx is a size the two context tests do not depend on; only the strings
// matter there.
var onePx, _ = style.FromPx(10)
