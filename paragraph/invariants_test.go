package paragraph

import (
	"strings"
	"testing"
	"unicode"

	"github.com/mgilbir/forme/bidi"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What has to be true of every paragraph, whatever is in it.
//
// The tests beside this file pin particular answers: this text, this width, this
// many lines. Those are worth having and they are also brittle in a specific way
// — each one is true of one input, so a change that is wrong for a thousand
// inputs and right for the dozen written down here passes.
//
// These are the other kind. Each states something that must hold for *any*
// paragraph, and each is run over a corpus chosen to be awkward rather than
// representative. None of them says how the breaking works; they say what it
// must not be caught doing, which is what makes them survive a rewrite of the
// thing they watch.

// texts is the corpus. Each one is here because it is a different shape of
// problem, not because it is a different sentence.
var texts = []struct {
	name string
	text string
}{
	{"empty", ""},
	{"one word", "hello"},
	{"ordinary prose", "the quick brown fox jumps over the lazy dog"},
	{"one word longer than any line", "supercalifragilisticexpialidocious"},
	{"a long word among short ones", "a b supercalifragilisticexpialidocious c d"},
	{"runs of spaces", "a    b     c"},
	{"leading and trailing space", "   padded   "},
	{"nothing but spaces", "     "},
	{"newlines", "first\nsecond\nthird"},
	{"tabs", "a\tb\tc"},
	// Scripts that break by different rules: CJK breaks between characters,
	// Hebrew reads right to left, Devanagari has clusters that must not be cut,
	// and the combining sequence is a single grapheme spelled in two runes.
	{"CJK", "日本語のテキストです"},
	{"CJK and Latin", "日本語 and English 混在"},
	{"Hebrew", "אב גד הו"},
	{"Devanagari", "क्षि हिन्दी"},
	{"combining marks", "áêĩ ōu̅"},
	{"Hangul", "각갂 갃"},
	{"emoji with a zero-width joiner", "a \U0001f468‍\U0001f469‍\U0001f466 b"},
	{"non-breaking space", "a b c"},
	{"soft hyphen", "super­cali­fragilistic"},
	{"a single very long unbroken run", strings.Repeat("x", 200)},
	{"punctuation only", "!@#$%^&*()"},
}

// widths is the set of measures each text is broken at: too narrow for anything,
// narrow enough to break most things, comfortable, and wider than any of the
// texts. The pathological ones are the point — a breaker that is correct at 400px
// and loops at 1px is not correct.
var widths = []float64{0, 1, 7, 30, 100, 400, 10000}

// whiteSpaces are the values of white-space-collapse, plus the one case that
// comes from the other longhand.
//
// The names are the longhand's own rather than the shorthand's, because
// WhiteSpaceOf reads white-space-collapse and nothing else: whether a line may
// wrap at all is text-wrap's business, and every value below wraps. The last
// entry is the case that does not, built by hand because no collapse value
// produces it — and it is the case that turns a break opportunity into one the
// line is not allowed to take.
var whiteSpaces = []struct {
	name string
	ws   WhiteSpace
}{
	{"collapse", WhiteSpaceOf("collapse")},
	{"preserve", WhiteSpaceOf("preserve")},
	{"preserve-breaks", WhiteSpaceOf("preserve-breaks")},
	{"break-spaces", WhiteSpaceOf("break-spaces")},
	{"nowrap", noWrap(WhiteSpaceOf("collapse"))},
}

func noWrap(w WhiteSpace) WhiteSpace {
	w.Wrap = false
	return w
}

// itemsOf builds the items a run of text flattens to, the way a layout engine
// does it: cut the text at the opportunities the values allow, then measure each
// piece in the face it will be set in.
//
// It is deliberately the same shape as the engine's own construction rather than
// a convenient simplification, because a property proved about items no caller
// would build is a property about nothing.
func itemsOf(t *testing.T, br *Breaker, face *shape.Face, text string,
	ws WhiteSpace, ow OverflowWrap) []Item {

	t.Helper()
	size := u(size20)
	pieces, _ := SplitAtBreaks(text, ws, WordBreak{}, LineBreak{})
	out := make([]Item, 0, len(pieces))
	afterCollapsible := true
	for _, p := range pieces {
		if p.Segment {
			out = append(out, Item{Face: face, Size: size, Forced: true})
			afterCollapsible = true
			continue
		}
		if p.Collapsible && afterCollapsible {
			continue
		}
		it := Item{
			Text: p.Text, Face: face, Size: size,
			BreakBefore: p.BreakBefore,
			Space:       p.Space, Collapsible: p.Collapsible, TrimAtEnd: p.TrimAtEnd,
			Tab:       p.Tab,
			Hangs:     p.Space && !p.Collapsible && !ws.BreakSpaces && (ws.Collapse || ws.Wrap),
			HangsHard: p.Space && !p.Collapsible && ws.Collapse,
			NoWrap:    !ws.Wrap,
			BreakWord: ow.BreakWord,
			Anywhere:  ow.Anywhere,
		}
		if !p.Tab {
			it.Width = br.MeasureSpaced(face, p.Text, size, TextSpacing{})
		}
		out = append(out, it)
		afterCollapsible = p.Collapsible
	}
	return out
}

// maxLines is the most lines a correct breaker can produce from a set of items.
//
// One per item is the obvious bound and it is wrong: overflow-wrap cuts inside a
// word, so a single item of n characters can legitimately become n lines. The
// bound is therefore a character apiece, plus an item apiece for the ones that
// carry no text — a forced break, a float marker — plus one for a break at the
// very end.
func maxLines(items []Item) int {
	n := len(items) + 1
	for _, it := range items {
		n += len([]rune(it.Text))
	}
	return n
}

// brokenLines runs the breaker to exhaustion and returns the lines.
//
// The bound on the loop is the invariant about progress, checked here rather than
// in a test of its own because a breaker that fails to advance hangs every other
// test in this file and a hang reports nothing. len(items)+2 lines is generous:
// the most a correct breaker can produce is one line per item, plus one for a
// trailing forced break.
func brokenLines(t *testing.T, br *Breaker, items []Item, width float64) [][]Item {
	t.Helper()
	var lines [][]Item
	i, iByte := 0, 0
	for i < len(items) {
		if len(lines) > maxLines(items) {
			t.Fatalf("%d items produced %d lines at %gpx; the breaker is not making "+
				"progress", len(items), len(lines), width)
		}
		line, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, u(width), 0)
		if next < i || (next == i && nextByte <= iByte) {
			t.Fatalf("the cursor went from item %d byte %d to item %d byte %d at "+
				"%gpx: it must always move forward", i, iByte, next, nextByte, width)
		}
		lines = append(lines, line)
		i, iByte = next, nextByte
	}
	return lines
}

