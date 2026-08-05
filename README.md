# forme

Text shaping and font reading in Go: OpenType layout for the complex scripts,
the Unicode bidirectional algorithm, and enough of the font formats to do it.

A *forme* is the assembled type locked in a chase, ready to print. This is the
part that decides which glyphs go where.

```go
import "github.com/mgilbir/forme/shape"

face, err := shape.Load(ttf)
glyphs, missing := face.ShapeGlyphs("नमस्ते")
for _, g := range glyphs {
    // g.GID, g.XAdvance, g.XOffset, g.YOffset, g.Cluster
}
```

Glyphs come back in **visual order** — the order a pen draws them, left to
right — whatever scripts the string mixes, so a caller can draw them as they
are.

## Packages

	forme/shape   the shaping engine: what glyph goes where
	forme/font    the font formats underneath it: sfnt, CFF, glyph names

The root is deliberately empty. Shaping is where this starts rather than what it
is for, and paragraph splitting belongs above it.

## What it does

- **Shaping** for the scripts that need it: the Indic model (Devanagari and its
  relatives), Khmer, Myanmar, and the Universal Shaping Engine, which covers
  some seventy more — Javanese, Balinese, Tibetan, Buginese, Tai Tham and the
  rest — from a table derived from Unicode's own properties rather than from
  per-script knowledge.
- **Cursive joining** for Arabic and its relatives, and the mark ordering of
  UTR #53.
- **OpenType layout**: GSUB 1–6 and GPOS 1–8, mark attachment, cursive
  attachment, contextual and chained-contextual rules, mark filtering sets.
- **The bidirectional algorithm**, UAX #9, run in full against Unicode's own
  conformance suites.
- **Normalisation** as shaping needs it, and **subsetting** of sfnt and CFF.

## How it is known to be right

Against HarfBuzz, over six fonts and 20,515 strings, checked in as expectations
so the comparison runs with nothing but a Go toolchain:

	latin 12475 · arabic 1869 · khmer 2465 · javanese 1110 · balinese 800 · tibetan 1796

Two of those differ on purpose, and both are cases where HarfBuzz is the one out
of step — each settled by asking CoreText as a third opinion rather than by
argument. They are listed with their reasons in `shape/harfbuzz_test.go` and pinned in
the corpora, so a difference that stops being deliberate fails the test.

There is also a differential fuzzer (`testdata/harfbuzz/difffuzz.py`) that
generates text rather than listing it, and a CoreText harness
(`testdata/coretext/`) for the questions two implementations cannot settle
between them.

	make test            # build, vet, and the checked-in comparison
	make test-bidi       # fetches Unicode's conformance suites and runs them
	make hbfuzz          # differential fuzzing; needs python and uharfbuzz

## Generated tables

Everything derived from Unicode is generated, never typed: the script ranges,
the Indic categories, the joining types, the canonical equivalences, the
ignorable set, the glyph-name list, and the Universal Shaping Engine's category
table. `cmd/gen*` are those generators and each says what it derives from.

## Licence

The code is under the licence in `LICENSE`. The fonts under `shape/notosans/` and
`testdata/harfbuzz/fonts/` are Google's Noto builds under the SIL Open Font
License 1.1, with their notices beside them; they are test data and shipping
this module does not embed them in anything.
