// Package shape turns text into glyphs, and answers the measurement questions
// laying it out asks.
//
// It reads a font program — sfnt or CFF, static or variable — and does the two
// things a layout engine cannot do without: it says which glyphs a string is
// set with and how wide they are, and it says what a document format needs in
// order to embed the face. What it does not do is write a file. See
// embedding.go: what is here is the facts, in the font's own units, and packing
// them into any format's encoding belongs to whoever writes it.
//
// # Three kinds of face
//
// A face loaded with Load or LoadInstance is composite: its codes are two
// bytes, big-endian, and name a glyph — the glyph index, or for a CID-keyed CFF
// the glyph's CID (see codeForGID) — which is what a Type0 font with
// Identity-H encoding reads (ISO 32000-2 9.7). It is the only kind that can set
// most of Unicode, and the only kind that is shaped: every layout table is
// keyed by glyph index.
//
// LoadSimple reads the same program as a simple font, whose codes are one
// byte of WinAnsiEncoding, and Standard is one of the fourteen faces a reader
// is expected to have, with no program at all. Both are set a code per
// character at the font's published widths, with no substitution and no
// positioning — see shapeByCode — because a one-byte code cannot name what
// those would produce.
//
// Encode is the plain path: one code per character, or per part of a
// character the face draws decomposed, with no rules applied.
//
// # Subsetting, and the ordering it imposes
//
// Only the glyphs a face has been asked to encode or shape are kept, so Subset
// (or SubsetGlyphs) has to come after everything that sets text in the face.
// Subsetting first produces a program carrying .notdef alone, and every glyph
// the document goes on to show is one the program does not define; nothing
// here refuses that, because nothing here can tell a document that set no
// text from one that set it too late.
//
// # Shaping
//
// ShapeGlyphs is the full pipeline: the font's substitutions, kerning, mark
// attachment and cursive forms, returning positioned glyphs. The
// ShapeGlyphsInContext family is the same with the text either side of a run,
// and MeasureShaped measures what it draws.
//
// The rules applied are those the font declares for the run's own script, and
// for the language system of the run's language (Features.Language). The
// syllabic scripts are also reordered: their characters are not stored in the
// order they are drawn, and ShapeGlyphs puts them right. Four models between
// them — nine Indic scripts share one (indic.go), Khmer (khmer.go) and Myanmar
// (myanmar.go) each have their own, and the Universal Shaping Engine (use.go)
// covers Tibetan, Javanese, Balinese, Sinhala and a long tail. See layout.go for
// exactly what is read and each shaper's own file for what it covers.
//
// # Subsetting
//
// A font whose outlines are CFF2 is read as the CFF font it draws where it is
// cut (cff2cff.go), and is subsetted and embedded as that.
//
// Both glyf and CFF outlines are subsetted, by the same rule: glyph indices are
// retained and a dropped glyph becomes an empty one. A CID-keyed CFF is the
// exception. Its subset holds only the glyphs kept, renumbered in their order,
// with a charset that gives each the CID it had — so the CIDs Encode writes
// still name the glyphs they did, and the subset carries nothing else.
package shape

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/mgilbir/forme/font"
)

// maxFontWork bounds the work of reading one font program: the cmap, the glyf
// walk and the CFF's INDEXes and Private DICTs, all drawn from one font.Budget.
// A font reaching this is malformed or hostile; the readers stop rather than
// spinning, and Load refuses it rather than embedding a font it only half knows.
//
// It is the figure the cmap alone used to be allowed per subtable, now for the
// whole font. That is nearly four times the one legitimate shape that comes
// close — a LastResort-style format 13 subtable mapping all of Unicode, 1.1
// million codes — and more than forty times what the largest CJK face in the
// corpora spends (Noto Sans SC, about ninety thousand).
const maxFontWork = 1 << 22