// visible is the text with everything the breaking is entitled to remove taken
// out, and nothing else.
//
// Two things and no more. White space, because §4.1.2 collapses it, hangs it and
// trims it at line edges — that is the rule, not a liberty. And U+200B ZERO WIDTH
// SPACE, which SplitAtBreaks consumes outright because it "is a break opportunity
// and nothing else": it marks a place a line may end and draws nothing, so
// keeping it would be keeping a character that can never appear.
//
// Everything else stays, deliberately. A soft hyphen, a zero-width joiner and a
// byte order mark are all invisible on the page and none of them is this code's
// to drop — an engine that quietly swallowed the joiner out of an emoji sequence
// would set two people where the author wrote a family, and the point of this
// predicate is that such a loss still fails the tests that use it.
func visible(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cc, r) || r == 0x200B {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// asSegmented is the input as §4.1.1 hands it to the cutting, with the two
// normalisations that section defines already applied.
//
// A carriage return, alone or before a line feed, *is* a segment break and is
// normalised to one — CSS Text §4.1.1 says so outright, and an engine that kept
// the CR would set an invisible character in the middle of a paragraph. A
// zero-width space is consumed under every value, because it marks a place a
// line may end and is nothing else.
//
// Both are applied to the expected side rather than excused in the assertion, so
// what is asserted stays "the pieces spell the same text" — the strongest form
// the statement can honestly take. It is spelled out here in three lines rather
// than borrowed from the package, so that a fault in the package's own
// normalising cannot hide inside the test that watches it.
func asSegmented(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\u200b", "")
}

func lineRunes(line []Item) string {
	var b strings.Builder
	for _, it := range line {
		b.WriteString(it.Text)
	}
	return b.String()
}

// TestBreakingLosesNoVisibleCharacter is the first thing a line breaker must do
// and the last thing anyone checks: put every character somewhere, once, in
// order.
//
// The white space is excluded because CSS Text §4.1.2 explicitly removes some of
// it — a collapsible space at a line edge is dropped, and a preserved one may
// hang past the measure. Everything else is conserved, and a breaker that drops a
// letter, repeats one, or reorders two has produced a page that reads wrongly and
// no test of line *positions* would notice.
func TestBreakingLosesNoVisibleCharacter(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, w := range whiteSpaces {
			for _, width := range widths {
				items := itemsOf(t, br, face, tc.text, w.ws, OverflowWrap{})
				lines := brokenLines(t, br, items, width)
				var got strings.Builder
				for _, line := range lines {
					got.WriteString(lineRunes(line))
				}
				want := visible(strings.Join(itemTexts(items), ""))
				if visible(got.String()) != want {
					t.Errorf("%s at %gpx, white-space %s: the lines read %q, want the "+
						"characters %q in that order — breaking may drop or hang white "+
						"space and may not touch anything else",
						tc.name, width, w.name, visible(got.String()), want)
				}
			}
		}
	}
}

