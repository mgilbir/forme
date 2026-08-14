package paragraph

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/style"

	"github.com/mgilbir/forme/bidi"
)

// The invariants next door, over text nobody chose.
//
// The corpus in invariants_test.go is a list of things someone thought of, which
// is the limit of what it can catch. This runs the same properties over whatever
// the fuzzer produces — lone surrogates, unassigned code points, a megabyte of
// combining marks, a tab between two halves of a grapheme cluster. Line breaking
// reads every character of its input and asks Unicode about most of them, so the
// space of inputs that reach a different branch is very large and not
// enumerable by hand.
//
// It is a crash-and-invariant fuzzer rather than a differential one: there is no
// second implementation to compare against, so what it checks is that the
// breaking always terminates, always makes progress, never loses a character and
// never overflows a line it could have broken.

func fuzzSeeds() []string {
	out := make([]string, 0, len(texts))
	for _, tc := range texts {
		out = append(out, tc.text)
	}
	return append(out,
		"\x00", "\ufeff", "\u00ad", "\u200b", "\u00a0", "\u3000",
		"a\u0302\u0303b", strings.Repeat("\t", 40),
		strings.Repeat("a ", 500), "\U0001f600\u200d\U0001f600",
	)
}

// FuzzBreakParagraph breaks arbitrary text at an arbitrary measure and holds the
// result to the properties that must survive any input.
func FuzzBreakParagraph(f *testing.F) {
	for _, s := range fuzzSeeds() {
		for _, w := range []uint16{0, 1, 13, 97, 1000} {
			f.Add(s, w, uint8(0))
		}
	}
	f.Fuzz(func(t *testing.T, text string, rawWidth uint16, mode uint8) {
		// A whole megabyte of text is a slow test rather than a different one.
		if len(text) > 4096 {
			t.Skip()
		}
		// Valid UTF-8 is a precondition rather than a case. Text reaches this
		// package from an HTML parser, and the encoding step of that standard has
		// already replaced every malformed sequence with U+FFFD — so a string with
		// a stray 0xD7 in it is not an input this code can meet. Go's own
		// iteration substitutes U+FFFD for one, which is a safe answer and not the
		// same string, and asserting that nothing changes would be asserting
		// against the language rather than against the breaking.
		if !utf8.ValidString(text) {
			t.Skip()
		}
		br := NewBreaker(nil)
		face := courier(t)
		w := whiteSpaces[int(mode)%len(whiteSpaces)]
		ow := OverflowWrap{}
		if mode&0x80 != 0 {
			ow.BreakWord = true
		}
		width := float64(rawWidth) / 4

		items := itemsOf(t, br, face, text, w.ws, ow)

		// Progress and termination. A breaker that stalls hangs the fuzzer, which
		// reports a timeout rather than the input — so the bound is checked here
		// and the input is named.
		var lines [][]Item
		i, iByte := 0, 0
		for i < len(items) {
			if len(lines) > maxLines(items) {
				t.Fatalf("%q at %gpx under %s: %d items produced %d lines; the "+
					"breaking is not making progress",
					text, width, w.name, len(items), len(lines))
			}
			line, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, u(width), 0)
			if next < i || (next == i && nextByte <= iByte) {
				t.Fatalf("%q at %gpx under %s: the cursor went from item %d byte %d "+
					"to item %d byte %d; it must always move forward",
					text, width, w.name, i, iByte, next, nextByte)
			}
			lines = append(lines, line)
			i, iByte = next, nextByte
		}

		// Every visible character, once, in order.
		var got strings.Builder
		for _, line := range lines {
			got.WriteString(lineRunes(line))
		}
		var want strings.Builder
		for _, it := range items {
			want.WriteString(it.Text)
		}
		if visible(got.String()) != visible(want.String()) {
			t.Fatalf("%q at %gpx under %s: the lines read %q, want the characters %q",
				text, width, w.name, visible(got.String()), visible(want.String()))
		}

		// No line overflows where breaking at one of its own opportunities would
		// have left something that fitted. Merely holding an opportunity is not
		// enough: a line whose first item is wider than the measure overflows
		// however it is broken, and an opportunity under a non-wrapping value is
		// one the line is not allowed to take.
		if width > 0 {
			for n, line := range lines {
				if lineWidth(line) <= u(width) {
					continue
				}
				var prefix style.Unit
				for k, it := range line {
					if k > 0 && it.BreakBefore && !it.NoWrap && prefix <= u(width) {
						t.Fatalf("%q at %gpx under %s: line %d spends %gpx, and breaking "+
							"before its item %d would have left %gpx, which fits — the "+
							"overflow was avoidable",
							text, width, w.name, n, lineWidth(line).Px(), k, prefix.Px())
					}
					if !it.Hangs {
						prefix = prefix.Add(it.Width)
					}
				}
			}
		}

		// And the reordering keeps every run.
		resolved := resolveBidi(items, bidi.LeftToRight)
		i, iByte = 0, 0
		for i < len(resolved) {
			line, next, nextByte, _, _ := br.BreakOneLine(resolved, i, iByte, u(width), 0)
			order := LineVisualOrder(line)
			if order != nil {
				if len(order) != len(line) {
					t.Fatalf("%q at %gpx: %d runs came back in an order of %d",
						text, width, len(line), len(order))
				}
				seen := make([]bool, len(line))
				for _, k := range order {
					if k < 0 || k >= len(line) || seen[k] {
						t.Fatalf("%q at %gpx: the visual order %v is not a permutation "+
							"of the line's %d runs", text, width, order, len(line))
					}
					seen[k] = true
				}
			}
			if next == i && nextByte == iByte {
				break
			}
			i, iByte = next, nextByte
		}
	})
}