// Face is a loaded font program: its metrics, its character-to-glyph mapping,
// and the bytes to embed.
//
// It is not safe for concurrent use. Encode and every shaping call record
// which glyphs a document used — it is what Subset keeps — so two goroutines
// setting text through one Face, even only to measure it, race on that record.
// The layout tables cached as each script selects them, and the shaping plans,
// are behind a lock and are shared by Clone; the record is not, and Clone is
// how to set text from two goroutines. See Clone.
type Face struct {
	// gidToCID is the CID of each glyph index, for a face whose outlines are a
	// CID-keyed CFF. nil otherwise, and nil is the ordinary case: for every
	// other kind of face a glyph index is the only numbering there is.
	gidToCID []int
	// The character collection those CIDs are numbered in, read at load from
	// the same parse gidToCID comes from. Empty for every face that has none —
	// see CharacterCollection, which is the only thing that should read them,
	// because empty here has two meanings and one branch tells them apart.
	registry, ordering string
	supplement         int
	// data is the font program as Load read it — after a WOFF or WOFF 2 is
	// unwrapped, and for LoadInstance the instance it cut — and is what
	// Program hands out. Nothing here writes to it.
	data []byte
	prog *font.Program
	// fsType is OS/2 fsType, the font's embedding permissions, and
	// fsTypeStated whether the font has an OS/2 table long enough to state
	// them. See EmbeddingPermissions.
	fsType       FSType
	fsTypeStated bool

	name       string
	unitsPerEm int
	ascent     int
	descent    int
	capHeight  int
	bbox       [4]int
	italic     float64
	stemV      int
	flags      int

	// The metrics a font states rather than implies, and the record of which of
	// them it actually stated — see Descriptor.Declared, and Metric.
	lineGap                      int
	typoAscent, typoDescent      int
	typoLineGap                  int
	useTypoMetrics               bool
	xHeight                      int
	underlinePos, underlineThick int
	strikeoutPos, strikeoutSize  int
	weight                       int
	widthClass                   int
	styleItalic, styleOblique    bool
	// family and subfamily are the name table's typographic family and
	// style, or the legacy ones: see Family and Subfamily.
	family, subfamily string
	declared          Metric
	// axes are the variation axes fvar declares, which is how a caller learns
	// that a face is variable and where on each axis the outlines it was handed
	// actually sit. A face from LoadInstance has none: it was cut at one point
	// of a design space and no longer has one. See Axes.
	axes []Axis

	// cff reports that the outlines are CFF rather than glyf, which changes
	// both how the program is embedded and whether it can be subsetted.
	cff bool
	// ink measures a CFF glyph's ink by running its charstring, which a glyf
	// face has in its glyph headers instead: nil for every face but a CFF one.
	// See cffink.go.
	ink *cffInk
	// cff2 is a face whose outlines are CFF2 read at its default instance,
	// written as CFF a glyph at a time as it is asked for (cff2cff.go), and
	// nil for every other face. cff2Limits is what writing an instance cut
	// from one reported. Each says which glyphs were written empty, and why.
	cff2       *cff2Default
	cff2Limits []string
	// glyfOut is the glyf outlines of a TrueType face, read the way COLR's are
	// (colrink.go) whether or not the face has that table, for GlyphOutline:
	// nil for a face with CFF outlines. outlines is what GlyphOutline keeps,
	// and is nil only for a face with no font program. See outline.go.
	glyfOut  *colrInk
	outlines *outlineCache
	// colr measures the ink of a colour glyph by painting it, which is asked
	// before the outline is: nil for a face with no COLR table. See
	// colrink.go.
	colr *colrInk
	// bitmap reads the ink of a colour bitmap glyph from its metrics, which is
	// asked before COLR is: nil for a face with no CBDT. See bitmapink.go.
	bitmap *cbdtInk
	// sbix reads the ink of an sbix bitmap glyph from its image's size, which
	// is asked before anything else: nil for a face with no sbix table, or one
	// HarfBuzz would refuse. See sbixink.go.
	sbix *sbixInk
	// varc measures and draws a glyph through the VARC table, which is asked
	// after COLR and before the outline: nil for a face with none, or one
	// HarfBuzz would refuse. See varc.go.
	varc *varcFace
	// varcInk is the ink of each VARC glyph of a face from LoadInstance, which
	// was written out as a glyf outline and whose table was dropped, as
	// HarfBuzz measures the glyph at the location. See varcinstance.go.
	varcInk map[int]extents
	// simple is set when the face is to be embedded as a simple font: one byte
	// per character through WinAnsiEncoding, rather than as a composite font
	// keyed by glyph index.
	simple bool
	// std is set for one of the fourteen standard faces, which has no program
	// at all: its metrics are the ones Adobe published and its codes are
	// WinAnsi characters rather than glyph indices.
	std *stdMetrics

	// layout is what the font declares whatever the script — every feature in
	// the table, taken together. It answers the questions that are about the
	// face rather than about a run: whether it has kerning at all, which
	// features it offers. Shaping does not use it except as the fallback for a
	// font that declares no scripts.
	layout *layout
	// hmtx is the horizontal metrics table and longMetrics how many of its
	// records carry an advance, kept for the two readers that need a glyph's
	// metrics in font units rather than scaled: placing the marks of a face
	// with no positioning of its own (fallback.go), and hanging a glyph set
	// upright from half its advance (vertical.go). Both are integer arithmetic
	// on them.
	hmtx        []byte
	longMetrics int
	// vert is what the vertical metrics of a glyph set upright are read from:
	// vhea and vmtx, VORG, and for a TrueType face the glyph headers. See
	// vertical.go.
	vert verticalTables
	// layoutTables are the GSUB, GPOS, GDEF and kern bytes, kept so that the
	// layout can be read again for the script and language of a run;
	// positionings and scriptLayouts cache those readings, by what each
	// selected.
	layoutTables map[string][]byte
	// cache holds the per-script readings. It is a pointer because faces made
	// for separate documents share one parse and therefore share these too:
	// what a font declares for a script does not depend on the document, and
	// reading tens of thousands of kern pairs again per document is pure waste.
	cache *layoutCache
	// varCoords is where in a variable font's design space this face was read,
	// in normalized coordinates, or nil for a font that has no design space.
	// The rules a font states can differ across it — see FeatureVariations in
	// layout.go — and the default instance is zero on every axis, which is what
	// nil stands for.
	varCoords []float64

	// settingsOn and settingsOff are the features this face was loaded asking
	// for and against — an @font-face rule's font-feature-settings — in the
	// settled form Features.Tags is kept in. Empty for a face nobody asked
	// that of, which is every face but one WithFeatureSettings made. See
	// WithFeatureSettings.
	settingsOn, settingsOff string

	// morx is the face's AAT substitutions, where it has them. See morx.go.
	morx *morxTable

	// bitmapOnly is a face whose glyphs are only bitmaps, with no outlines at
	// all. See BitmapOnly.
	bitmapOnly bool

	// collection is where a face of a collection keeps its tables, which are
	// slices of the collection, for data is nil. See fontcollection.go.
	collection *collectionFace

	used    map[int]bool // glyph indices this face has encoded
	runWork *runWork     // opt-in per-call budget, never shared by Clone
	// spareWork is the budget ShapeGlyphsBounded reuses from call to call on
	// this face, so that bounding a run costs no allocation. Not shared by
	// Clone, for the reason used is not.
	spareWork *runWork
}

// faceName is the face's PostScript name, and "Embedded" for a font that
// states none.
func faceName(name []byte) string {
	if n := postScriptName(name); n != "" {
		return n
	}
	return "Embedded"
}

// hasBitmaps reports whether a font has a table of bitmap glyphs: CBDT and
// CBLC, sbix, or EBDT and EBLC.
func hasBitmaps(tables map[string][]byte) bool {
	present := func(tag string) bool { return len(tables[tag]) > 0 }
	return present("CBDT") && present("CBLC") || present("sbix") || present("EBDT") && present("EBLC")
}

// BitmapOnly reports whether the face's glyphs are only bitmaps: a font with
// CBDT, sbix or EBDT and no glyf, CFF or CFF2 outlines, such as Noto Color
// Emoji's CBDT build. Every glyph's outline is empty, so GlyphOutline draws
// nothing and the glyphs are painted from their images (PaintGlyph); and it
// cannot be subsetted or embedded as a font, which has to carry outlines.
func (f *Face) BitmapOnly() bool { return f.bitmapOnly }

// layoutTableNames are the tables readLayout reads, and so the ones a face
// keeps in order to read them again per script. The rest of the font is not
// retained: f.data already holds it.
var layoutTableNames = [...]string{"GSUB", "GPOS", "GDEF", "kern"}

// keepLayoutTables takes the layout tables out of a parsed font.
func keepLayoutTables(tables map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(layoutTableNames))
	for _, name := range layoutTableNames {
		if t := tables[name]; len(t) > 0 {
			out[name] = t
		}
	}
	return out
}

// noCmapError says why a font with no usable Unicode cmap is refused.
//
// The refusal is one — a face that maps no character to a glyph can set no
// text — but the reason is not, and the reason is what the author reads, in the
// @font-face finding that quotes this. A font whose map is there and names only
// glyphs past maxp's count was told it "has no Unicode character map", which is
// false of it and sends whoever reads it looking for a table the font carries.
// font.CmapState is what tells the cases apart.
func noCmapError(prog *font.Program) error {
	switch prog.CmapState {
	case font.CmapUnreadable:
		return errors.New("fonts: the font's Unicode character map could not be " +
			"read: each of its subtables is in a format this engine does not read, " +
			"is cut short, or maps no character to a glyph")
	case font.CmapNamesNoGlyph:
		return fmt.Errorf("fonts: the font's Unicode character map names no glyph "+
			"the font has: every character in it is mapped to a glyph index past "+
			"the %d glyphs its maxp declares", prog.NumGlyphs)
	}
	return errors.New("fonts: the font has no Unicode character map")
}