func itemTexts(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

// lineWidth is what a line spends, which excludes anything hanging past its end
// — §4.1.2 puts a hanging space outside the measure "for fit, alignment, or
// justification".
func lineWidth(line []Item) style.Unit {
	var used style.Unit
	for _, it := range line {
		if it.Hangs {
			continue
		}
		used = used.Add(it.Width)
	}
	return used
}

// TestNoLineOverflowsUnlessNothingCouldMove is the half of the greedy contract
// that can be stated without restating the rules.
//
// A line may exceed its measure — a word longer than the box has to go
// somewhere, and CSS Text §5.5 says it overflows rather than being cut. What a
// line may not do is exceed its measure while holding an opportunity it could
// have used: that is text pushed past the edge for no reason, and it is invisible
// in any test that checks where the lines *are* rather than what is on them.
//
// The width counted excludes anything hanging, because §4.1.2 puts a hanging
// space outside the measure "for fit, alignment, or justification" — the space at
// the end of a line is not what makes the line too long.
func TestNoLineOverflowsUnlessNothingCouldMove(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, width := range widths {
			if width == 0 {
				continue // nothing fits, so everything overflows by construction
			}
			items := itemsOf(t, br, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
			for n, line := range brokenLines(t, br, items, width) {
				if lineWidth(line) <= u(width) {
					continue
				}
				// It overflowed. That is avoidable only if breaking at one of the
				// line's own opportunities would have left something that fitted —
				// which is not the same as the line merely holding one. A line
				// whose *first* item is wider than the measure overflows whatever
				// is done with the rest of it, and moving a later word down would
				// shorten nothing.
				var prefix style.Unit
				for k, it := range line {
					if k > 0 && it.BreakBefore && !it.NoWrap && prefix <= u(width) {
						t.Errorf("%s at %gpx: line %d reads %q and spends %gpx, and "+
							"breaking before its item %d (%q) would have left %gpx, "+
							"which fits — the overflow was avoidable",
							tc.name, width, n, lineRunes(line), lineWidth(line).Px(),
							k, it.Text, prefix.Px())
						break
					}
					if !it.Hangs {
						prefix = prefix.Add(it.Width)
					}
				}
			}
		}
	}
}

// TestALineIsAsFullAsItCouldBe is the half of the greedy contract the overflow
// property does not reach: a line that stops early.
//
// It is the harder fault to see. A line that overflows looks wrong; a line that
// ends two words short looks like a narrower paragraph, and every test that
// checks where the lines *are* agrees with it. Nothing in the properties above
// notices — the text is all there, no line overflows, the count is monotonic in
// the measure, and every one of them is satisfied by a breaker that fills each
// line to half the room it was given.
//
// The statement is: whatever the line took, taking one more item would not have
// fitted. It is checked over the items rather than over the returned line,
// because the candidate is a line that was never built — items[from:next+1], the
// line as it stands plus the first thing that went below it, including the space
// between them that the real line hung or trimmed. That space is interior in the
// candidate and takes its full width there, which is exactly why "the line spends
// 84px and the next word is 36px, in 130px" is not the arithmetic that matters.
func TestALineIsAsFullAsItCouldBe(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, w := range whiteSpaces {
			if !w.ws.Wrap {
				continue // one line, however long
			}
			for _, width := range widths {
				items := itemsOf(t, br, face, tc.text, w.ws, OverflowWrap{})
				from, fromByte := 0, 0
				for from < len(items) {
					line, next, nextByte, _, forced := br.BreakOneLine(
						items, from, fromByte, u(width), 0)
					if next >= len(items) || forced || nextByte != 0 || fromByte != 0 {
						// The last line, a break the author asked for, or a line
						// that ends inside a word — none of them is a line that
						// chose where to stop.
						from, fromByte = next, nextByte
						continue
					}
					joining := items[next]
					if !joining.BreakBefore || joining.NoWrap || joining.Space {
						// Nothing could have joined this line, or what would have
						// is a space, whose own width is the thing §4.1.2 lets a
						// line ignore.
						from, fromByte = next, nextByte
						continue
					}
					// The candidate is everything up to the *next* opportunity, not
					// one item. A break opportunity falls before a word and not
					// before the space that follows it, so a word and its trailing
					// space are one indivisible piece of line: under break-spaces,
					// where that space keeps its width, "over the " is 108px and
					// does not fit in 100 although "over the" would.
					end := next + 1
					for end < len(items) && !(items[end].BreakBefore && !items[end].NoWrap) {
						end++
					}
					// Everything in the range counts, and then the hanging tail is
					// taken off again. A space *between* two words on a line is
					// interior and takes its width; only one left at the end hangs,
					// which is what §4.1.2 excludes "for fit". Dropping every
					// hanging item instead of the trailing run would measure
					// "the quick" as 96px, when the space in the middle of it is
					// as real as the letters.
					var candidate style.Unit
					for _, it := range items[from:end] {
						candidate = candidate.Add(it.Width)
					}
					for k := end - 1; k > from && items[k].Hangs; k-- {
						candidate = candidate.Sub(items[k].Width)
					}
					if candidate <= u(width) {
						t.Errorf("%s under %s at %gpx: the line reads %q, and it plus "+
							"everything up to the next opportunity is %gpx, which fits "+
							"— the line stopped early",
							tc.name, w.name, width, lineRunes(line), candidate.Px())
					}
					from, fromByte = next, nextByte
				}
			}
		}
	}
}

