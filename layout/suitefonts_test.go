package layout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// The font library the suite harness lends the engine, and the one thing it
// deliberately does not contain.
//
// It does not contain Ahem. A quarter of the suite is written in that face, and
// every document that uses it asks for it the way a document asks for anything —
// `<link rel="stylesheet" href="/fonts/ahem.css">`, whose whole content is an
// @font-face pointing at /fonts/Ahem.ttf. The engine loads that itself now, from
// the document, through the same resolver an <img> goes through. An earlier
// version of this file handed the face over by filename because the document's
// own request for it could not be honoured; honouring it is more faithful than
// the shortcut was, and the shortcut is gone.
//
// What is left here is a real font library: the pan-Unicode Noto faces, which
// are what a *caller* supplies. They are not a shortcut for anything the
// documents ask for — no suite document names Noto — they are coverage for the
// scripts the fourteen standard PDF faces do not have, which is exactly the job
// FallbackFontSet exists for.
//
// They are loaded from the checkout rather than vendored: forme ships no font
// bytes at all. See the note beside NOTO_DIR in the Makefile.

// suiteFonts is a FontSet that falls back to Noto for the scripts the standard
// faces do not have, and defers to those for everything else.
type suiteFonts struct {
	fallback []*shape.Face
	standard FontSet

	// named answers a family the document asks for by name, which the standard
	// fourteen have no face for. See ipaFamilies.
	named map[string]*shape.Face
}

// FaceFor implements FallbackFontSet: the first Noto face that can set the whole
// of the text, or nothing.
//
// Weight and style are ignored. Only the regular faces are fetched, and a
// document whose Hebrew is meant to be bold is far better served by upright
// Hebrew than by the space the standard faces would put there — the substitution
// is reported either way, so nothing is being hidden.
func (w suiteFonts) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	for _, f := range w.fallback {
		if _, missing := f.ShapeGlyphs(text); missing == 0 {
			return f, true
		}
	}
	return nil, false
}

func (w suiteFonts) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if f, ok := w.named[strings.ToLower(strings.TrimSpace(family))]; ok {
		return f, true
	}
	return w.standard.Face(family, bold, italic)
}

var (
	wptFontsOnce sync.Once
	wptFontSet   FontSet
)

// fontSetForWPT is built once: the Noto faces are megabytes apiece and parsing
// them per document, twice per reftest, over five thousand of them, would cost
// more than the whole run.
//
// Sharing one face between documents is safe *here* and would not be in a
// caller that produced PDFs, because a face records the glyphs it was asked to
// show and that record decides what each document embeds. This harness compares
// display lists and writes no PDF.
func fontSetForWPT() FontSet {
	wptFontsOnce.Do(func() {
		wptFontSet = suiteFonts{
			standard: StandardFonts(),
			fallback: notoFaces(),
			named:    ipaFaces(),
		}
	})
	return wptFontSet
}

// notoEnv names the directory `make noto-fonts` fetches into. It is fonttest's
// so that the variable a message tells a reader to set cannot drift from the
// one the helpers read.
const notoEnv = fonttest.NotoEnv

// fallbackFacesInUse returns the faces the harness is actually lending the
// engine, so a report can say how many there were.
//
// None is a legitimate state — everything still runs, see notoFaces — but it is
// a different measurement from the one the ratchet's baseline was taken at, and
// a count that does not know which of the two it is cannot be read.
func fallbackFacesInUse() []*shape.Face {
	if w, ok := fontSetForWPT().(suiteFonts); ok {
		return w.fallback
	}
	return nil
}

