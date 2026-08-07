// Package fonts embeds font programs into a PDF and answers the measurement
// questions laying text out asks.
//
// It is the other half of drawing text. The content package writes the
// operators; this decides what bytes those operators show and puts the font
// program in the file so a reader can render them.
//
// # Composite fonts only, deliberately
//
// A face is embedded as a Type0 font with Identity-H encoding and a
// CIDFontType2 descendant (ISO 32000-2 9.7). The alternative — a simple font
// with a single-byte encoding — is limited to 256 codes and to the glyphs a
// standard encoding names, which rules out most of Unicode. Anything laying out
// real text needs the composite form, so that is the only form here rather than
// a choice to get wrong.
//
// The practical consequence is in the encoding: a character code is two bytes,
// big-endian, and equals the glyph index. Encode does that mapping; the bytes it
// returns are what content.Builder.ShowText takes.
//
// # Subsetting, and the ordering it imposes
//
// Only the glyphs a face has been asked to encode are embedded, so Embed must
// come after the drawing that uses the font. Embedding first produces a font
// carrying .notdef alone, and every glyph the document goes on to show is one
// the program does not define; Embed refuses that rather than writing it.
//
// # Shaping
//
// Shape applies the font's own kerning and ligatures and returns spans ready
// for a text operator; Encode is the plain path that maps runes to glyphs one
// at a time. ShapeGlyphs is the full pipeline, which also attaches marks and
// joins cursive forms.
//
// The rules applied are those the font declares for the run's own script, and
// for the language system named by SetLanguage. The syllabic scripts are also
// reordered: their characters are not stored in the order they are drawn, and
// ShapeGlyphs puts them right. Nine Indic scripts share one model (indic.go),
// and Khmer (khmer.go) and Myanmar (myanmar.go) each have their own. The
// scripts the Universal Shaping Engine covers — Tibetan, Javanese, Balinese,
// Sinhala and a long tail — are not reordered, so text in them is still not
// correctly set by this package. See layout.go for exactly what is read and
// each shaper's own file for what it covers.
//
// # What it does not do
//
// Both glyf and CFF outlines are subsetted, by the same rule: glyph indices are
// retained and a dropped glyph becomes an empty one.
//
// A CID-keyed CFF is refused outright. Its CIDs are not glyph indices, and
// everything here assumes they are.
package shape

import (
	"errors"
	"fmt"
	"sort"

	"github.com/mgilbir/forme/font"
)

// maxCmapWork bounds the cmap parse. A font reaching this is malformed or
// hostile; the reader reports a partial cmap rather than spinning, and Load
// refuses it rather than embedding a font whose mapping it only half knows.
const maxCmapWork = 1 << 22

// Face is a loaded font program: its metrics, its character-to-glyph mapping,
// and the bytes to embed.
//
// It is not safe for concurrent use. Encode records which glyphs a document
// used, so two goroutines encoding through one Face race on that record; and
// shaping caches the font's layout tables as each script selects them, so two
// goroutines merely *measuring* shaped text race on that cache.
type Face struct {
	data []byte
	prog *font.Program

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
	declared                     Metric
	// axes are the variation axes fvar declares, which is how a caller learns
	// that a face is variable and where on each axis the outlines it was handed
	// actually sit. A face from LoadInstance has none: it was cut at one point
	// of a design space and no longer has one. See Axes.
	axes []Axis

	// cff reports that the outlines are CFF rather than glyf, which changes
	// both how the program is embedded and whether it can be subsetted.
	cff bool
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
	// layoutTables are the GSUB, GPOS, GDEF and kern bytes, kept so that the
	// layout can be read again for the script of a run; positionings and
	// scriptLayouts cache those readings, by what each selected. language names
	// the language system to select within a script; empty is the font's
	// default.
	layoutTables map[string][]byte
	// cache holds the per-script readings. It is a pointer because faces made
	// for separate documents share one parse and therefore share these too:
	// what a font declares for a script does not depend on the document, and
	// reading tens of thousands of kern pairs again per document is pure waste.
	cache    *layoutCache
	language string
	// varCoords is where in a variable font's design space this face was read,
	// in normalized coordinates, or nil for a font that has no design space.
	// The rules a font states can differ across it — see FeatureVariations in
	// layout.go — and the default instance is zero on every axis, which is what
	// nil stands for.
	varCoords []float64

	used map[int]bool // glyph indices this face has encoded
}

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

// Load parses an sfnt font program — TrueType or OpenType — and prepares it for
// embedding. The bytes are retained and written into the PDF as they are.
//
// A variable font loads at its default instance, which is what its glyf outlines
// already are. LoadInstance is the way to ask for any other point in its design
// space; see instance.go for why the default is rarely the one that was wanted.
func Load(data []byte) (*Face, error) { return loadFace(data, nil) }