// FuzzSplitAtBreaks holds the cutting to its own invariant: it decides where a
// line may end and does not edit the text.
func FuzzSplitAtBreaks(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, text string, mode uint8) {
		if len(text) > 4096 {
			t.Skip()
		}
		// Valid UTF-8 is a precondition rather than a case. Text reaches this
		// package from an HTML parser, and the encoding step of that standard has
		// already replaced every malformed sequence with U+FFFD — so a string with
		// a stray 0xD7 in it is not an input this code can meet. Go's own
		// iteration substitutes U+FFFD for one, which is a safe answer and not the
		// same string, and asserting that nothing changes would be asserting
		// against the language rather than against the breaking.
		if !utf8.ValidString(text) {
			t.Skip()
		}
		w := whiteSpaces[int(mode)%len(whiteSpaces)]
		pieces, _ := SplitAtBreaks(text, w.ws, WordBreak{}, LineBreak{})

		var b strings.Builder
		for _, p := range pieces {
			if p.Segment {
				b.WriteString("\n")
				continue
			}
			b.WriteString(p.Text)
		}
		if visible(b.String()) != visible(text) {
			t.Fatalf("%q under %s: the pieces spell %q, want the visible characters "+
				"of the input", text, w.name, b.String())
		}
		if !w.ws.Collapse && asSegmented(b.String()) != asSegmented(text) {
			t.Fatalf("%q under %s: the pieces spell %q and nothing is collapsed under "+
				"this value, so nothing may change", text, w.name, b.String())
		}
		// A piece carrying no text is a segment break and nothing else: an empty
		// piece that is not one would be a break opportunity offered at a place
		// with no character, which the breaking would take and make no progress on.
		for i, p := range pieces {
			if p.Text == "" && !p.Segment {
				t.Fatalf("%q under %s: piece %d holds no text and is not a segment "+
					"break", text, w.name, i)
			}
		}
	})
}