// headUnitsPerEm is the em a font's glyphs are drawn on: head's unitsPerEm,
// and a thousand units where the font has no head HarfBuzz reads (one shorter
// than the table's fifty-four bytes), as HarfBuzz takes it.
//
// A head that states an em outside 16..16384 is refused. OpenType allows no
// other (since 1.8.2), and nothing else agrees what such a font is. HarfBuzz
// reads it as a thousand units (hb-ot-head-table.hh, get_upem). FreeType
// refuses the font outright, and so do the browsers' font sanitizer and every
// PDF reader built on FreeType. pdf.js takes the em as stated for its metrics.
// So there is no number this package could measure and write a document's
// widths with that the readers of the embedded program would agree with:
// HarfBuzz's thousand is not the em the program states, and the stated em is
// not one most readers will draw at all. Refusing it is what the author can
// act on; either reading is a page drawn wrong.
func headUnitsPerEm(head []byte) (int, error) {
	if len(head) < 54 {
		return 1000, nil
	}
	u := font.Be16(head, 18)
	if u < 16 || u > 16384 {
		return 0, fmt.Errorf("fonts: the font's head table states %d units to the em, "+
			"outside the 16 to 16384 OpenType allows; FreeType and the browsers refuse "+
			"such a font, so no reader would draw it as measured", u)
	}
	return u, nil
}

// Load parses an sfnt font program — TrueType or OpenType — and prepares it for
// embedding. The bytes are retained as they are, and Subset cuts them down.
//
// A variable font loads at its default instance, which is what its glyf outlines
// already are. LoadInstance is the way to ask for any other point in its design
// space; see instance.go for why the default is rarely the one that was wanted.
func Load(data []byte) (*Face, error) { return loadFace(data, nil) }

// loadFace is Load, told where in a design space the program was cut. The
// coordinates are normalized, and nil means the default instance — which is
// zero on every axis, so it is also what a font with no design space gets.
func loadFace(data []byte, coords []float64) (*Face, error) {
	// A WOFF is an sfnt taken apart and deflated table by table, and a WOFF 2
	// is one taken apart and re-encoded; unwrapping either here — before
	// anything has looked at a table — is the whole of what this module needs
	// to know about them. Everything below reads the result as the ordinary
	// font program it is.
	if font.IsWOFF(data) || font.IsWOFF2(data) {
		sfnt, err := font.DecodeWOFF(data)
		if err != nil {
			return nil, err
		}
		data = sfnt
	}
	tables := font.SFNTTables(data)
	if tables == nil {
		if font.CollectionOffsets(data) != nil {
			return nil, errors.New("fonts: a font collection (.ttc or .otc) holds several fonts; " +
				"LoadCollection loads one of them")
		}
		return nil, errors.New("fonts: not an sfnt font program (TrueType or OpenType)")
	}
	return loadTables(data, tables, coords)
}

