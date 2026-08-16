package shape

// What a document format needs in order to embed a face.
//
// A shaper decides where glyphs go; a format that carries the text has to
// describe the font as well, so that a reader without it can still lay the page
// out. PDF wants a FontDescriptor, a width for every glyph, a mapping back to
// characters and the subsetted program itself. Other formats want the same
// facts under other names.
//
// So what is here is the facts, in the font's own units, and not any format's
// encoding of them. Packing flags into bits, scaling to a thousandth of an em,
// building a CMap — those belong to whoever is writing the file.

// Descriptor is a face's own metrics: what a reader needs to set the text when
// the font is not available to it.
//
// Lengths are in font units. Divide by UnitsPerEm for fractions of an em, which
// is what most formats state them in.
type Descriptor struct {
	// Ascent and Descent are the font's own, from hhea. Descent is negative.
	Ascent, Descent int
	// CapHeight is the height of a capital letter, from OS/2, and is zero for a
	// font that declares none.
	CapHeight int
	// BBox is the box enclosing every glyph: xMin, yMin, xMax, yMax.
	BBox [4]int
	// ItalicAngle is degrees clockwise from vertical, so an italic is negative.
	ItalicAngle float64
	// StemV is the width of a vertical stem, estimated from the weight the font
	// declares because it cannot be measured without rasterising: no table
	// states it, and the outlines that would show it are the thing being
	// described.
	StemV int
	// Flags is the bit set PDF's FontDescriptor uses — fixed pitch, serif,
	// symbolic and the rest. It is here because it is derived from the font and
	// nowhere else, and a caller that wants the bits individually can take them
	// apart more easily than it can work them out.
	Flags int

	// LineGap is the leading hhea asks for between one line's descent and the
	// next line's ascent. Ascent + Descent alone is not the height of a line:
	// CSS calls the third term line-height: normal and every browser takes it
	// from the font, so a formula without it is not a stricter reading of the
	// face but the right formula with a term missing.
	LineGap int

	// TypoAscent, TypoDescent and TypoLineGap are OS/2's sTypo* trio, which many
	// fonts disagree with hhea about. UseTypoMetrics carries OS/2 fsSelection
	// bit 7, which is the font saying which of the two it means. A consumer that
	// honours it is following the font's own instruction rather than guessing;
	// one that does not should stay with the hhea three above, which is what
	// this module's Ascent and Descent are.
	TypoAscent, TypoDescent, TypoLineGap int
	UseTypoMetrics                       bool

	// XHeight is OS/2 sxHeight, the height of a lowercase x. CSS's ex unit is
	// defined against it, and vertical-align: middle against half of it; the
	// half-em both fall back to is the specified fallback and not the answer.
	// For a standard face it is the AFM's XHeight, which twelve of the fourteen
	// publish — Symbol and ZapfDingbats have no lowercase to measure.
	XHeight int

	// UnderlinePosition and UnderlineThickness are post's, in font units, with
	// the position the distance from the baseline to the *top* of the stroke and
	// so negative for a rule drawn below it. StrikeoutPosition and StrikeoutSize
	// are OS/2's equivalent for a line through the middle.
	//
	// A standard face's underline comes from its AFM, converted: PostScript
	// measures to the centre of the stroke and post to its top, so the position
	// reported is half a thickness above the number Adobe published. The field
	// means one thing whichever kind of face answered it. An AFM carries no
	// strikeout, so the fourteen state none.
	UnderlinePosition, UnderlineThickness int
	StrikeoutPosition, StrikeoutSize      int

	// Weight is OS/2 usWeightClass: 100 for Thin, 400 for Regular, 700 for Bold.
	//
	// It is worth more than it looks on a variable font. Load hands back the
	// outlines as they are stored, which is the face's default instance, and a
	// quarter of the variable faces published under the OFL default to something
	// lighter than Regular. The name is no guide: the legacy name records can
	// spell only four styles, so a face whose default is Thin is commonly still
	// called Regular there, and several are. This is the number that says what
	// was actually drawn — LoadInstance rewrites it from the location it cut the
	// face at, so it says that for an instance too.
	Weight int

	// Declared is the set of the above the font actually states.
	//
	// Zero and unknown are different answers and a consumer has to tell them
	// apart: a font may legitimately declare a line gap of nothing, and the
	// fourteen standard faces have no hhea, OS/2 or post table to declare one
	// in — so they state no line gap, no strikeout and no typographic trio, and
	// only the x-height and underline their AFM publishes. Every field above
	// that can be absent has a bit here, and the bit is the only way to know.
	Declared Metric
}

// Metric names a metric a font may or may not state, for Descriptor.Declared.
type Metric uint32

const (
	MetricLineGap Metric = 1 << iota
	MetricTypoMetrics
	MetricXHeight
	MetricCapHeight
	MetricUnderline
	MetricStrikeout
	MetricWeight
)

