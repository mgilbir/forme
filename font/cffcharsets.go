package font

import "sync"

// The two predefined charsets a CFF font may name instead of carrying one:
// Expert (id 1) and Expert Subset (id 2), from TN #5176 Appendix C. Id 0,
// ISOAdobe, is the identity — SID n for GID n — and needs no table.
//
// A charset is what gives a non-CID CFF glyph its name, and a name is what
// GlyphNames and WidthByName are keyed by. Reading these two as the identity, as
// this did, does not make a font's glyphs *unknown*: it makes them known
// wrongly. GID 2 of an Expert font is "exclamsmall" and the identity calls it
// "quotedbl", so the width recorded under "quotedbl" belongs to a small-cap
// exclamation mark, and a consumer checking a PDF's declared widths against the
// font's own compares two unrelated glyphs and believes the answer.
//
// # Why names and not SIDs
//
// The tables are the glyph names, and the SIDs are resolved from
// cffStandardStrings at first use. The SIDs are what the parser wants and the
// names are what a reader can check against the specification, and a table of
// 166 opaque numbers is one transcription error away from the fault above with
// nothing to catch it. Resolving them through the module's own standard strings
// also means the two cannot disagree: a name that is not in that table is a
// build the tests fail rather than a zero quietly written into a charset.

var cffExpertCharsetNames = []string{
	".notdef", "space", "exclamsmall", "Hungarumlautsmall", "dollaroldstyle",
	"dollarsuperior", "ampersandsmall", "Acutesmall", "parenleftsuperior",
	"parenrightsuperior", "twodotenleader", "onedotenleader", "comma", "hyphen",
	"period", "fraction", "zerooldstyle", "oneoldstyle", "twooldstyle",
	"threeoldstyle", "fouroldstyle", "fiveoldstyle", "sixoldstyle",
	"sevenoldstyle", "eightoldstyle", "nineoldstyle", "colon", "semicolon",
	"commasuperior", "threequartersemdash", "periodsuperior", "questionsmall",
	"asuperior", "bsuperior", "centsuperior", "dsuperior", "esuperior",
	"isuperior", "lsuperior", "msuperior", "nsuperior", "osuperior",
	"rsuperior", "ssuperior", "tsuperior", "ff", "fi", "fl", "ffi", "ffl",
	"parenleftinferior", "parenrightinferior", "Circumflexsmall",
	"hyphensuperior", "Gravesmall", "Asmall", "Bsmall", "Csmall", "Dsmall",
	"Esmall", "Fsmall", "Gsmall", "Hsmall", "Ismall", "Jsmall", "Ksmall",
	"Lsmall", "Msmall", "Nsmall", "Osmall", "Psmall", "Qsmall", "Rsmall",
	"Ssmall", "Tsmall", "Usmall", "Vsmall", "Wsmall", "Xsmall", "Ysmall",
	"Zsmall", "colonmonetary", "onefitted", "rupiah", "Tildesmall",
	"exclamdownsmall", "centoldstyle", "Lslashsmall", "Scaronsmall",
	"Zcaronsmall", "Dieresissmall", "Brevesmall", "Caronsmall",
	"Dotaccentsmall", "Macronsmall", "figuredash", "hypheninferior",
	"Ogoneksmall", "Ringsmall", "Cedillasmall", "onequarter", "onehalf",
	"threequarters", "questiondownsmall", "oneeighth", "threeeighths",
	"fiveeighths", "seveneighths", "onethird", "twothirds", "zerosuperior",
	"onesuperior", "twosuperior", "threesuperior", "foursuperior",
	"fivesuperior", "sixsuperior", "sevensuperior", "eightsuperior",
	"ninesuperior", "zeroinferior", "oneinferior", "twoinferior",
	"threeinferior", "fourinferior", "fiveinferior", "sixinferior",
	"seveninferior", "eightinferior", "nineinferior", "centinferior",
	"dollarinferior", "periodinferior", "commainferior", "Agravesmall",
	"Aacutesmall", "Acircumflexsmall", "Atildesmall", "Adieresissmall",
	"Aringsmall", "AEsmall", "Ccedillasmall", "Egravesmall", "Eacutesmall",
	"Ecircumflexsmall", "Edieresissmall", "Igravesmall", "Iacutesmall",
	"Icircumflexsmall", "Idieresissmall", "Ethsmall", "Ntildesmall",
	"Ogravesmall", "Oacutesmall", "Ocircumflexsmall", "Otildesmall",
	"Odieresissmall", "OEsmall", "Oslashsmall", "Ugravesmall", "Uacutesmall",
	"Ucircumflexsmall", "Udieresissmall", "Yacutesmall", "Thornsmall",
	"Ydieresissmall",
}

