# forme

A rendering engine for paged media, in Go. HTML and CSS go in; what comes out is
a list of things to draw on a sheet of paper — with the typography underneath it,
down to the glyph.

A *forme* is the assembled type locked in a chase, ready to print. This is
everything up to that point, and it stops there on purpose: nothing here writes a
PDF or a PNG. A backend takes the ops and puts them somewhere, and everything
above that line is the same whichever it is.

```go
import "github.com/mgilbir/forme/layout"

out := layout.Compose(layout.Input{
    HTML: "<h1>Invoice</h1><p>…</p>",
    CSS:  []layout.Stylesheet{{Source: "h1 { font: 24pt serif }"}},
}, layout.Options{})

// The verdict first: a refused document is one the caller was told not to
// render, and the findings say why.
for _, f := range out.Findings {
    log.Printf("%s: %s", f.Rule, f.Message)
}
if out.Refused {
    return
}

// The ops are in content-box coordinates at the document's natural size, so
// the two numbers beside them are the backend's to apply: Scale is what the
// page had to be shrunk by to fit, and the page's margins are the offset from
// the corner of the paper to the origin the ops are measured from.
origin := layout.Point{X: out.Page.Margin.Left, Y: out.Page.Margin.Top}
backend.Begin(out.Page.Width, out.Page.Height, origin, out.Scale)

for _, op := range out.Ops {
    switch op := op.(type) {
    case layout.DrawText:  // op.Text, op.Face, op.Size, op.At, op.RTL …
    case layout.FillRect:  // op.Rect, op.Color
    case layout.DrawImage: // op.Image, op.Rect
    case layout.TileImage: // op.Image, op.Clip, op.Tile — a repeated background
    }
}
```

`Compose` is the whole of it: build the box tree, lay it out on the sheet, decide
the scale, check that what came out is worth producing, and paint it. What it
reports beside the ops is as much the point as the ops are — a page that had to
be shrunk past legibility, a run of text pushed outside its box, a property the
stylesheet used that nothing implements. Print has no scrollbar and no reflow, so
a page that is quietly wrong stays wrong.

`Refused` is the authority and not a summary of `Findings`: a rule counts the
moment it fires, before the list deduplicates and before its bound cuts it, so a
document refused by its six-hundredth finding is refused with that finding
nowhere in the list. `Truncated` says the list is some of them rather than all.

The same code, minus the imaginary backend, is `layout.Example` in
`layout/example_test.go` — compiled, so this cannot go stale again.

## Packages

	forme/layout       boxes, floats, tables, lines, and the paint ops
	forme/paragraph    a paragraph into lines: breaking, ordering, stacking
	forme/style        the cascade: which declaration wins, and what it computed to
	forme/css          CSS syntax: tokens, component values, selectors
	forme/html         the parser, and the document tree it builds
	forme/shape        the shaping engine: what glyph goes where
	forme/font         the font formats underneath it: sfnt, CFF, glyph names
	forme/bidi         the Unicode bidirectional algorithm, UAX #9
	forme/segment      grapheme cluster boundaries, UAX #29
	forme/fonts/notosans   a face to shape with, embedded, under the OFL
	forme/fonttest     synthetic faces, so a test can build the font it needs

The root is deliberately empty. Each layer is usable without the ones above it:
`shape` needs no document, `paragraph` needs no box tree, and `css` will parse a
stylesheet for anything that asks.

`shape` embeds no font. Two megabytes in the package every caller imports, for a
face most of them will not use, is a cost paid by everyone to serve a few — so the
font is a package of its own and is paid for by importing it:

```go
face, err := notosans.Face()
```

## What it does

**Layout.** CSS 2.1's visual formatting model: the block and inline models,
floats and clearance, margin collapsing, absolute and relative positioning,
tables including the collapsing border model, lists and counters, generated
content, backgrounds and borders, overflow and clipping, and the stacking order
of Appendix E. From CSS Text: white space processing, word and line breaking,
`text-wrap: balance`, tab stops. From CSS Overflow: `line-clamp`.

**Boxes beyond CSS 2.1**, each a deliberately narrow slice: flexbox
(`display: flex`), grid (`display: grid`), multiple columns (`column-count`,
`column-width`) and the vertical writing modes (`writing-mode`,
`text-orientation`). Narrow on purpose, and each says so out loud — a container
written with something its slice does not arrange is laid out as it would have
been without the feature, and the reason is reported. A box arranged that should
not have been is a page that is quietly wrong; a box refused is a page with a
finding attached to it.

What it does *not* do is fragment across pages. A document is laid out on one
sheet and scaled to fit it (`Page.MinScale`), and the only fragmentation
anywhere is multicol's, within a single box.

