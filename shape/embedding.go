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
// Lengths are in font units — the face's own grid, whose size is Face.UnitsPerEm.
// Divide by it for fractions of an em, which is what most formats state them in.
// Face.GlyphAdvance is *not* in these units: widths arrive already scaled to a
// thousandth of an em, which is the one place the two grids part company.
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
	MetricItalicAngle
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

// GlyphAdvance is how far the pen moves after a glyph, in thousandths of an em.
//
// Not font units, and not the unit Descriptor's lengths are in — see the note on
// advanceGID. A thousandth of an em is what the formats that state a width table
// state it in, which is why the scaling happens once on the way in rather than
// at each of them.
//
// It is the font's own advance and not the one shaping decided: a kern or a
// mark attachment changes what a *run* does without changing what the glyph
// says about itself, and a width table describes the glyph.
func (f *Face) GlyphAdvance(gid int) float64 { return f.advanceGID(gid) }

// GlyphAdvances is the advance of every glyph, indexed by glyph id, in
// thousandths of an em.
//
// The whole table at once, because a format that states widths states them for
// a range and has to see which are alike.
func (f *Face) GlyphAdvances() []float64 {
	if f.prog == nil {
		return nil
	}
	return append([]float64(nil), f.prog.WidthByGID...)
}

// GlyphCode is the number the embedded font addresses a glyph by — the number
// to key a width table on, to write into a /CIDSet, and to put in a content
// stream.
//
// For everything but a CID-keyed CFF it is the glyph index, which is what
// Identity-H means and why the two words are used interchangeably about most
// faces. A CIDFontType0 is addressed by CID instead: the code goes into the
// font's charset and comes out as a glyph index, so writing the glyph index
// there sends the reader to whatever glyph happens to carry that number.
//
// # Why this and not the mapping
//
// The two numberings agree for a font whose charset is the identity, and many
// are — so a caller that reaches for a CID only when it remembers to gets a
// document that is right for most faces and wrong for the CJK ones, which is
// the worst way for this to be wrong. There is no branch here to forget: the
// answer is already the right number for the face in hand, and a caller that
// uses it everywhere a glyph is named is correct for every kind of face.
//
// It is what Encode writes, so a width table keyed on it agrees with the codes
// in the content stream by construction rather than by both being derived
// correctly. Out-of-range and negative glyph ids come back unchanged, because a
// glyph the face does not have has no code and inventing one would hide the
// caller's error rather than the font's.
func (f *Face) GlyphCode(gid int) int { return f.codeForGID(gid) }

// IsCFF reports whether the outlines are CFF rather than glyf, which decides
// how a format has to carry the program and what it may say about it.
func (f *Face) IsCFF() bool { return f.cff }

// IsCIDKeyed reports whether the outlines are a CID-keyed CFF: whether the
// codes Encode writes, and GlyphCode returns, are CIDs rather than glyph
// indices.
//
// It answers the one question CharacterCollection's ok deliberately does not.
// ok is false for a face with no CFF, for a CFF that is not CID-keyed, and for
// a CID-keyed CFF whose ROS is unusable, and a caller that only has to write a
// /CIDSystemInfo needs no more than that. A caller deciding how to embed the
// face does: a face that is not CID-keyed is embedded addressed by its glyph
// indices, while a CID-keyed one whose collection cannot be stated has to be
// refused, because its codes are CIDs in a numbering nothing can name. So:
//
//   - IsCIDKeyed false: the codes are glyph indices, and CharacterCollection's
//     ok is false because there is no collection at all.
//   - IsCIDKeyed true and ok true: the codes are CIDs in the collection it
//     names.
//   - IsCIDKeyed true and ok false: the codes are CIDs and the font has not
//     said which collection they are numbered in.
//
// It reads the same field codeForGID and CharacterCollection do, from the
// parse Load already did, so it cannot disagree with the codes Encode writes:
// true here is exactly the case in which those codes are CIDs. Re-reading the
// program to find out would be a second parse that could reach a second
// answer. A face from LoadSimple or Standard is never CID-keyed.
func (f *Face) IsCIDKeyed() bool { return f.gidToCID != nil }

// FSType is a font's OS/2 fsType: what its licence says a document may do with
// it when embedding it. It is the font's own sixteen bits, as the font wrote
// them, reserved bits and all.
//
// The OpenType OS/2 table defines it. Bits 0 to 3 are the usage permission,
// and a value with none of them set is Installable embedding, which permits
// everything. From OS/2 version 3 a font may set at most one of them; versions
// 0 to 2 allowed several, and the specification says to honour the least
// restrictive of those present — Editable over Preview & Print over
// Restricted. Bits 8 and 9 are independent of the usage and restrict how the
// font is embedded rather than whether.
type FSType uint16

const (
	// FSTypeRestricted is Restricted License embedding: the font must not be
	// embedded unless a less restrictive usage bit is also set (see FSType).
	FSTypeRestricted FSType = 0x0002
	// FSTypePreviewPrint is Preview & Print embedding: the font may be
	// embedded, and the document opened read-only.
	FSTypePreviewPrint FSType = 0x0004
	// FSTypeEditable is Editable embedding: the font may be embedded, and the
	// document edited.
	FSTypeEditable FSType = 0x0008
	// FSTypeNoSubsetting says the font must not be subsetted before it is
	// embedded: a document may carry it whole or not at all. Program is the
	// whole of it.
	FSTypeNoSubsetting FSType = 0x0100
	// FSTypeBitmapOnly says only bitmaps the font contains may be embedded,
	// and no outlines. A font carrying no bitmaps may therefore not be
	// embedded at all.
	FSTypeBitmapOnly FSType = 0x0200
)

// EmbeddingPermissions is the face's OS/2 fsType, and whether the font states
// one.
//
// It is read at load, from the program the face was loaded from, so a caller
// embedding a face need not read the OS/2 table again — and a caller handed a
// face rather than bytes has the answer too. fsType is at the same place in
// every version of the table, so a version 0 table states it as a version 5
// one does; the value comes back as the font wrote it, and what the usage bits
// mean together is described at FSType.
//
// stated is false for a face whose program has no OS/2 table, or one too short
// to reach the field, and for a standard face, which has no program. OS/2 is
// required of an OpenType font but optional in a TrueType one, and a font that
// states nothing has placed no restriction; stated is here so that a caller
// that wants to treat the two differently can. The subset carries the OS/2
// table through unchanged, so it states the same permissions.
func (f *Face) EmbeddingPermissions() (fsType FSType, stated bool) {
	return f.fsType, f.fsTypeStated
}

// Program is the whole font program the face was loaded from: what a document
// embeds when it may not subset the face (FSTypeNoSubsetting), or chooses not
// to.
//
// It is an sfnt, TrueType or OpenType, and it is the program the face reads
// its own tables from. A face loaded from a WOFF or WOFF 2 returns the sfnt
// the container held, since that is the program and a document format carries
// the program; a face from LoadInstance returns the instance it cut, which is
// what it draws. A standard face has no program and returns nil. A clone
// returns its face's.
//
// It is not a copy, because a CJK program is megabytes and a document may ask
// for it once per face it embeds. For a face from Load it is the very slice
// Load was given, which Load keeps rather than copies. So it must not be
// modified: the face goes on reading it, and a change shows up as a font that
// says something else.
func (f *Face) Program() []byte { return f.data }

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
// wrong for exactly the fonts this distinguishes. A caller that has to tell the
// second case from the third — to embed a face that is not CID-keyed by its
// glyph indices, and refuse one that is and cannot name its collection — asks
// IsCIDKeyed, which reads the same field.
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