// loadTables is loadFace for a font taken apart into its tables: data is the
// font program they were taken from, or nil for a face of a collection, whose
// tables are slices of the collection and not a program of their own (see
// fontcollection.go).
func loadTables(data []byte, tables map[string][]byte, coords []float64) (*Face, error) {
	unitsPerEm, err := headUnitsPerEm(tables["head"])
	if err != nil {
		return nil, err
	}
	_, hasGlyf := tables["glyf"]
	_, hasCFF := tables["CFF "]
	// CFF2 outlines: a variable font whose charstrings blend their own
	// variations. A document format that predates CFF2 cannot carry it, and a
	// face is embedded as what it draws, so such a face is read as the CFF font
	// it draws at its default instance (cff2Default in cff2cff.go) — exactly
	// what the CFF2 charstrings draw there, since nothing is blended — and is
	// measured, subsetted and embedded as that. LoadInstance cuts it anywhere
	// else.
	_, hasCFF2 := tables["CFF2"]
	cff2Outlines := hasCFF2 && !hasGlyf && !hasCFF
	// A font whose glyphs are only bitmaps — CBDT, sbix or EBDT and no
	// outlines, which is how Noto Color Emoji's CBDT build and most bitmap
	// emoji fonts are made — is a face whose every outline is empty: it is
	// shaped and measured from hmtx and the bitmaps, and painted from them.
	// See BitmapOnly.
	bitmapOnly := !hasGlyf && !hasCFF && !cff2Outlines && hasBitmaps(tables)
	if !hasGlyf && !hasCFF && !cff2Outlines && !bitmapOnly {
		return nil, errors.New("fonts: the font carries neither glyf nor CFF outlines, nor bitmaps")
	}
	// One budget for the whole font, shared by the sfnt and CFF readers, so
	// that what is bounded is the font and not each of its parts.
	budget := font.NewBudget(maxFontWork)
	prog := font.ParseSFNTTablesWithin(tables, budget)
	if prog == nil {
		return nil, errors.New("fonts: the font program could not be parsed")
	}
	if prog.CmapPartial {
		return nil, errors.New("fonts: the font's character map is truncated, so its glyph coverage is unknown")
	}
	if err := budget.Err(); err != nil {
		return nil, err
	}
	if len(prog.Cmap) == 0 {
		return nil, noCmapError(prog)
	}
	// The CID a glyph index stands for, when the outlines are a CID-keyed CFF
	// and the two numberings differ. nil for every other kind of face, which is
	// what "the code is the glyph index" means.
	var gidToCID []int
	var registry, ordering string
	var supplement int
	var cff2 *cff2Default
	if cff2Outlines {
		var err error
		if cff2, err = newCFF2Default(tables, budget); err != nil {
			// Said as what it is — a CFF2 font whose table cannot be read —
			// rather than as the table's own complaint alone, which names an
			// INDEX or a DICT and not the kind of font it was.
			return nil, fmt.Errorf("fonts: the font's outlines are CFF2, and its CFF2 table cannot be read: %w", err)
		}
		// The CFF it is embedded as is CID-keyed in Adobe-Identity-0, each
		// glyph's CID its index: see cff2cff.go.
		gidToCID = identityCIDs(prog.NumGlyphs)
		registry, ordering, supplement = "Adobe", "Identity", 0
	} else if !hasGlyf && !bitmapOnly {
		// The CFF table has to be parsed on its own: the sfnt reader answers
		// questions from cmap, hmtx and maxp and never opens it, so nothing
		// about the outlines is known until it is asked directly. (Reading
		// prog.WidthByCID here instead would be a check that can never fire.)
		//
		// Not the widths, which are the costly part of a CFF — a charstring
		// interpreted per glyph — and which nothing here reads: the advances
		// come from hmtx.
		cff := font.ParseCFFGlyphs(tables["CFF "], budget)
		if err := budget.Err(); err != nil {
			return nil, err
		}
		if cff == nil {
			return nil, errors.New("fonts: the CFF table could not be parsed")
		}
		// A CID-keyed CFF numbers its glyphs by CID and maps CID to glyph index
		// through its charset — two numberings, not one. Reading it needs
		// neither: a charstring is found by glyph index like any other, the cmap
		// gives glyph indices, and the advances come from hmtx. So shaping and
		// measuring a CID-keyed face works exactly as it does for any other, and
		// this used to refuse one anyway.
		//
		// Where the two numberings part is embedding, and gidToCID below is what
		// tells them apart there: a CIDFontType0 is addressed by CID, so Encode
		// says CIDs for such a face and glyph indices for every other.
		gidToCID = cff.GIDToCID
		// And the collection those CIDs belong to, taken from the same parse.
		// Reading it here rather than leaving the caller to parse the CFF again
		// is not only the cost — 8 ms of a 21 ms load, for a face this size —
		// but which bytes: a caller reaching for the CFF itself has to pick
		// between the face and the subset, and only one of those is the program
		// a document ends up carrying.
		registry, ordering, supplement = cff.Registry, cff.Ordering, cff.Supplement
	}

	f := &Face{
		data:       data,
		gidToCID:   gidToCID,
		registry:   registry,
		ordering:   ordering,
		supplement: supplement,
		prog:       prog,
		cff:        !hasGlyf && !bitmapOnly,
		bitmapOnly: bitmapOnly,
		unitsPerEm: unitsPerEm,
		varCoords:  coords,
		used:       map[int]bool{},
	}
	head := tables["head"]
	if len(head) >= 54 {
		f.bbox = [4]int{
			signed16(font.Be16(head, 36)), signed16(font.Be16(head, 38)),
			signed16(font.Be16(head, 40)), signed16(font.Be16(head, 42)),
		}
		if font.Be16(head, 44)&0x02 != 0 { // macStyle italic
			f.italic = -12
		}
	}
	if hhea := tables["hhea"]; len(hhea) >= 36 {
		f.ascent = signed16(font.Be16(hhea, 4))
		f.descent = signed16(font.Be16(hhea, 6))
		f.lineGap = signed16(font.Be16(hhea, 8))
		f.declared |= MetricLineGap
		f.hmtx, f.longMetrics = tables["hmtx"], font.Be16(hhea, 34)
	}
	// sCapHeight arrived in OS/2 version 2, so what says a font states one is
	// the version and not the table's length: a version 1 table long enough to
	// reach offset 88 has something else there, and a version 2 table that puts
	// a zero there has not measured its capitals. Both were read as a declared
	// cap height, and the zero was then quietly replaced by the ascent two
	// lines below — so a Descriptor said "this font states a cap height" and
	// handed back a number the font had never written.
	if os2 := tables["OS/2"]; len(os2) >= 90 && font.Be16(os2, 0) >= 2 {
		if h := signed16(font.Be16(os2, 88)); h != 0 {
			f.capHeight = h
			f.declared |= MetricCapHeight
		}
	}
	f.readOS2(tables["OS/2"])
	f.readStyle(tables["OS/2"], head)
	f.family = nameWithFallback(tables["name"], 16, 1)
	f.subfamily = nameWithFallback(tables["name"], 17, 2)
	f.readPost(tables["post"])
	switch {
	case cff2 != nil:
		f.cff2 = cff2
		f.ink = newCFF2Ink(cff2)
	case !hasGlyf && !bitmapOnly:
		f.ink = newCFFInk(tables["CFF "], prog.NumGlyphs)
	}
	if len(tables["COLR"]) > 0 {
		f.colr = newCOLRInk(f, tables, prog.NumGlyphs)
	}
	if hasGlyf {
		f.glyfOut = newCOLRInk(f, tables, prog.NumGlyphs)
	}
	f.outlines = newOutlineCache(programSize(data, tables), tables)
	f.morx = readMorx(tables, prog.NumGlyphs)
	f.bitmap = newCBDTInk(tables, f.unitsPerEm)
	f.sbix = newSbixInk(tables, prog.NumGlyphs, f.unitsPerEm)
	f.varc = newVARCFace(f, tables, prog.NumGlyphs)
	f.vert = readVerticalTables(tables, prog.NumGlyphs, budget)
	if err := budget.Err(); err != nil {
		return nil, err
	}
	f.axes = readAxes(tables["fvar"])
	f.stemV = stemV(tables["OS/2"])
	if f.capHeight == 0 {
		f.capHeight = f.ascent
	}
	f.layoutTables = keepLayoutTables(tables)
	// The unfiltered reading, and the positioning half it is built on, are the
	// ones a font with no ScriptList falls back to, so they are cached under
	// the key a nil selection gets rather than read a second time for it.
	pos := readPositioning(f.layoutTables, nil, noRequiredFeature, coords)
	f.cache = &layoutCache{positionings: map[string]*layout{selectionKey(nil): pos}}
	f.layout = readLayout(f.layoutTables, nil, pos, coords)
	f.name = faceName(tables["name"])
	// Flags (ISO 32000-2 9.8.2, Table 121). Symbolic is the honest answer for a
	// font embedded with Identity-H: the codes are glyph indices, not
	// characters in any standard encoding, so bit 3 (Symbolic) is set and bit 6
	// (Nonsymbolic) is not.
	f.flags = 1 << 2 // Symbolic
	if isFixedPitch(prog) {
		f.flags |= 1 // FixedPitch
	}
	if f.italic != 0 {
		f.flags |= 1 << 6 // Italic
	}
	return f, nil
}

// Name is the font's PostScript name, which becomes /BaseFont.
//
// It is the name for embedding and not a description of what was drawn. On a
// variable face the two can differ: the legacy name records spell only four
// styles, so a face whose default instance is Thin is commonly still named
// Regular there, and several published under the OFL are. Descriptor().Weight
// is what says which.
func (f *Face) Name() string { return f.name }

// An Axis is one of a variable font's variation axes, in user coordinates.
type Axis struct {
	// Tag is the four-character axis tag: wght, wdth, opsz, slnt, ital, or one
	// of the many a foundry may define for itself.
	Tag string
	// Min, Default and Max are the range the axis offers and where it rests.
	Min, Default, Max float64
}

// Axes are the variation axes the face declares, empty for a static font.
//
// A face from Load carries the outlines as they are stored, which is the
// default instance, with every axis at its Default below. Axes is how a caller
// reads which design space it is in and where it was taken, so it can say "this
// face is variable and was taken at wght=100" rather than discovering it from
// the shape of the letters — and it is what a caller reads before naming a
// point to LoadInstance, which is the way to be handed another one.
//
// A face from LoadInstance has no axes. It was cut at one location and is a
// static font, which is what a design point is once it has been chosen; what it
// was cut at is in Descriptor().Weight and in Name.
func (f *Face) Axes() []Axis {
	if len(f.axes) == 0 {
		return nil
	}
	out := make([]Axis, len(f.axes))
	copy(out, f.axes)
	return out
}

// IsVariable reports whether the face declares variation axes.
func (f *Face) IsVariable() bool { return len(f.axes) > 0 }