var cffExpertSubsetCharsetNames = []string{
	".notdef", "space", "dollaroldstyle", "dollarsuperior", "parenleftsuperior",
	"parenrightsuperior", "twodotenleader", "onedotenleader", "comma", "hyphen",
	"period", "fraction", "zerooldstyle", "oneoldstyle", "twooldstyle",
	"threeoldstyle", "fouroldstyle", "fiveoldstyle", "sixoldstyle",
	"sevenoldstyle", "eightoldstyle", "nineoldstyle", "colon", "semicolon",
	"commasuperior", "threequartersemdash", "periodsuperior", "asuperior",
	"bsuperior", "centsuperior", "dsuperior", "esuperior", "isuperior",
	"lsuperior", "msuperior", "nsuperior", "osuperior", "rsuperior",
	"ssuperior", "tsuperior", "ff", "fi", "fl", "ffi", "ffl",
	"parenleftinferior", "parenrightinferior", "hyphensuperior",
	"colonmonetary", "onefitted", "rupiah", "centoldstyle", "figuredash",
	"hypheninferior", "onequarter", "onehalf", "threequarters", "oneeighth",
	"threeeighths", "fiveeighths", "seveneighths", "onethird", "twothirds",
	"zerosuperior", "onesuperior", "twosuperior", "threesuperior",
	"foursuperior", "fivesuperior", "sixsuperior", "sevensuperior",
	"eightsuperior", "ninesuperior", "zeroinferior", "oneinferior",
	"twoinferior", "threeinferior", "fourinferior", "fiveinferior",
	"sixinferior", "seveninferior", "eightinferior", "nineinferior",
	"centinferior", "dollarinferior", "periodinferior", "commainferior",
}

var (
	predefinedCharsetsOnce sync.Once
	expertCharset          []int
	expertSubsetCharset    []int
)

// cffPredefinedCharset returns the GID→SID table a predefined charset id names,
// and whether the id is one of the three.
//
// ISOAdobe comes back nil with ok true: it is the identity, and building 229
// entries to say so would invite the caller to treat "no table" as "no charset".
// The caller reads a nil table as the identity, which is what it is.
func cffPredefinedCharset(id int) (sids []int, ok bool) {
	switch id {
	case 0:
		return nil, true
	case 1:
		predefinedCharsetsOnce.Do(resolvePredefinedCharsets)
		return expertCharset, true
	case 2:
		predefinedCharsetsOnce.Do(resolvePredefinedCharsets)
		return expertSubsetCharset, true
	}
	return nil, false
}

// resolvePredefinedCharsets turns the two name tables into SIDs, once.
//
// A name absent from cffStandardStrings cannot happen — a predefined charset
// names only predefined strings, which is the whole of what makes it predefined
// — and TestThePredefinedCharsetsResolve is what says so. It resolves to SID 0
// here rather than panicking, because a font reader that stops the program over
// its own table is worse than one that names a glyph ".notdef".
func resolvePredefinedCharsets() {
	sid := make(map[string]int, len(cffStandardStrings))
	for i, s := range cffStandardStrings {
		sid[s] = i
	}
	resolve := func(names []string) []int {
		out := make([]int, len(names))
		for i, n := range names {
			out[i] = sid[n]
		}
		return out
	}
	expertCharset = resolve(cffExpertCharsetNames)
	expertSubsetCharset = resolve(cffExpertSubsetCharsetNames)
}

// isoAdobeCharsetLen is how many glyphs the ISOAdobe charset names: SIDs 0
// through 228, which is every standard string that is a glyph name rather than
// one of the expert or notational ones after it.
//
// It matters because the identity has no end of its own. A font declaring
// charset 0 and carrying more glyphs than this has named none of the extras, and
// continuing the identity past here hands them the names of standard strings
// they have nothing to do with.
const isoAdobeCharsetLen = 229