// notoFaceNames is the fallback library, in the order it is asked in.
//
// It is named rather than written inline in notoFaces because it is what
// "the library is complete" means: a face that is not loaded changes what every
// document holding text it covers is set in, so the count of loaded faces is
// checked against this list rather than against a number written down beside it.
var notoFaceNames = []string{
	// Broadest first, so the common case is answered by the first face
	// asked. The rest each add a script the one before it does not have.
	"NotoSans-Regular.ttf",
	"NotoSansHebrew-Regular.ttf",
	"NotoSansArabic-Regular.ttf",
	"NotoSansDevanagari-Regular.ttf",
	"NotoSerifTibetan-Regular.ttf",
	"NotoSansArmenian-Regular.ttf",
	"NotoSansGeorgian-Regular.ttf",
	"NotoSansJP-VF.ttf",
	// One block apiece, and each is the only face here with a glyph for it:
	// Ogham, Coptic, Deseret, and the Number Forms the Roman numerals live
	// in. They are last because they answer nothing else — a face that
	// covers one script is asked after every face that might cover the
	// text outright.
	"NotoSansOgham-Regular.ttf",
	"NotoSansCoptic-Regular.ttf",
	"NotoSansDeseret-Regular.ttf",
	"NotoSansSymbols-Regular.ttf",
	"Unifont-Regular.otf",
	"UnifontUpper-Regular.otf",
}

// notoFaces loads the fallback faces, or none.
//
// Absent, everything still runs: the documents that need them report a missing
// glyph exactly as they did before, and the ratchet is what says whether that
// has cost anything. The order is coverage-first — the pan-Unicode face, then
// the two that add a script it does not have — so the common case is answered
// by the first one asked.
func notoFaces() []*shape.Face {
	dir, _ := fonttest.NotoRoot()
	if !fonttest.NotoPresent(dir) {
		return nil
	}
	var out []*shape.Face
	for _, name := range notoFaceNames {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			missingFallbackFaces.add(name, err)
			continue
		}
		face, err := loadSuiteFace(data)
		if err != nil {
			missingFallbackFaces.add(name, err)
			continue
		}
		registerBlockFont(face, data)
		out = append(out, face)
	}
	return out
}

// The faces this list names and could not load, and why.
//
// A face that is skipped changes what every document in the corpus is set in,
// and this list used to skip them silently: a "continue" with nothing recorded.
// Two of the fourteen went missing in a CI run and a hundred documents stopped
// passing cleanly, and what the ratchet said about it was "this is a layout
// regression" — which was false, and is the one thing it must not say wrongly,
// because the reading it invites is to lower the number.
//
// So the skip is remembered and the ratchet names it. The library is fetched
// from upstreams that move, and a fetch that half works is the failure this
// exists to make legible.
var missingFallbackFaces missingFaces

type missingFaces struct {
	mu    sync.Mutex
	seen  map[string]bool
	names []string
}

func (m *missingFaces) add(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen[name] {
		return
	}
	if m.seen == nil {
		m.seen = map[string]bool{}
	}
	m.seen[name] = true
	m.names = append(m.names, fmt.Sprintf("%s (%v)", name, err))
}

// list is what could not be loaded, in the order the loader met it.
func (m *missingFaces) list() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.names...)
}

// registerBlockFont records which of a face's glyphs are filled rectangles, for
// the comparison in blockglyph_test.go.
//
// A face with none — which is every ordinary text face — is not registered, and
// its runs are compared as text as they always were. The work is done once per
// face, while the font set is built, because it reads every outline in the font.
func registerBlockFont(face *shape.Face, data []byte) {
	if bf, err := newBlockFont(data); err == nil {
		blockFonts[face] = bf
	}
}

// loadSuiteFace reads a fallback face at the weight a document that says nothing
// is asking for.
//
// A variable font read as it stands is its *default instance*, and a font's
// default is whatever its designer put in fvar — which for NotoSansJP-VF is
// wght 100. Every CJK document in the suite was being set in hairline: not a
// weight any of them asked for, with a hairline's advances and a hairline's line
// breaks, in forty-nine of the reftests.
//
// So a face with a weight axis is instanced at 400, which is what "font-weight:
// normal" means and what an unstyled element computes to. A face with no weight
// axis is read as it stands, because there is nothing to ask it for — and
// LoadInstance is deliberately an error in that case rather than a value quietly
// ignored, which is what makes the two paths distinguishable here.
func loadSuiteFace(data []byte) (*shape.Face, error) {
	if face, err := shape.LoadInstance(data, map[string]float64{"wght": 400}); err == nil {
		return face, nil
	}
	return shape.Load(data)
}