// TestAWiderMeasureNeverNeedsMoreLines is the other half, and it is the one that
// catches a line ended early.
//
// Saying "the line should have been fuller" needs the fitting rules restated in
// the test, hanging spaces and all, which makes the test a second implementation
// and a worse one. This says the same thing sideways and states nothing: give a
// paragraph more room and it cannot need more lines. A breaker that ends a line
// early does it at some widths and not others, and the count goes up somewhere.
//
// It is also the property the balancer's search rests on. That search narrows the
// measure looking for a smaller ragged edge and assumes the line count only ever
// rises as it does so; if it did not, the search would settle on noise.
func TestAWiderMeasureNeverNeedsMoreLines(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, tc := range texts {
		for _, w := range whiteSpaces {
			prev, prevWidth := -1, 0.0
			for _, width := range widths {
				items := itemsOf(t, br, face, tc.text, w.ws, OverflowWrap{})
				n := len(brokenLines(t, br, items, width))
				if prev >= 0 && n > prev {
					t.Errorf("%s under white-space %s took %d lines at %gpx and %d at "+
						"the wider %gpx — more room cannot need more lines",
						tc.name, w.name, prev, prevWidth, n, width)
				}
				prev, prevWidth = n, width
			}
		}
	}
}

// TestBreakingIsStableAtTheSameWidth is the property a balancer depends on
// without ever saying so.
//
// text-wrap: balance lays the same paragraph out at a dozen candidate widths and
// compares the results, and it counts lines at one width and then breaks at it
// again expecting the same answer. A breaker whose output depended on anything
// but its arguments — a memo keyed too loosely, a slice written back through —
// would make that search compare two different paragraphs and settle on
// whichever it happened to measure first.
func TestBreakingIsStableAtTheSameWidth(t *testing.T) {
	face := courier(t)
	for _, tc := range texts {
		for _, width := range widths {
			first := NewBreaker(nil)
			second := NewBreaker(nil)
			itemsA := itemsOf(t, first, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
			itemsB := itemsOf(t, second, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})

			want := renderLines(brokenLines(t, first, itemsA, width))
			// The same breaker asked twice, which is what the balancer does.
			again := renderLines(brokenLines(t, first, itemsA, width))
			// And a breaker with a cold memo, which is what the first pass over a
			// document does.
			cold := renderLines(brokenLines(t, second, itemsB, width))

			if again != want {
				t.Errorf("%s at %gpx broke to %q and then to %q — the same breaker gave "+
					"two answers for one paragraph", tc.name, width, want, again)
			}
			if cold != want {
				t.Errorf("%s at %gpx broke to %q with a warm memo and %q with a cold "+
					"one — the memo is changing the answer, not just the speed",
					tc.name, width, want, cold)
			}
		}
	}
}