**Paragraphs.** Where a line may break and where it does, what order the runs on
it are drawn in, and how tall it turns out — stated over text, measured widths
and Unicode, with no box tree anywhere near it.

**Shaping** for the scripts that need it: the Indic model (Devanagari and its
relatives), Khmer, Myanmar, and the Universal Shaping Engine, which covers some
seventy more — Javanese, Balinese, Tibetan, Buginese, Tai Tham and the rest — from
a table derived from Unicode's own properties rather than from per-script
knowledge. Cursive joining for Arabic and its relatives, and the mark ordering of
UTR #53. OpenType layout: GSUB 1–6 and GPOS 1–8, mark attachment, cursive
attachment, contextual and chained-contextual rules, mark filtering sets.

**Fonts.** sfnt and CFF, variable fonts instanced at a named or arbitrary point in
their design space, subsetting, and the metrics a layout engine has to ask for —
including what the fourteen standard PDF faces state, which is not the same
question.

Glyphs come back in **visual order** — the order a pen draws them, left to right —
whatever scripts the string mixes, so a caller can draw them as they are.

## How it is known to be right

Against other people's test suites, because a suite this repository wrote is a
record of what it thought of.

| | |
|---|---|
| **CSS Working Group reftests** | 6,253 documents rendered and compared against their references — **5,961 pass with nothing unsupported reported in either document** |
| **Unicode's bidi conformance** | 861,948 cases across `BidiTest.txt` and `BidiCharacterTest.txt`, no failures |
| **Unicode's grapheme boundaries** | all 766 cases of `GraphemeBreakTest.txt` |
| **Unicode's normalisation forms** | all 20,034 cases of `NormalizationTest.txt`, both NFC invariants |
| **HarfBuzz**, over six fonts | 20,623 strings, two deliberate differences |
| **The CSS Syntax suite** | 229 cases from the suite `css-parsing-tests` publishes, with 13 more deliberately excused and each excuse named |

The reftest number is a **ratchet**: it may never be lowered to make a red test
green, so a drop is a layout regression and every failing name is printed — all
of them, which the run checks against its own count. A
*rise* fails the test too, and asks for the constant to be raised in the same
commit — a number that only tightens when somebody remembers to look is not a
ratchet, and this one fell about fifteen hundred passes behind before that was
true. It is also counted honestly — a document only counts once *nothing* in
either half of the comparison raised an unsupported finding, because two pages
agreeing about a feature neither implements is not evidence.

The two HarfBuzz differences are cases where HarfBuzz is the one out of step, each
settled by asking CoreText as a third opinion rather than by argument. They are
listed with their reasons in `shape/harfbuzz_test.go` and pinned in the corpora,
so a difference that stops being deliberate fails the test.

Beyond the suites: 20 fuzz targets, fifteen of them scheduled weekly, a
differential fuzzer against HarfBuzz that generates text rather than listing it,
and a CoreText harness for the questions two implementations cannot settle
between them.

	make test          # gofmt, vet, and the tests that need nothing fetched
	make race          # the same, under the race detector
	make test-wpt      # fetches the CSS WG reftests and runs the ratchet
	make test-bidi     # fetches Unicode's bidi conformance suites
	make test-grapheme # and its grapheme boundary cases
	make test-normalization # and its normalisation forms
	make test-css      # the CSS Syntax suite
	make hbfuzz        # differential fuzzing; needs python and uharfbuzz
	make test-difffuzz # that fuzzer's classifier, which needs only python
	make wpt-breakdown # where the reftests that are not clean actually are

The corpora are fetched rather than vendored — the reftests alone are eighty
megabytes of somebody else's repository — and everything fetched is gitignored.
The HarfBuzz comparison is the exception: its expectations are checked in, so it
runs under `make test` with nothing but a Go toolchain.

CI runs the lot on every push — the gate, the four fetched suites and the race
detector — and fuzzes weekly. Only `hbfuzz` is left out, because it needs a
Python environment and HarfBuzz itself.

## Generated tables

Everything derived from Unicode is generated, never typed: the script ranges, the
Indic categories, the joining types, the canonical equivalences, the ignorable
set, the glyph-name list, the grapheme break properties, the named colours, the
HTML entities, and the Universal Shaping Engine's category table. `cmd/gen*` are
those generators and each says what it derives from.

## Licence

The code is under the licence in `LICENSE`. The fonts under `fonts/notosans/` and
`testdata/harfbuzz/fonts/` are Google's Noto builds under the SIL Open Font
License 1.1, with their notices beside them; they are test data and shipping this
module does not embed them in anything.