// Has reports whether the font stated a metric, as against leaving it zero.
func (d Descriptor) Has(m Metric) bool { return d.Declared&m == m }

// Descriptor returns the face's metrics.
func (f *Face) Descriptor() Descriptor {
	return Descriptor{
		Ascent:      f.ascent,
		Descent:     f.descent,
		CapHeight:   f.capHeight,
		BBox:        f.bbox,
		ItalicAngle: f.italic,
		StemV:       f.stemV,
		Flags:       f.flags,

		LineGap:            f.lineGap,
		TypoAscent:         f.typoAscent,
		TypoDescent:        f.typoDescent,
		TypoLineGap:        f.typoLineGap,
		UseTypoMetrics:     f.useTypoMetrics,
		XHeight:            f.xHeight,
		UnderlinePosition:  f.underlinePos,
		UnderlineThickness: f.underlineThick,
		StrikeoutPosition:  f.strikeoutPos,
		StrikeoutSize:      f.strikeoutSize,
		Weight:             f.weight,
		Declared:           f.declared,
	}
}

// GlyphAdvance is how far the pen moves after a glyph, in font units.
//
// It is the font's own advance and not the one shaping decided: a kern or a
// mark attachment changes what a *run* does without changing what the glyph
// says about itself, and a width table describes the glyph.
func (f *Face) GlyphAdvance(gid int) float64 { return f.advanceGID(gid) }

// GlyphAdvances is the advance of every glyph, indexed by glyph id.
//
// The whole table at once, because a format that states widths states them for
// a range and has to see which are alike.
func (f *Face) GlyphAdvances() []float64 {
	if f.prog == nil {
		return nil
	}
	return append([]float64(nil), f.prog.WidthByGID...)
}

// IsCFF reports whether the outlines are CFF rather than glyf, which decides
// how a format has to carry the program and what it may say about it.
func (f *Face) IsCFF() bool { return f.cff }

// CharacterCollection is the collection this face's CIDs are numbered in — the
// CFF's ROS — and whether it has one to state.
//
// A PDF must say it. ISO 32000-2 9.7.4.2 requires a CIDFont's /CIDSystemInfo to
// be compatible with the character collection of its glyph source, so a
// document declaring Adobe-Identity-0 over an Adobe-Japan1 font is making a
// false statement about its own numbering. The three values go together and are
// only meaningful together, which is why they are returned that way.
//
// # What ok means, and why it is one answer and not three
//
// A caller reading this out of the font program itself has three separate ways
// to end up writing nonsense, and has to remember all of them: the face may have
// no CFF at all, so there is no collection and none may be written; it may have
// one that is not CID-keyed, where the glyph index is the only numbering there
// is; or it may be CID-keyed and have failed to say — a ROS naming strings the
// font does not carry, or a supplement below zero, which is a version number and
// counts up. The last is the dangerous one, because half of it parses, and half
// a collection is the shape that goes into a document unnoticed.
//
// ok is false for all three. A caller that has to write a /CIDSystemInfo and
// gets false should refuse to embed the face rather than reach for a default:
// Adobe-Identity-0 is not a safe fallback, it is a specific claim, and it is
// wrong for exactly the fonts this distinguishes.
//
// The values come from the parse Load already did, so this costs nothing and
// cannot disagree with the CIDs Encode writes. It describes the program the
// face will embed: subsetting carries the ROS and the string INDEX through
// untouched, so the subset states the same collection.
func (f *Face) CharacterCollection() (registry, ordering string, supplement int, ok bool) {
	// gidToCID rather than IsCFF: it is the field that says the outlines are
	// CID-keyed, which is a narrower thing than being CFF, and it is what Encode
	// consults to decide whether the codes it writes are CIDs at all. A
	// collection describing a numbering Encode does not use is worse than none.
	if f.gidToCID == nil || f.registry == "" || f.ordering == "" {
		return "", "", 0, false
	}
	return f.registry, f.ordering, f.supplement, true
}

// SubsetGlyphs is Subset, also reporting which glyphs the subset kept.
//
// A format that names the subset needs them: PDF writes a tag derived from the
// set so that two subsets of one face can be told apart, and a bitmap of it so
// a reader can check the program against what the file claims. Both have to be
// computed from what the subsetter actually kept rather than from what was
// asked for, since keeping one glyph can require keeping another.
func (f *Face) SubsetGlyphs() (program []byte, kept []int, err error) {
	return f.subset()
}

// Cmap is the face's character-to-glyph mapping, copied.
//
// It is the way back: a format that wants the text extractable has to say which
// character each glyph came from, and shaping has long since stopped tracking
// that. A glyph reachable from several characters appears several times, and
// choosing between them is the caller's — the choices are not equivalent and
// depend on what the mapping is for.
func (f *Face) Cmap() map[rune]int {
	if f.prog == nil {
		return nil
	}
	out := make(map[rune]int, len(f.prog.Cmap))
	for r, gid := range f.prog.Cmap {
		out[r] = gid
	}
	return out
}