func renderLines(lines [][]Item) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = lineRunes(l)
	}
	return strings.Join(out, "␤")
}

// resolveBidi gives every item the paragraph and level UAX #9 assigns it, which
// is what LineVisualOrder reads.
//
// Without this the items carry no paragraph, LineVisualOrder answers "logical
// order, nothing to do", and a test of the reordering would be testing that it
// declined to run — which is how the first version of the test below passed
// while proving nothing.
func resolveBidi(items []Item, dir bidi.Direction) []Item {
	var text []rune
	spans := make([][2]int, len(items))
	for i, it := range items {
		spans[i] = [2]int{len(text), len(text) + len([]rune(it.Text))}
		text = append(text, []rune(it.Text)...)
	}
	if len(text) == 0 {
		return items
	}
	para := bidi.Resolve(text, dir)
	levels := para.Levels()
	out := make([]Item, len(items))
	for i, it := range items {
		it.Para, it.BidiStart, it.BidiEnd = para, spans[i][0], spans[i][1]
		if it.BidiStart < len(levels) {
			it.Level = levels[it.BidiStart]
		}
		out[i] = it
	}
	return out
}

// TestTheVisualOrderIsAPermutation is the one thing reordering must never get
// wrong.
//
// UAX #9 rearranges the runs on a line; which arrangement is right is settled
// next door against Unicode's own conformance data, over eight hundred thousand
// cases. What is settled here is cheaper and not covered there: whatever order
// comes back, it holds every run exactly once. An index dropped is a word that
// vanishes from the page and an index repeated is one drawn twice, and neither
// looks like a bidi fault when it is met.
//
// Both base directions are run, because the reordering of a left-to-right
// paragraph with Hebrew in it and a right-to-left one with Latin in it are
// different code paths reaching the same requirement.
func TestTheVisualOrderIsAPermutation(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	for _, dir := range []struct {
		name string
		d    bidi.Direction
	}{{"left to right", bidi.LeftToRight}, {"right to left", bidi.RightToLeft}} {
		for _, tc := range texts {
			for _, width := range widths {
				items := itemsOf(t, br, face, tc.text, WhiteSpaceOf("collapse"), OverflowWrap{})
				items = resolveBidi(items, dir.d)
				for _, line := range brokenLines(t, br, items, width) {
					// A nil order is the documented answer for a line that reads
					// left to right throughout: the reordering is the identity.
					// Taken as the identity, the property is the same one.
					order := LineVisualOrder(line)
					if order == nil {
						order = make([]int, len(line))
						for i := range order {
							order[i] = i
						}
					}
					if len(order) != len(line) {
						t.Fatalf("%s, %s, %gpx: %d runs came back in an order of %d",
							tc.name, dir.name, width, len(line), len(order))
					}
					seen := make([]bool, len(line))
					for _, k := range order {
						if k < 0 || k >= len(line) {
							t.Fatalf("%s, %s, %gpx: the order names run %d of %d",
								tc.name, dir.name, width, k, len(line))
						}
						if seen[k] {
							t.Fatalf("%s, %s, %gpx: run %d appears twice in the visual "+
								"order, so it would be drawn twice",
								tc.name, dir.name, width, k)
						}
						seen[k] = true
					}
				}
			}
		}
	}
}

