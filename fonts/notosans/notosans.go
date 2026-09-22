// Package notosans bundles one typeface, Noto Sans, so that a caller can set
// text without supplying a font file.
//
// Every other way of getting a shape.Face needs a font program from somewhere,
// and a backend that embeds what it draws — a PDF writer, which has to carry
// every font a page shows — needs the program as well as the metrics. This is
// one that is always there: Noto Sans, as Google Fonts ships it, under the SIL
// Open Font License. The licence, the copyright notice and the provenance are
// in this directory beside this file, and travel with it as that licence
// requires; see README.md.
//
// # Why the variable font, and why that is not a problem
//
// Google Fonts ships Noto Sans as a variable font, and it is the *only* build
// that carries Devanagari: the narrower per-script upstream this originally
// took has Latin, Greek and Cyrillic alone. A variable font handed on whole
// would be wrong — a PDF reader, for one, is not asked to instance one — but a
// face is subsetted before it is embedded, and subsetting keeps the static
// tables and drops fvar, gvar, avar, HVAR, MVAR and STAT. In a variable font
// the glyf outlines *are* the default instance, so what reaches a document is
// an ordinary static font at the default weight, which for this face is
// Regular: the advances are identical to the separately-published static
// Regular, glyph for glyph.
//
// A document made with it carries no licence obligation of its own. OFL 1.1 is
// explicit that the requirement to stay under the licence "does not apply to
// any document created using the Font Software".
package notosans

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/mgilbir/forme/shape"
)

//go:embed NotoSans-Variable.ttf
var notoSansRegular []byte

//go:embed OFL.txt
var notoSansLicense string

// Face returns the bundled face, to be embedded as a composite font.
//
// This is the one to reach for. A composite font addresses glyphs by index, so
// it can show everything the face covers — Latin, Greek, Cyrillic and
// Devanagari — and it is what shaping needs: ligatures, kerning and contextual
// substitution are all statements about glyph indices, and a simple font cannot
// name them.
//
// Each call returns a new face. A face records the glyphs it was asked to set,
// which is what subsetting is computed from, so sharing one between documents
// would put each document's glyphs into the other's font.
func Face() (*shape.Face, error) {
	notoOnce.Do(func() { notoPrototype, notoErr = shape.Load(notoSansRegular) })
	if notoErr != nil {
		return nil, fmt.Errorf("notosans: the bundled Noto Sans could not be read: %w", notoErr)
	}
	return notoPrototype.Clone(), nil
}

// The parsed prototype, read once and never handed out.
//
// Reading this face cost 16 ms and 9.6 MB on every call, and a program that
// writes documents calls it once per document. Nine tenths of that is reading
// the layout tables — a face this size states some sixty thousand kern pairs —
// and none of it depends on the document: the parsed program, the tables and
// the rules read out of them are the same every time.
//
// What is *not* the same is the set of glyphs the document asked for, which is
// what subsetting is computed from. So the parse is shared and the record of
// use is not; see shape.Face.Clone.
var (
	notoOnce      sync.Once
	notoPrototype *shape.Face
	notoErr       error
)

// Simple returns the bundled face, to be embedded as a simple font: one byte
// per character, WinAnsiEncoding.
//
// It makes a smaller file and a simpler one, and it costs the two things a
// simple font cannot do — anything outside WinAnsi's 224 characters of Latin,
// and any shaping at all, because a one-byte code names nothing in the layout
// tables. Use it for plain Latin text where size matters; use Face otherwise.
func Simple() (*shape.Face, error) {
	simpleOnce.Do(func() {
		simplePrototype, simpleErr = shape.LoadSimple(notoSansRegular)
	})
	if simpleErr != nil {
		return nil, fmt.Errorf("notosans: the bundled Noto Sans could not be read: %w", simpleErr)
	}
	return simplePrototype.Clone(), nil
}

// The simple face's prototype, read once for the reason the composite one is.
//
// This read the two megabytes again on every call, and a program that writes
// documents calls it once per document — so a hundred documents were a hundred
// parses of the same bytes, while the composite face beside it had been shared
// since the day the cost was measured. Nothing about the parse depends on the
// document; what does is the record of which glyphs it used, and Clone is what
// gives each caller its own.
var (
	simpleOnce      sync.Once
	simplePrototype *shape.Face
	simpleErr       error
)

// License is the text of the SIL Open Font License 1.1 as it is distributed
// with the bundled font, including the copyright line.
//
// It is exposed because the licence requires it to travel with the font, and a
// program that embeds the font in something it ships may need to reproduce it —
// in an about box, a credits file, a --licenses flag. Reading it off disk is not
// an option for a single binary, so it is compiled in.
func License() string { return notoSansLicense }

// Regular is the font program's own bytes, for a caller that wants to read it
// itself — a copy, which the caller may do what it likes with.
//
// It used to be the embedded array itself, and that array is the one the
// prototypes above were loaded from and every face they clone reads, and the
// bytes the subsetter copies into a document. A caller that patched a table in
// place changed the font under every face in the process. Two megabytes copied
// per call is the price of that not being possible, and a caller that wants the
// bytes wants them once.
func Regular() []byte { return bytes.Clone(notoSansRegular) }