// readOS2 takes the metrics OS/2 states beyond the cap height already read.
//
// The offsets are the table's, and the version gates the last of them: sxHeight
// arrived in version 2, and a version 0 table simply stops before it. Reading
// it anyway would return whatever followed the table in the file.
func (f *Face) readOS2(os2 []byte) {
	// fsType is at offset 8 in every version, and read on its own before the
	// length the rest needs: Apple's original version 0 table stops at 68
	// bytes, and a font that states a restriction in one has stated it.
	if len(os2) >= 10 {
		f.fsType, f.fsTypeStated = FSType(font.Be16(os2, 8)), true
	}
	if len(os2) < 78 { // through usWinDescent, which every version has
		return
	}
	f.weight = font.Be16(os2, 4)
	f.declared |= MetricWeight
	f.strikeoutSize = signed16(font.Be16(os2, 26))
	f.strikeoutPos = signed16(font.Be16(os2, 28))
	f.declared |= MetricStrikeout
	f.useTypoMetrics = font.Be16(os2, 62)&0x80 != 0 // fsSelection USE_TYPO_METRICS
	f.typoAscent = signed16(font.Be16(os2, 68))
	f.typoDescent = signed16(font.Be16(os2, 70))
	f.typoLineGap = signed16(font.Be16(os2, 72))
	f.declared |= MetricTypoMetrics
	if font.Be16(os2, 0) >= 2 && len(os2) >= 88 {
		f.xHeight = signed16(font.Be16(os2, 86))
		f.declared |= MetricXHeight
	}
}

// readPost takes the underline the post table states.
func (f *Face) readPost(post []byte) {
	if len(post) < 12 {
		return
	}
	// The italic angle, which nothing read.
	//
	// post states it as a 16.16 fixed-point number of degrees counter-clockwise
	// from vertical, and it is the only place a font says how far its letters
	// lean. What stood in for it was macStyle's italic *bit* and the constant
	// -12: every italic face embedded at twelve degrees whatever it was drawn
	// at, and every oblique instance of a variable font — where the angle is
	// the axis being varied — embedded at zero, because macStyle's bit is not
	// set on the default instance the bit was read from.
	//
	// A font that says nothing keeps the macStyle guess, since a reader that
	// leans an italic by nothing at all is worse than one that leans it by
	// roughly the right amount.
	if angle := postItalicAngle(post); angle != 0 {
		f.italic = angle
		f.declared |= MetricItalicAngle
	}
	f.underlinePos = signed16(font.Be16(post, 8))
	f.underlineThick = signed16(font.Be16(post, 10))
	f.declared |= MetricUnderline
}

// postItalicAngle reads post's italicAngle: a 16.16 signed fixed-point number of
// degrees counter-clockwise from vertical.
//
// Rounded to a hundredth. The angle reaches a PDF as a number and every real
// font states two decimal places or fewer, so what is past them is the binary
// fraction's own noise — -12.000001 where the font wrote -12.
func postItalicAngle(post []byte) float64 {
	if len(post) < 8 {
		return 0
	}
	return math.Round(fixed1616(font.Be32(post, 4))*100) / 100
}

// readAxes reads fvar's axis records, which is all this module wants from it.
//
// The instance records after them are deliberately not read. They name points
// in the design space, and naming a point is only useful to something that can
// go there — which this cannot.
// It is parseFvar's answer and not a second reading of the same table.
//
// There were two, and they did not agree: this one checked neither the version
// nor the axis count nor that an axis runs the way an axis runs, and stopped at
// the first record that did not fit rather than refusing the table. So Axes
// named axes LoadInstance would not accept — a design space with a default
// outside its own range, or sixty-five thousand axes of it — and a caller
// reading Axes to find out what it may ask for was told something the thing it
// would ask could not do.
func readAxes(fvar []byte) []Axis {
	axes, err := parseFvar(fvar)
	if err != nil {
		return nil
	}
	out := make([]Axis, len(axes))
	for i, a := range axes {
		out[i] = Axis{Tag: a.tag, Min: a.min, Default: a.def, Max: a.max}
	}
	return out
}

// fixed1616 reads fvar's 16.16 signed fixed point as the number it stands for.
func fixed1616(v uint32) float64 { return float64(int32(v)) / 65536 }

// GlyphID maps a rune to the number the content stream will carry for it,
// reporting whether the font covers it.
//
// For an embedded face that number is a glyph index, because the encoding is
// Identity-H. For a standard face it is the WinAnsiEncoding byte, because the
// encoding is a character encoding — the two are different kinds of number and
// the only thing they have in common is that Encode writes them.
func (f *Face) GlyphID(r rune) (int, bool) {
	if f.simple {
		code, _, ok := stdCode(r)
		if !ok {
			return 0, false
		}
		if gid, mapped := f.prog.Cmap[r]; !mapped || gid == 0 {
			return 0, false
		}
		return int(code), true
	}
	if f.std != nil {
		code, name, ok := stdCode(r)
		if !ok {
			return 0, false
		}
		if _, has := f.std.widths[name]; !has {
			return 0, false
		}
		return int(code), true
	}
	gid, ok := f.prog.Cmap[r]
	return gid, ok && gid != 0
}

// Advance is the horizontal advance of a rune in thousandths of an em, the
// unit PDF text space uses. It reports whether the font maps the rune at all;
// for one it does not, the advance is .notdef's.
func (f *Face) Advance(r rune) (float64, bool) {
	if f.std != nil {
		return f.stdAdvance(r)
	}
	if f.simple {
		// The glyph is found through the character map as always; only the code
		// that will name it differs.
		gid, ok := f.prog.Cmap[r]
		if !ok || gid == 0 {
			return f.advanceGID(0), false
		}
		return f.advanceGID(gid), true
	}
	gid, ok := f.GlyphID(r)
	if !ok {
		return f.advanceGID(0), false
	}
	return f.advanceGID(gid), true
}

// composite reports whether the face is embedded as a composite font, keyed by
// glyph index with two-byte codes.
//
// It is the question the shaping paths turn on. A composite face's GlyphID
// returns a glyph index, which is what the layout tables are keyed by and what
// a two-byte code names. A simple or standard face's GlyphID returns a
// *character code* instead — one byte, WinAnsi — and that number names nothing
// in GSUB or GPOS. Shaping such a face by glyph index applies the wrong kerns
// and writes codes of the wrong width, which is a page of scrambled text.
func (f *Face) composite() bool { return f.std == nil && !f.simple }

// advanceGID is the advance of a glyph index, in thousandths of an em.
//
// Not font units, which is what this said for a long time and what a reader of
// the header would expect: font.FontProgram scales WidthByGID on the way in, so
// that every caller of it is on one grid whatever the face's own grid is. A
// Descriptor's lengths *are* font units, and the two are the same number only
// for the fonts whose em is a thousand units — which the bundled face is, so
// nothing here saw the difference.
//
// It is only meaningful for a composite face; the guard is against a caller
// reaching it for one of the others, where there may be no program at all.
func (f *Face) advanceGID(gid int) float64 {
	if f.prog == nil || gid < 0 || gid >= len(f.prog.WidthByGID) {
		return 0
	}
	return f.prog.WidthByGID[gid]
}