// TestTheSuitesVariableFaceIsLoadedAtNormalWeight.
//
// A variable font read as it stands is its default instance, and a font's
// default is whatever its designer put in fvar. NotoSansJP-VF's wght axis
// defaults to 100, so the suite was setting every CJK document in hairline —
// with a hairline's advances — and forty-nine reftests named "NotoSansJP-Thin"
// in their findings while doing it.
//
// Nothing in the ratchet moved when this was fixed, and that is worth recording
// rather than hiding: this font's ascent and descent do not vary with weight, so
// the line metrics were right all along and only the advances were wrong. What
// changed is that the face the harness lends the engine is now the one a caller
// would lend it.
func TestTheSuitesVariableFaceIsLoadedAtNormalWeight(t *testing.T) {
	data := fonttest.NotoFile(t, "NotoSansJP-VF.ttf")

	// As it stands: the font's own default, which is the lightest weight it has.
	asIs, err := shape.Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if asIs.Name() != "NotoSansJP-Thin" {
		t.Logf("the font's default instance is now %q; this test is about the "+
			"weight the harness asks for, which is below", asIs.Name())
	}

	got, err := loadSuiteFace(data)
	if err != nil {
		t.Fatalf("loadSuiteFace: %v", err)
	}
	if got.Name() == asIs.Name() {
		t.Errorf("the face came back as %q, the same as reading the file as it "+
			"stands; a document that says nothing about weight is asking for 400",
			got.Name())
	}

	// A face with no weight axis is read as it stands rather than refused, which
	// is the other half of loadSuiteFace and is most of the list it loads.
	static := fonttest.NotoFile(t, "NotoSans-Regular.ttf")
	if _, err := loadSuiteFace(static); err != nil {
		t.Errorf("a face with no weight axis was refused: %v", err)
	}
}

// ipaFamilies is the Japanese faces the suite asks for by name.
//
// Four hanging-punctuation documents write
// `font-family: "IPAMincho", "IPAGothic", "IPA明朝", "IPAゴシック"` and no
// @font-face at all, so there is no URL for the resolver to fetch: the family
// is a *system* font the suite expects the platform to have. That is a
// different request from Doulos SIL's, which is an @font-face the engine loads
// through its own resolver, and it is why copying the file into the corpus the
// way Armenian and Georgian were copied cannot answer it.
//
// So the harness answers it the way a caller would, which is what this whole
// file is: a font library lent to the engine. The Noto faces above are lent for
// *coverage* — FaceFor asks which face can set some text — and these are lent
// by *name*, which is the other half of the same job.
//
// The two Japanese names are the same two faces spelled in Japanese, which is
// how the suite writes them.
var ipaFamilies = map[string]string{
	"ipamincho": "ipam.ttf",
	"ipa明朝":     "ipam.ttf",
	"ipagothic": "ipag.ttf",
	"ipaゴシック":   "ipag.ttf",
}

// ipaFaces loads them, or none.
//
// Missing files are recorded in the same place a missing fallback face is, for
// the same reason: a face that is not loaded changes what the documents naming
// it are set in, and a silent skip is what once turned an incomplete library
// into "this is a layout regression".
func ipaFaces() map[string]*shape.Face {
	dir, _ := fonttest.NotoRoot()
	if !fonttest.NotoPresent(dir) {
		return nil
	}
	out := map[string]*shape.Face{}
	byFile := map[string]*shape.Face{}
	for family, name := range ipaFamilies {
		face, ok := byFile[name]
		if !ok {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				missingFallbackFaces.add(name, err)
				continue
			}
			if face, err = loadSuiteFace(data); err != nil {
				missingFallbackFaces.add(name, err)
				continue
			}
			registerBlockFont(face, data)
			byFile[name] = face
		}
		out[family] = face
	}
	return out
}
