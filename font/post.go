package font

// macStandardNames is the 258 glyph names of the standard Macintosh ordering,
// which a post table names glyphs by: all of them in order, for version 1.0;
// by index, for the indices of a version 2.0 table below 258; and by offset
// from the glyph's own index, for version 2.5. Apple's TrueType Reference
// Manual, chapter 6, "post", lists them.
//
// It is an array and not a slice so that the compiler counts it: an entry
// dropped or inserted would shift every name after it, and name glyph 36 "B" in
// every font that calls it "A".
var macStandardNames = [258]string{
	".notdef", ".null", "nonmarkingreturn", "space", "exclam", "quotedbl",
	"numbersign", "dollar", "percent", "ampersand", "quotesingle", "parenleft",
	"parenright", "asterisk", "plus", "comma", "hyphen", "period", "slash", "zero",
	"one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
	"colon", "semicolon", "less", "equal", "greater", "question", "at", "A", "B",
	"C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R",
	"S", "T", "U", "V", "W", "X", "Y", "Z", "bracketleft", "backslash",
	"bracketright", "asciicircum", "underscore", "grave", "a", "b", "c", "d", "e",
	"f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u",
	"v", "w", "x", "y", "z", "braceleft", "bar", "braceright", "asciitilde",
	"Adieresis", "Aring", "Ccedilla", "Eacute", "Ntilde", "Odieresis", "Udieresis",
	"aacute", "agrave", "acircumflex", "adieresis", "atilde", "aring", "ccedilla",
	"eacute", "egrave", "ecircumflex", "edieresis", "iacute", "igrave",
	"icircumflex", "idieresis", "ntilde", "oacute", "ograve", "ocircumflex",
	"odieresis", "otilde", "uacute", "ugrave", "ucircumflex", "udieresis",
	"dagger", "degree", "cent", "sterling", "section", "bullet", "paragraph",
	"germandbls", "registered", "copyright", "trademark", "acute", "dieresis",
	"notequal", "AE", "Oslash", "infinity", "plusminus", "lessequal",
	"greaterequal", "yen", "mu", "partialdiff", "summation", "product", "pi",
	"integral", "ordfeminine", "ordmasculine", "Omega", "ae", "oslash",
	"questiondown", "exclamdown", "logicalnot", "radical", "florin", "approxequal",
	"Delta", "guillemotleft", "guillemotright", "ellipsis", "nonbreakingspace",
	"Agrave", "Atilde", "Otilde", "OE", "oe", "endash", "emdash", "quotedblleft",
	"quotedblright", "quoteleft", "quoteright", "divide", "lozenge", "ydieresis",
	"Ydieresis", "fraction", "currency", "guilsinglleft", "guilsinglright", "fi",
	"fl", "daggerdbl", "periodcentered", "quotesinglbase", "quotedblbase",
	"perthousand", "Acircumflex", "Ecircumflex", "Aacute", "Edieresis", "Egrave",
	"Iacute", "Icircumflex", "Idieresis", "Igrave", "Oacute", "Ocircumflex",
	"apple", "Ograve", "Uacute", "Ucircumflex", "Ugrave", "dotlessi", "circumflex",
	"tilde", "macron", "breve", "dotaccent", "ring", "cedilla", "hungarumlaut",
	"ogonek", "caron", "Lslash", "lslash", "Scaron", "scaron", "Zcaron", "zcaron",
	"brokenbar", "Eth", "eth", "Yacute", "yacute", "Thorn", "thorn", "minus",
	"multiply", "onesuperior", "twosuperior", "threesuperior", "onehalf",
	"onequarter", "threequarters", "franc", "Gbreve", "gbreve", "Idotaccent",
	"Scedilla", "scedilla", "Cacute", "cacute", "Ccaron", "ccaron", "dcroat",
}