// Measure is the width of a string set at the given size, in user-space units.
// Runes the font does not map contribute .notdef's advance, which is what a
// renderer will draw — except the space separators and the non-breaking hyphen
// a composite face draws with a stand-in, which contribute the stand-in's (see
// spacefallback.go).
func (f *Face) Measure(s string, size float64) float64 {
	var total float64
	for _, r := range s {
		if hiddenAfterShaping(r) {
			// Nothing is drawn for it, so nothing is measured for it. This has
			// to agree with what Encode emits or a caller lays out to one width
			// and draws another — see ignorable.go.
			//
			// The *after* predicate, and it is the right one whichever kind of
			// face this is. A join control survives into shaping so that the
			// joining and the syllable models can read it, and is taken back out
			// before anything is positioned — so nothing is ever drawn for one
			// and nothing may be measured for one. Asking the shaping predicate
			// here charged a full character for a non-joiner on a simple face,
			// which then drew it as a space.
			continue
		}
		var one [2]rune
		if parts, ok := f.drawnAs(r, 0, one[:0]); ok {
			for _, p := range parts {
				w, _ := f.Advance(p)
				total += w
			}
			continue
		}
		w, _ := f.Advance(r)
		if !f.composite() {
			// A face addressed by character code sets a character it has no
			// code for as a space, so it is a space that must be measured.
			_, w = f.missingByCode()
		} else if gid, kind, ok := f.standIn(r); ok {
			// A composite face draws a space separator it lacks as its own
			// space, at the separator's width, and a non-breaking hyphen as a
			// hyphen: see spacefallback.go.
			w = f.standInAdvance(kind, f.advanceGID(gid))
		}
		total += w
	}
	return total * size / 1000
}

// drawnAs is what this face draws for a character, appended to out: the
// character itself where the face has it, and where it does not, its canonical
// decomposition, if the face has every part of that. ok is false where it has
// neither.
//
// It is the same rule normalize applies, asked for the same reason: what is
// measured has to be what is drawn. The shaper takes a character the face
// cannot set and emits its canonical decomposition instead, so a measurement
// that stopped at .notdef reports a width the page will not have.
//
// The case is not exotic. U+2000 EN QUAD decomposes to U+2002 EN SPACE and
// U+2001 EM QUAD to U+2003 EM SPACE — Unicode states both as singletons — and a
// font carrying the spaces without the quads is ordinary. The suite's Ahem is
// one: it sets an en quad as a half em, and this measured it as a whole one,
// which put the words either side of three of them two ems further apart than
// they are drawn.
//
// Measure, Encode and the path that sets a face by character code all ask it,
// so that the three agree. Measure alone did, and the other two drew what it
// measured as a decomposition as a space or a .notdef: U+212A KELVIN SIGN in
// Helvetica measured as a K and drawn as a space.
//
// Only if *every* part is one this face has: half a decomposition drawn and the
// rest as .notdef is the disagreement this exists to prevent.
func (f *Face) drawnAs(r rune, depth int, out []rune) ([]rune, bool) {
	if _, ok := f.GlyphID(r); ok {
		return append(out, r), true
	}
	if depth >= maxDecompositionDepth {
		return out, false
	}
	a, b, decomposed := canonicalDecompose(r)
	if !decomposed {
		return out, false
	}
	n := len(out)
	out, ok := f.drawnAs(a, depth+1, out)
	if ok && b != 0 {
		out, ok = f.drawnAs(b, depth+1, out)
	}
	if !ok {
		return out[:n], false
	}
	return out, true
}

// missingByCode is what a face addressed by character code — one of the
// fourteen standard faces, or a face embedded as a simple font — sets for a
// character it has no code for: the space's code, and the width the space is
// drawn at. Measure, Encode and the by-code shaping path all ask it, so that
// what is measured, what is written and what is drawn are one answer.
//
// Both kinds are addressed by one byte of WinAnsiEncoding, so a character
// outside it — or in it, with no glyph in the face — has no byte that means it.
// A space is what a reader shows for a code the font leaves undefined, and it
// keeps the character's place in the text: dropped, "aαb" is extracted as the
// word "ab". The simple faces had three answers: Measure took the character's
// own advance, or .notdef's where the face had not even that; Encode left it
// out; and the by-code path, which is what layout draws with, drew a space.
// The standard faces were already set as a space by all three.
func (f *Face) missingByCode() (code int, width float64) {
	width, _ = f.Advance(' ')
	return ' ', width
}

// Encode maps a string to the character codes the face is embedded with, one
// per character — or one per part, for a character the face draws as its
// canonical decomposition (see drawnAs). For a composite face that is what a
// Type0/Identity-H font expects: two bytes per glyph, big-endian, each the
// glyph index, or the glyph's CID where the outlines are CID-keyed. For a
// simple or standard face it is one byte of WinAnsiEncoding.
//
// On a composite face a rune the font does not map encodes as glyph 0, which
// renders as .notdef — the visible "this font has no glyph for that" box —
// unless the shaper draws it with a stand-in, whose glyph it is encoded as
// (see spacefallback.go); on a simple or standard face it encodes as the space
// (see missingByCode). That is deliberate: an error here would mean a caller could not lay out text
// containing one stray character, and silently dropping it would lose content.
// The second result reports how many runes were missing so a caller that cares
// can react.
func (f *Face) Encode(s string) (codes []byte, missing int) {
	if f.simple {
		return f.encodeSimple(s)
	}
	if f.std != nil {
		// One byte per character: the codes are WinAnsi, not glyph indices.
		// A character the encoding has no code for becomes the space — see
		// missingByCode — and the count says how many there were. Each code
		// is recorded as used, as the by-code path records what it draws, so
		// that encoding and drawing one text leave the same record.
		codes = make([]byte, 0, len(s))
		var parts []rune
		for _, r := range s {
			if hiddenAfterShaping(r) {
				continue // nothing is drawn for it, so it gets no code
			}
			var ok bool
			if parts, ok = f.drawnAs(r, 0, parts[:0]); !ok {
				missing++
				code, _ := f.missingByCode()
				f.used[code] = true
				codes = append(codes, byte(code))
				continue
			}
			for _, p := range parts {
				code, _ := f.GlyphID(p)
				f.used[code] = true
				codes = append(codes, byte(code))
			}
		}
		return codes, missing
	}
	codes = make([]byte, 0, 2*len(s))
	var parts []rune
	for _, r := range s {
		if hiddenAfterShaping(r) {
			continue // nothing is drawn for it, so it gets no code
		}
		var ok bool
		if parts, ok = f.drawnAs(r, 0, parts[:0]); !ok {
			// The glyph the shaper draws in its place, where there is one —
			// see spacefallback.go — and .notdef where there is not.
			gid, _, stood := f.standIn(r)
			if !stood {
				missing++
			}
			f.used[gid] = true
			code := f.codeForGID(gid)
			codes = append(codes, byte(code>>8), byte(code))
			continue
		}
		for _, p := range parts {
			gid, _ := f.GlyphID(p)
			f.used[gid] = true
			code := f.codeForGID(gid)
			codes = append(codes, byte(code>>8), byte(code))
		}
	}
	return codes, missing
}