// FuzzBalance holds the three searches to their contracts over arbitrary text and
// arbitrary bands.
//
// The tests next door run them over a corpus and a handful of band shapes, which
// is a list of things someone thought of. The searches are the part of this
// package most likely to be wrong on an input nobody imagined: each is a hunt
// over a space of arrangements, each has a bound it gives up at, and each returns
// a sentinel for "nothing to choose" that a caller then has to handle. Those are
// the edges a corpus misses.
func FuzzBalance(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s, uint16(400), uint16(60), uint16(200), uint8(3))
	}
	f.Fuzz(func(t *testing.T, text string, rawWidth, rawA, rawB uint16, rawLines uint8) {
		if len(text) > 2048 || !utf8.ValidString(text) {
			t.Skip()
		}
		br := NewBreaker(nil)
		face := courier(t)
		width := u(float64(rawWidth) / 4)
		bands := []style.Unit{u(float64(rawA) / 4), u(float64(rawB) / 4)}

		items := itemsOf(t, br, face, text, WhiteSpaceOf("collapse"), OverflowWrap{})

		// The width search: a cap that never widens the box, and never costs a
		// line against the same bands it was searched in.
		if cap := br.BalanceWidthInBands(items, bands, width, 0); cap != style.MaxUnit {
			if cap > width {
				t.Fatalf("%q at %gpx, bands %v: balanced to %gpx, wider than the box",
					text, width.Px(), bands, cap.Px())
			}
			full := br.countLinesInBands(items, bands, width, 0, MaxBalanceLines+1)
			capped := br.countLinesInBands(items, bands, cap, 0, MaxBalanceLines+2)
			if capped > full {
				t.Fatalf("%q at %gpx, bands %v: the box takes %d lines and the balanced "+
					"%gpx takes %d", text, width.Px(), bands, full, cap.Px(), capped)
			}
		}

		// A band narrower than the probe is the line's room, so the banded count
		// and the plain one are the same paragraph.
		narrow := style.Min(bands[0], bands[1])
		if narrow > 0 && narrow < width {
			banded := br.countLinesInBands(items, []style.Unit{narrow}, width, 0, 99)
			plain := br.countLines(items, narrow, 0, 99)
			if banded != plain {
				t.Fatalf("%q: %d lines in a uniform %gpx band probed at %gpx, and %d at a "+
					"plain %gpx", text, banded, narrow.Px(), width.Px(), plain, narrow.Px())
			}
		}

		// The scored search. Its "lines" argument is a precondition rather than a
		// request: layout hands it len(bands), which is the number of lines the
		// greedy layout came to, and asks for a tidier arrangement into that many.
		//
		// Note that no width reaches this search at all — it works inside the
		// bands, which are the rooms the lines really had. So the count has to be
		// derived the same way, by breaking greedily in those bands with nothing
		// capping them. Deriving it at some probe width instead asks for an
		// arrangement into a number of lines the bands cannot make, which is not a
		// case layout produces and not one the answer is defined for.
		lines := br.countLinesInBands(items, bands, style.MaxUnit, 0, MaxBalanceLines+1)
		if caps := br.BalanceScoredCaps(items, bands, 0, lines); caps != nil {
			if len(caps) != lines {
				t.Fatalf("%q: asked for %d lines and got %d caps", text, lines, len(caps))
			}
			got, _ := linesWithCaps(t, br, items, bands, caps, 0)
			if len(got) != lines {
				t.Fatalf("%q, bands %v: the caps for %d lines break to %d",
					text, bands, lines, len(got))
			}
		}

		// The clamped search: never wider than the box, and never showing less of
		// the text than the box did.
		ellipsis := u(12)
		clampLines := int(rawLines%6) + 1
		clamped := br.BalanceClampedWidth(items, width, 0, ellipsis, clampLines)
		if clamped > width {
			t.Fatalf("%q at %gpx: the clamped balance is %gpx, wider than the box",
				text, width.Px(), clamped.Px())
		}
		wantI, wantByte := br.clampedReach(items, width, 0, ellipsis, clampLines)
		gotI, gotByte := br.clampedReach(items, clamped, 0, ellipsis, clampLines)
		if gotI < wantI || (gotI == wantI && gotByte < wantByte) {
			t.Fatalf("%q at %gpx, %d lines: the box reached item %d byte %d and the "+
				"balanced %gpx reaches only %d/%d",
				text, width.Px(), clampLines, wantI, wantByte, clamped.Px(), gotI, gotByte)
		}
		if gotI < 0 || gotI > len(items) {
			t.Fatalf("%q: the clamped reach is item %d of %d", text, gotI, len(items))
		}
	})
}
