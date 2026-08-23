package paragraph

import "strings"

// word-space-transform, CSS Text 4: making a break opportunity visible.
//
// A zero width space and a <wbr> mark a place a line may end and put nothing on
// the page. That is what an author of English wants and not what an author of
// Japanese wants: Japanese is written without spaces between words, and a reader
// learning it — or a dictionary, or a children's book — wants the word divisions
// shown. The property turns each of those marks into a space one can see.
//
// It is the whole reason both of its values exist. "space" sets a U+0020, which
// is what a Latin-script document would use; "ideographic-space" sets a U+3000,
// which is the width of one character and is what a Japanese document uses,
// because a Latin space between two ideographs looks like a mistake.

// WordSpaceTransform is what the property sets.
type WordSpaceTransform struct {
	// Separator is what a virtual word separator becomes: "" for none, a space,
	// or an ideographic space. It is the character rather than a keyword because
	// the character is what every later stage needs and there are exactly two.
	Separator string
}

// Transforms reports whether anything is to be done, which is the question every
// caller actually asks.
func (w WordSpaceTransform) Transforms() bool { return w.Separator != "" }

// The two characters, named.
const (
	ordinarySpace    = " "
	ideographicSpace = "　"
)

// WordSpaceTransformOf reads the property, and returns what it could not act on.
//
// The grammar is "none | [ space | ideographic-space ] || auto-phrase", so the
// value is up to two words and auto-phrase may come on either side of the other.
//
// auto-phrase is the half this engine cannot do. It asks for word separators to
// be *invented* at phrase boundaries the author did not mark, which for Japanese
// means a dictionary and a segmentation model — the same thing word-break's own
// auto-phrase asks for and is reported for. What is done here is the rest: a
// document that writes "auto-phrase" beside "space" gets its explicit marks
// expanded and is told the inferred ones are missing, which is a page with too
// few spaces rather than a page with none.
func WordSpaceTransformOf(value string) (WordSpaceTransform, string) {
	var out WordSpaceTransform
	unhandled := ""
	seen := false
	for _, word := range strings.Fields(strings.ToLower(strings.TrimSpace(value))) {
		switch word {
		case "none":
			// Explicit and the initial value both; nothing to record.
			seen = true
		case "space":
			out.Separator, seen = ordinarySpace, true
		case "ideographic-space":
			out.Separator, seen = ideographicSpace, true
		case "auto-phrase":
			unhandled = "auto-phrase"
		default:
			// Not a value of this property. Nothing is done, and nothing is
			// reported either: an unreadable declaration is the cascade's to
			// report and it drops one before it reaches here.
			return WordSpaceTransform{}, ""
		}
	}
	if !seen && unhandled == "" {
		return WordSpaceTransform{}, ""
	}
	return out, unhandled
}

// IsVirtualWordSeparator reports whether a character is one of the marks this
// property expands.
//
// U+200B ZERO WIDTH SPACE is the one a document writes in its text. <wbr> is the
// other and is an element, so it is not a character and cannot be asked here —
// the box that builds it turns it into a U+200B of its own, which is what the
// HTML specification says it is rendered as and is what puts the two on the same
// path through everything below.
func IsVirtualWordSeparator(r rune) bool { return r == 0x200B }