// loadFace is Load, told where in a design space the program was cut. The
// coordinates are normalized, and nil means the default instance — which is
// zero on every axis, so it is also what a font with no design space gets.
func loadFace(data []byte, coords []float64) (*Face, error) {
	tables := font.SFNTTables(data)
	if tables == nil {
		return nil, errors.New("fonts: not an sfnt font program (TrueType or OpenType)")
	}
	_, hasGlyf := tables["glyf"]
	_, hasCFF := tables["CFF "]
	if !hasGlyf && !hasCFF {
		return nil, errors.New("fonts: the font carries neither glyf nor CFF outlines")
	}
	prog := font.ParseSFNT(data, maxCmapWork)
	if prog == nil {
		return nil, errors.New("fonts: the font program could not be parsed")
	}
	if prog.CmapPartial {
		return nil, errors.New("fonts: the font's character map is truncated, so its glyph coverage is unknown")
	}
	if len(prog.Cmap) == 0 {
		return nil, errors.New("fonts: the font has no Unicode character map")
	}
	if !hasGlyf {
		// The CFF table has to be parsed on its own: the sfnt reader answers
		// questions from cmap, hmtx and maxp and never opens it, so nothing
		// about the outlines is known until it is asked directly. (Reading
		// prog.WidthByCID here instead would be a check that can never fire.)
		cff := font.ParseCFF(tables["CFF "])
		if cff == nil {
			return nil, errors.New("fonts: the CFF table could not be parsed")
		}
		if cff.WidthByCID != nil {
			// A CID-keyed CFF numbers its glyphs by CID and maps CID to glyph
			// index through its charset — two numberings, not one. Everything
			// here assumes they are the same: Encode emits glyph indices as
			// character codes, and /W is written by glyph index. Embedding one
			// anyway produces widths keyed by one numbering and codes by the
			// other, which this module's own validator reports.
			//
			// Handling it means reading the charset and encoding through it.
			// Refusing until then is the honest answer; mis-embedding is not.
			return nil, errors.New("fonts: CID-keyed CFF fonts are not supported; their CIDs are not glyph indices")
		}
	}

	f := &Face{
		data:       data,
		prog:       prog,
		cff:        !hasGlyf,
		unitsPerEm: 1000,
		varCoords:  coords,
		used:       map[int]bool{},
	}
	head := tables["head"]
	if len(head) >= 54 {
		if u := font.Be16(head, 18); u > 0 {
			f.unitsPerEm = u
		}
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
	}
	if os2 := tables["OS/2"]; len(os2) >= 90 {
		f.capHeight = signed16(font.Be16(os2, 88))
		f.declared |= MetricCapHeight
	}
	f.readOS2(tables["OS/2"])
	f.readPost(tables["post"])
	f.axes = readAxes(tables["fvar"])
	f.stemV = stemV(tables["OS/2"])
	if f.capHeight == 0 {
		f.capHeight = f.ascent
	}
	f.layoutTables = keepLayoutTables(tables)
	// The unfiltered reading, and the positioning half it is built on, are the
	// ones a font with no ScriptList falls back to, so they are cached under
	// the key a nil selection gets rather than read a second time for it.
	pos := readPositioning(f.layoutTables, nil, coords)
	f.cache = &layoutCache{positionings: map[string]*layout{selectionKey(nil): pos}}
	f.layout = readLayout(f.layoutTables, nil, pos, coords)
	f.name = postScriptName(tables["name"])
	if f.name == "" {
		f.name = "Embedded"
	}
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
	f.underlinePos = signed16(font.Be16(post, 8))
	f.underlineThick = signed16(font.Be16(post, 10))
	f.declared |= MetricUnderline
}