// TestRightToLeftRunsAreActuallyReordered is what stops the test above from
// being satisfied by a reordering that never reorders.
//
// Every assertion there holds for a function that returns the identity for every
// input, which is a permutation and is wrong for every line of Hebrew. This is
// the other side: given text that must be rearranged, the order that comes back
// is not the one it went in as.
func TestRightToLeftRunsAreActuallyReordered(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	// Two Hebrew words and a space, in a line wide enough to hold them all, so
	// the whole paragraph is one line and the reordering has something to do.
	items := itemsOf(t, br, face, "אב גד", WhiteSpaceOf("collapse"), OverflowWrap{})
	items = resolveBidi(items, bidi.LeftToRight)
	lines := brokenLines(t, br, items, 400)
	if len(lines) != 1 {
		t.Fatalf("the Hebrew took %d lines, want 1", len(lines))
	}
	order := LineVisualOrder(lines[0])
	if order == nil {
		t.Fatal("a line of Hebrew came back in logical order — the reordering did " +
			"not run, so the words would be drawn in the order they were written")
	}
	logical := true
	for i, k := range order {
		if i != k {
			logical = false
		}
	}
	if logical {
		t.Errorf("a line of Hebrew came back as %v, which is the order it went in — "+
			"the words would read backwards", order)
	}
}

// TestCollapsingIsIdempotent is what makes the white space processing safe to
// reach twice.
//
// §4.1.1's collapsing runs once per text node, and the engine that calls it also
// re-lays boxes out — for a balanced paragraph, for a table column, for a float
// that moved. Collapsing that changed its answer on a second pass would make a
// box narrow every time it was measured again, which is a fault that appears
// only in documents that are laid out more than once and is invisible in the
// ones written to test the collapsing.
func TestCollapsingIsIdempotent(t *testing.T) {
	for _, value := range []string{"collapse", "preserve", "preserve-breaks", "break-spaces"} {
		for _, tc := range texts {
			once := CollapseWhitespace(tc.text, value)
			twice := CollapseWhitespace(once, value)
			if once != twice {
				t.Errorf("%s under %q collapsed to %q and then to %q — collapsing must "+
					"reach its answer in one pass", tc.name, value, once, twice)
			}
			if len(once) > len(tc.text) {
				t.Errorf("%s under %q grew from %d bytes to %d — collapsing removes "+
					"white space and adds nothing", tc.name, value, len(tc.text), len(once))
			}
			if visible(once) != visible(tc.text) {
				t.Errorf("%s under %q lost or changed a visible character: %q became %q",
					tc.name, value, visible(tc.text), visible(once))
			}
		}
	}
}

// TestSplittingKeepsTheTextExactly is the invariant the collapsing above relies
// on: cutting text into pieces is a cut and nothing else.
//
// Two things are not "editing" and are allowed for. A segment break becomes a
// piece of its own carrying no text, because it is an instruction rather than
// something to set, and is put back before the comparison. And under a value that
// collapses, a tab or a newline *is* replaced by a space — that is what §4.1.1
// says collapsing does, and the engine normally does it before this is reached.
// So exactness is asserted for the preserving values, and conservation of every
// visible character for all four.
func TestSplittingKeepsTheTextExactly(t *testing.T) {
	for _, w := range whiteSpaces {
		for _, tc := range texts {
			pieces, _ := SplitAtBreaks(tc.text, w.ws, WordBreak{}, LineBreak{})
			var b strings.Builder
			for _, p := range pieces {
				if p.Segment {
					b.WriteString("\n")
					continue
				}
				b.WriteString(p.Text)
			}
			got := b.String()
			if visible(got) != visible(tc.text) {
				t.Errorf("%s under white-space %s: the pieces spell %q, want the visible "+
					"characters of %q — splitting decides where a line may end and must "+
					"not lose or invent one", tc.name, w.name, got, tc.text)
			}
			if !w.ws.Collapse && asSegmented(got) != asSegmented(tc.text) {
				t.Errorf("%s under white-space %s: the pieces spell %q, want %q exactly — "+
					"nothing is collapsed under this value, so nothing may change",
					tc.name, w.name, got, tc.text)
			}
		}
	}
}
