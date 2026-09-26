// Package font reads font programs: the sfnt container (TrueType and OpenType),
// the CFF outlines an OpenType font may carry, Type 1, and the WOFF 1 and WOFF 2
// wrappers a web font arrives in.
//
// It is what shape stands on. shape.Load unwraps a WOFF with DecodeWOFF, finds
// the tables with SFNTTables, and reads the program with ParseSFNTWithin and,
// for CFF outlines, ParseCFFGlyphs — all under one Budget, so that what a font
// can cost to read is bounded for the font as a whole and not per parser; a
// budget that runs out is an error Load reports, never a quietly smaller font.
// The rest of shape reads the tables SFNTTables finds for itself — the layout
// tables, gvar, the instancer's — with Be16 and Be32; the TrueType subsetter
// takes MarkComposite for a composite glyph's components, and the CFF one's
// seac closure CFFStandardSID, CFFSubrBias and StandardEncodingName. A simple
// font's encoding is WinAnsiEncodingNames, and GlyphNameToRune says what
// character each of its names stands for.
//
// Every byte read here is untrusted: a font arrives with a document, and the
// document's author chose it. So nothing is sized from a count the file states
// before the bytes behind the count are known to be there, every offset is
// checked against the data it points into before it is followed, and work a
// file can make the parser repeat is charged to the Budget.
//
// # What is here that the engine does not use
//
// The package came from a PDF/A validator, which asked questions of an embedded
// font this engine does not: the width of a glyph by name or by CID, which
// glyphs a Type 1 program defines, the Macintosh and symbol cmaps, how many
// cmap subtables a symbolic font has. Program still carries those answers, the
// CFF and Type 1 readers still compute them, and they are tested and fuzzed like
// the rest — so they are held to the same standard as what shape reads, for no
// caller in this engine.
//
// ParseType1 is part of that: reading a font program is this package's scope
// whichever program it is, and a PDF validator reads the Type 1 programs a PDF
// embeds through it. What is PDF's rather than the font's — the code-to-glyph
// rule of ISO 32000-1 9.6.6.4 and the glyph-name repertoires of ISO 19005-1
// 6.3.8 — is not here; a validator owns it.
package font