// readAxes reads fvar's axis records, which is all this module wants from it.
//
// The instance records after them are deliberately not read. They name points
// in the design space, and naming a point is only useful to something that can
// go there — which this cannot.
func readAxes(fvar []byte) []Axis {
	if len(fvar) < 16 {
		return nil
	}
	off, count, size := font.Be16(fvar, 4), font.Be16(fvar, 8), font.Be16(fvar, 10)
	if size < 20 {
		return nil
	}
	var out []Axis
	for i := 0; i < count; i++ {
		p := off + i*size
		if p < 0 || p+20 > len(fvar) {
			break
		}
		out = append(out, Axis{
			Tag:     string(fvar[p : p+4]),
			Min:     fixed1616(font.Be32(fvar, p+4)),
			Default: fixed1616(font.Be32(fvar, p+8)),
			Max:     fixed1616(font.Be32(fvar, p+12)),
		})
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

// advanceGID is the advance of a glyph index, in font units.
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
// renderer will draw.
func (f *Face) Measure(s string, size float64) float64 {
	var total float64
	for _, r := range s {
		if hiddenBeforeShaping(r) {
			// Nothing is drawn for it, so nothing is measured for it. This has
			// to agree with what Encode emits or a caller lays out to one width
			// and draws another — see ignorable.go.
			continue
		}
		w, ok := f.Advance(r)
		if !ok && f.std != nil {
			// A character outside the encoding is set as a space, so it is a
			// space that must be measured.
			w, _ = f.stdAdvance(' ')
		}
		total += w
	}
	return total * size / 1000
}

// Encode maps a string to the character codes a Type0/Identity-H font expects:
// two bytes per glyph, big-endian, each equal to the glyph index. The result is
// what content.Builder.ShowText takes.
//
// A rune the font does not map encodes as glyph 0, which renders as .notdef —
// the visible "this font has no glyph for that" box. That is deliberate: an
// error here would mean a caller could not lay out text containing one stray
// character, and silently dropping it would lose content. The second result
// reports how many runes were missing so a caller that cares can react.
func (f *Face) Encode(s string) (codes []byte, missing int) {
	if f.simple {
		return f.encodeSimple(s)
	}
	if f.std != nil {
		// One byte per character: the codes are WinAnsi, not glyph indices.
		// A character the encoding has no code for becomes the space, which is
		// what a reader would show for an undefined code anyway, and the count
		// says how many there were.
		codes = make([]byte, 0, len(s))
		for _, r := range s {
			if hiddenBeforeShaping(r) {
				continue // nothing is drawn for it, so it gets no code
			}
			code, ok := f.GlyphID(r)
			if !ok {
				missing++
				code = ' '
			}
			codes = append(codes, byte(code))
		}
		return codes, missing
	}
	codes = make([]byte, 0, 2*len(s))
	for _, r := range s {
		if hiddenBeforeShaping(r) {
			continue // nothing is drawn for it, so it gets no code
		}
		gid, ok := f.GlyphID(r)
		if !ok {
			missing++
			gid = 0
		}
		f.used[gid] = true
		codes = append(codes, byte(gid>>8), byte(gid))
	}
	return codes, missing
}

// Used returns the glyph indices this face has encoded, in order. It is what a
// subsetter will keep, and what /CIDSet is written from.
func (f *Face) Used() []int {
	out := make([]int, 0, len(f.used))
	for gid := range f.used {
		out = append(out, gid)
	}
	sort.Ints(out)
	return out
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

// mostCommonWidth picks /DW: the advance shared by the most glyphs, so /W
// carries the exceptions rather than the rule.
func (f *Face) mostCommonWidth() float64 {
	counts := map[float64]int{}
	for _, w := range f.prog.WidthByGID {
		counts[w]++
	}
	best, bestN := 1000.0, -1
	for w, n := range counts {
		if n > bestN || (n == bestN && w < best) {
			best, bestN = w, n
		}
	}
	return best
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

// forDocument returns a face that shares this one's parsing but keeps its own
// record of what a document used.
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
// The per-script layout caches are fresh too. They are lazily filled, so
// sharing them across faces would be a write from two goroutines to one map;
// the alternative is a lock on a path taken once per script per document, and
// the reading they save is small beside the reading forDocument already avoids.
// Clone returns a face that shares this one's reading of the font and records
// its own glyphs.
//
// A face remembers which glyphs it was asked to set, because that is what a
// subset is computed from — so one face used for two outputs puts each one's
// glyphs into the other. Reading the font again instead costs milliseconds and
// megabytes for an answer that cannot differ. Share the parse, not the face.
func (f *Face) Clone() *Face {
	out := *f
	out.used = map[int]bool{}
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
// ok is false when the face cannot answer — a CFF-flavoured font, whose glyph
// extents are in the charstrings and cannot be had without interpreting them —
// and a caller should fall back to the face's ascent and descent. It is also
// false for a string with nothing in it that draws.
//
// The numbers come from what the font states: the glyph header for a glyf-based
// face, and Adobe's published per-character boxes for the fourteen standard
// ones. Neither is verified against the outline, and a font that overstates a
// glyph's box makes this overstate the run's — which is the safe direction for
// the question it exists to answer.
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
		if hiddenBeforeShaping(r) {
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
// has is false when the face has no such table to read, which is the CFF case
// and is answered for the run as a whole rather than per character: a run whose
// extent is known for some of its letters and not for others has no extent this
// can report.
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
	if f.prog == nil || f.prog.GlyphBBox == nil {
		return 0, 0, false
	}
	// The character map rather than GlyphID, which answers with the WinAnsi
	// *code* for a simple face because that is what will be written. What is
	// wanted here is the outline, which is found by index either way.
	gid, ok := f.prog.Cmap[r]
	if !ok || gid <= 0 || gid >= len(f.prog.GlyphBBox) {
		// A character the face does not cover draws the notdef glyph, whose
		// extent is as much a part of the run's as any other's.
		gid = 0
	}
	if gid >= len(f.prog.GlyphBBox) {
		return 0, 0, false
	}
	b := f.prog.GlyphBBox[gid]
	return b[1], b[3], true
}