// codeForGID is the two-byte code a glyph is addressed by in the embedded font.
//
// For everything but a CID-keyed CFF that is the glyph index, which is what
// Identity-H means and why the two words are used interchangeably about most
// faces. A CIDFontType0 is addressed by CID instead: the code goes into the
// font's charset and comes out as a glyph index, so writing the glyph index
// there would send the reader to whatever glyph happens to carry that CID.
//
// The two agree for a font whose charset is the identity, which many are — so a
// mistake here shows on some CJK faces and not others, which is the worst way
// for it to show.
func (f *Face) codeForGID(gid int) int {
	if f.gidToCID == nil || gid < 0 || gid >= len(f.gidToCID) {
		return gid
	}
	return f.gidToCID[gid]
}

// Used returns the glyph indices this face has encoded, in order. It is what a
// subsetter will keep, and what /CIDSet is written from.
//
// Shaping records every glyph it returns and Encode every code it writes, so
// text is here however it is set. A glyph drawn by its index is not text and
// reaches neither: it is here because whoever drew it said so, with Use.
func (f *Face) Used() []int {
	out := make([]int, 0, len(f.used))
	for gid := range f.used {
		out = append(out, gid)
	}
	sort.Ints(out)
	return out
}

// Use records glyphs drawn by their indices, so that Used lists them and a
// subset keeps them.
//
// It is for a glyph no shaping returned and no Encode wrote: a formula's
// stretched operator, a size variant or the pieces of an assembly, which stand
// for no character and are drawn by index (layout.DrawGlyphs, which layout
// records itself), or any glyph a caller draws by index for reasons of its
// own. A subset built without them draws nothing where they are, and a /CIDSet
// written without them says the page draws glyphs the program does not have.
//
// The indices are the ones Used and SubsetGlyphs report. One the face does not
// have — negative, or past NumGlyphs — is not recorded: it is no glyph of this
// face, and a record naming it would have /CIDSet claim a glyph no subset can
// keep. A standard face has no glyph indices, and records nothing.
func (f *Face) Use(gids ...int) {
	n := f.NumGlyphs()
	for _, gid := range gids {
		if gid >= 0 && gid < n {
			f.used[gid] = true
		}
	}
}

// UnitsPerEm is the font's own coordinate grid: how many units make one em.
//
// A caller working in the thousandths of an em this package reports needs it
// only to convert back — to compare against a tool that reports font units, or
// to read a value out of the font's own tables. Almost nothing does.
func (f *Face) UnitsPerEm() int { return f.unitsPerEm }

// scale converts a value in font units to the 1/1000 em units PDF wants.
func (f *Face) scale(v int) float64 {
	return float64(v) * 1000 / float64(f.unitsPerEm)
}

func signed16(v int) int {
	if v >= 0x8000 {
		return v - 0x10000
	}
	return v
}

// isFixedPitch reports whether every mapped glyph has the same advance.
func isFixedPitch(p *font.Program) bool {
	first, seen := 0.0, false
	for _, gid := range p.Cmap {
		if gid <= 0 || gid >= len(p.WidthByGID) {
			continue
		}
		w := p.WidthByGID[gid]
		if !seen {
			first, seen = w, true
			continue
		}
		if w != first {
			return false
		}
	}
	return seen
}

// postScriptName reads name ID 6 from an sfnt name table, preferring the
// Windows/Unicode record a modern font carries and falling back to the
// Macintosh/Roman one.
func postScriptName(name []byte) string { return nameByID(name, 6) }

// nameByID reads one name record by its ID. Any name a font states is a name it
// states several times, once per platform, and the Windows record is the one to
// believe: the Macintosh record is a single-byte encoding kept for readers that
// no longer exist.
func nameByID(name []byte, id int) string {
	if len(name) < 6 {
		return ""
	}
	count := font.Be16(name, 2)
	storage := font.Be16(name, 4)
	var mac string
	for i := 0; i < count; i++ {
		rec := 6 + 12*i
		if rec+12 > len(name) {
			break
		}
		if font.Be16(name, rec+6) != id { // nameID
			continue
		}
		platform := font.Be16(name, rec)
		length := font.Be16(name, rec+8)
		off := storage + font.Be16(name, rec+10)
		if off+length > len(name) {
			continue
		}
		raw := name[off : off+length]
		switch platform {
		case 3, 0: // Windows or Unicode: UTF-16BE
			var s []byte
			for j := 0; j+1 < len(raw); j += 2 {
				if r := int(raw[j])<<8 | int(raw[j+1]); r > 0 && r < 0x80 {
					s = append(s, byte(r))
				}
			}
			if len(s) > 0 {
				return sanitizeName(string(s))
			}
		case 1: // Macintosh: single byte
			if mac == "" && len(raw) > 0 {
				mac = sanitizeName(string(raw))
			}
		}
	}
	return mac
}

// sanitizeName keeps a PostScript name to the characters ISO 32000-2 9.8.2
// allows in one. A name out of a font file is untrusted input.
func sanitizeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s) && len(out) < 63; i++ {
		c := s[i]
		if c > ' ' && c < 0x7F && c != '(' && c != ')' && c != '<' && c != '>' &&
			c != '[' && c != ']' && c != '{' && c != '}' && c != '/' && c != '%' && c != '#' {
			out = append(out, c)
		}
	}
	return string(out)
}

var errNoGlyphs = fmt.Errorf("fonts: the font program declares no glyphs")

// stemV estimates the dominant vertical stem width, which /FontDescriptor
// requires (ISO 32000-2 9.8.1, Table 120).
//
// It is an estimate and cannot honestly be anything else here. StemV is a Type 1
// notion: an sfnt does not carry it, and the only way to measure it is to
// analyse glyph outlines, deciding which contour segments are the stem of a
// letter — real work, and work whose answer no consumer in this module checks.
//
// What the font does carry is the weight it claims, in OS/2 usWeightClass, and
// stem width tracks weight closely. The relation below is the one PDF tooling
// has converged on: roughly 50 units at Thin rising past 200 at Black, growing
// with the square of weight rather than linearly, which is how stems actually
// thicken. A font with no OS/2 table falls back to the value for Regular.
//
// Being wrong here costs little — a viewer uses StemV only to synthesise a
// substitute face when the embedded one is unavailable, which for an embedded
// subset is never — but being wrong in a *documented* way is the point.
func stemV(os2 []byte) int {
	const regular = 400
	weight := regular
	if len(os2) >= 6 {
		if w := font.Be16(os2, 4); w >= 1 && w <= 1000 {
			weight = w
		}
	}
	// 50 at weight 100, ~88 at 400 (Regular), ~165 at 700 (Bold).
	v := 50 + (weight*weight)/6000
	if v > 250 {
		v = 250
	}
	return v
}

