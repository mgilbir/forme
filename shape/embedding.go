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
	XHeight int

	// UnderlinePosition and UnderlineThickness are post's, in font units, with
	// the position the distance from the baseline to the *top* of the stroke and
	// so negative for a rule drawn below it. StrikeoutPosition and StrikeoutSize
	// are OS/2's equivalent for a line through the middle.
	UnderlinePosition, UnderlineThickness int
	StrikeoutPosition, StrikeoutSize      int

	// Weight is OS/2 usWeightClass: 100 for Thin, 400 for Regular, 700 for Bold.
	//
	// It is worth more than it looks on a variable font. This module hands back
	// the outlines as they are stored, which is the face's default instance, and
	// a quarter of the variable faces published under the OFL default to
	// something lighter than Regular. The name is no guide: the legacy name
	// records can spell only four styles, so a face whose default is Thin is
	// commonly still called Regular there, and several are. This is the number
	// that says what was actually drawn.
	Weight int

	// Declared is the set of the above the font actually states.
	//
	// Zero and unknown are different answers and a consumer has to tell them
	// apart: a font may legitimately declare a line gap of nothing, and the
	// fourteen standard faces declare none of this at all because they have no
	// hhea, OS/2 or post table to declare it in. Every field above that can be
	// absent has a bit here, and the bit is the only way to know.
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