// readPost reads the glyph names a post table gives, into GlyphNames and
// GlyphByName and, where the widths were read, WidthByName.
//
// # The versions
//
// Version 1.0 names the first 258 glyphs in the standard order and no others.
// Version 2.0 gives each glyph an index: below 258 it is a standard name, and
// from 258 on it selects one of the Pascal strings that follow the index array,
// in the order they are written. Version 2.5 gives each glyph a signed byte to
// add to its own index, and the result is a standard name; the format is
// deprecated, and still in fonts. Version 3.0 says the font names no glyphs,
// and version 4.0 is Apple's composite-font form, which maps glyphs to
// character codes rather than names; both leave the fields nil, which is what
// any other version does too.
//
// # What a malformed table gets
//
// The post table's glyph count and maxp's are two statements of one number,
// and a font can make them disagree. maxp's is the one the rest of the font is
// read by, so it says which glyphs exist and a name for a glyph past it names
// nothing; post's says where its own index array ends, so it is the one that
// finds the strings. That is how HarfBuzz reads it. A glyph past post's count
// and within maxp's is a glyph the table does not name.
//
// An index array the table does not hold is not partly read: the strings are
// found only after it, so a table cut inside it carries no names at all, and
// HarfBuzz refuses it whole too. A string whose length runs past the end of the
// table is not there, and neither is any string after it, since each is found
// by stepping over the one before; an index selecting a string that is not
// there leaves its glyph unnamed. So does an empty string, as an unnamed glyph
// is left out of the CFF and Type 1 names: it is not a glyph called "".
//
// # Two glyphs, one name
//
// The format does not stop a font naming two glyphs alike, and a font that does
// has to answer "which glyph is X" somehow. The lower index answers, as it does
// in FreeType's FT_Get_Name_Index, which walks the glyphs from zero and returns
// the first match. WidthByName is that glyph's width, so the two maps describe
// the same glyph.
//
// # The cost
//
// Every glyph is visited once and every string stepped over once, so the work
// is bounded by the table's size and the font's glyph count, and nothing in the
// table can make it repeat. The Budget is not charged, as the single pass over
// hmtx is not.
func readPost(fp *Program, post []byte) {
	if len(post) < 32 {
		return
	}
	var nameOf func(gid int) string
	count := 0 // the glyphs the table speaks for
	switch Be32(post, 0) {
	case 0x00010000:
		count = len(macStandardNames)
		nameOf = func(gid int) string { return macStandardNames[gid] }
	case 0x00020000:
		if len(post) < 34 {
			return
		}
		count = Be16(post, 32)
		stringsAt := 34 + 2*count
		if stringsAt > len(post) {
			return
		}
		// Where each string's bytes are, in order, found by stepping over the
		// length bytes. The slice grows with the strings there are, which the
		// table's own length bounds, and not with any count the table states.
		var names [][2]int
		for p := stringsAt; p < len(post); {
			n := int(post[p])
			if p+1+n > len(post) {
				break
			}
			names = append(names, [2]int{p + 1, p + 1 + n})
			p += 1 + n
		}
		nameOf = func(gid int) string {
			i := Be16(post, 34+2*gid)
			if i < len(macStandardNames) {
				return macStandardNames[i]
			}
			if i -= len(macStandardNames); i < len(names) {
				return string(post[names[i][0]:names[i][1]])
			}
			return ""
		}
	case 0x00025000:
		if len(post) < 34 {
			return
		}
		count = Be16(post, 32)
		if 34+count > len(post) {
			return
		}
		nameOf = func(gid int) string {
			i := gid + int(int8(post[34+gid]))
			if i < 0 || i >= len(macStandardNames) {
				return ""
			}
			return macStandardNames[i]
		}
	default:
		return
	}
	count = min(count, fp.NumGlyphs)

	fp.GlyphNames = make(map[string]bool, count)
	fp.GlyphByName = make(map[string]int, count)
	if fp.WidthByGID != nil {
		fp.WidthByName = make(map[string]float64, count)
	}
	for gid := range count {
		name := nameOf(gid)
		if name == "" {
			continue
		}
		if _, seen := fp.GlyphByName[name]; seen {
			continue
		}
		fp.GlyphNames[name] = true
		fp.GlyphByName[name] = gid
		if fp.WidthByName != nil {
			fp.WidthByName[name] = fp.WidthByGID[gid]
		}
	}
}