// NumGlyphs is the number of glyphs the font program declares, including
// .notdef. It is unchanged by subsetting, which retains glyph indices. A
// standard face has no program, and reports zero.
func (f *Face) NumGlyphs() int {
	if f.prog == nil {
		return 0
	}
	return f.prog.NumGlyphs
}

// GlyphIDForTest returns the glyph index a character maps to in the font
// program, whatever encoding the face will be embedded with.
//
// GlyphID answers a different question — the number the *content stream* will
// carry, which for a simple font is a character code and not a glyph index — so
// a test checking what the subsetter kept needs this one. There is no other
// caller, and the name says so.
func (f *Face) GlyphIDForTest(r rune) (int, bool) {
	if f.prog == nil {
		return 0, false
	}
	gid, ok := f.prog.Cmap[r]
	return gid, ok && gid != 0
}

// Clone returns a face that shares this one's parsing but keeps its own record
// of what a document used.
//
// It is what a caller with a font library calls per document, and what layout
// calls for every face it is handed: a library is loaded once and used for
// years, and the two things below have to be told apart before it can be.
//
// The split is between what the *font* says and what a *document* did with it.
// The program, the tables and the rules read out of them are facts about the
// font: reading them again for a second document produces the same answer at
// the same cost, and for a face of any size that cost is most of what loading
// one comes to. A layout is built by its readers and never written to
// afterwards, so sharing it is sharing a value, not a variable.
//
// The set of glyphs encoded is the opposite. It is what the subset is computed
// from, so two documents sharing one would each embed a font carrying the
// other's glyphs — and, worse, a /CIDSet describing a set neither of them has.
// That one is always fresh.
//
// The per-script layout caches and the shaping plans are not: they are
// readings of the font's own tables, which no document can change, so a clone
// shares them with the face it was made from, and they are filled lazily
// behind a lock.
//
// So: one face used for two outputs puts each one's glyphs into the other, and
// two documents set at the same time through one face write one map from two
// goroutines. Reading the font again instead costs milliseconds and megabytes
// for an answer that cannot differ. Share the parse, not the face.
func (f *Face) Clone() *Face {
	out := *f
	out.used = map[int]bool{}
	out.runWork = nil
	out.spareWork = nil
	// The cache is deliberately *kept*, not reset: it holds readings of the
	// font's own tables, which no document can change. A layout is written only
	// by its readers, so what is shared is a value; the mutex is there because
	// the map is filled lazily, not because the layouts are mutable.
	return &out
}

// InkExtent is how far the glyphs of s reach above and below the baseline, at
// the given size, and whether the face can say.
//
// A tighter answer than the ascent and descent, and a different kind of answer.
// Ascent and descent describe the *face* — how much room a line of it needs,
// including for the tallest accent and the deepest tail it has — and every run
// set in it is given that much whether or not it uses any. This describes the
// text in hand: an ellipsis is three dots on the baseline however deep the
// face's descenders go, and a row of capitals has nothing below the baseline at
// all.
//
// The caller that needs the difference is one deciding whether some rectangle
// cuts the text: a box that ends just under the baseline does not clip a line
// of capitals, and answering from the face's descent says it does. For deciding
// how much room to *give* a line the face's own metrics remain the right
// numbers, because the next run set in it may be the one with the tail.
//
// It is the vertical extent alone. The horizontal one a caller already has a
// better answer to in Measure, which is the advance the text was laid out to;
// glyphs may overhang it slightly and none of this is precise enough to matter
// there.
//
// ok is false when the face cannot answer for some glyph of s — a CFF glyph
// whose charstring cannot be run, or one past the work the face may spend
// measuring (see LayoutLimits) — and a caller should fall back to the face's
// ascent and descent. It is also false for a string with nothing in it that
// draws.
//
// The numbers come from what the font states: the glyph header for a glyf-based
// face, the box the charstring draws for a CFF one, the box a colour glyph
// paints or its bitmap states, and Adobe's published per-character boxes for
// the fourteen standard ones. The header and the AFM
// are not verified against the outline, and a CFF glyph's box takes in its
// curves' control points, so a glyph's box may be larger than its ink and this
// overstate the run's — which is the safe direction for the question it exists
// to answer.
func (f *Face) InkExtent(s string, size float64) (above, below float64, ok bool) {
	top, bottom, any := f.inkUnits(s)
	if !any {
		return 0, 0, false
	}
	scale := size / float64(f.unitsPerEm)
	return float64(top) * scale, float64(-bottom) * scale, true
}

// inkUnits is InkExtent in the font's own units, before the size is applied.
func (f *Face) inkUnits(s string) (top, bottom int, ok bool) {
	for _, r := range s {
		if hiddenAfterShaping(r) {
			// Drawn as nothing, so it puts ink nowhere. The same exclusion
			// Measure makes, and for the same reason: the two must agree about
			// which characters are on the page.
			continue
		}
		lo, hi, has := f.glyphInk(r)
		if !has {
			return 0, 0, false
		}
		if lo == 0 && hi == 0 {
			// An empty glyph — a space. It marks nothing, so it neither raises
			// nor lowers the run's reach, and a run of them alone answers no.
			continue
		}
		if !ok || hi > top {
			top = hi
		}
		if !ok || lo < bottom {
			bottom = lo
		}
		ok = true
	}
	return top, bottom, ok
}

// glyphInk is one character's vertical extent in font units.
//
// has is false when the face cannot measure the glyph, which is answered for
// the run as a whole rather than per character: a run whose extent is known for
// some of its letters and not for others has no extent this can report.
func (f *Face) glyphInk(r rune) (lo, hi int, has bool) {
	if f.std != nil {
		_, name, ok := stdCode(r)
		if !ok {
			// Outside the encoding, so it is set as a space — which marks
			// nothing. Measure charges it a space's width for the same reason.
			return 0, 0, true
		}
		box, ok := f.std.ink[name]
		if !ok {
			return 0, 0, false
		}
		return box[0], box[1], true
	}
	if f.prog == nil {
		return 0, 0, false
	}
	// The character map rather than GlyphID, which answers with the WinAnsi
	// *code* for a simple face because that is what will be written. What is
	// wanted here is the outline, which is found by index either way.
	gid, ok := f.prog.Cmap[r]
	if !ok || gid <= 0 || gid >= f.prog.NumGlyphs {
		// A character the face does not cover draws the notdef glyph, whose
		// extent is as much a part of the run's as any other's.
		gid = 0
	}
	e, ok := f.glyphExtents(gid)
	if !ok {
		return 0, 0, false
	}
	return e.yBearing + e.height, e.yBearing, true
}
