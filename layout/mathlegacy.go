package layout

import (
	"sort"
	"strings"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
)

// What MathML 3 draws and MathML Core does not.
//
// MathML Core is the part of MathML a browser lays out, and it left out much
// that MathML 3 documents still write: elements that draw something of their
// own — <menclose>'s box or strike, <mfenced>'s brackets, <mglyph>'s picture,
// the elementary-mathematics layouts of <mstack> and <mlongdiv> — and
// attributes that change how something is laid out: a table's column
// alignment and lines, a fraction's bevel, a script's minimum size, the font
// attributes MathML 2 had before CSS, the named spaces a length could be. Core
// lays each out as if it were not there: an unknown element is a row, an
// attribute it does not define is ignored, a value it does not read is no
// value. So does this engine, as Core says to; and since the author wrote
// something that would have looked different, each is reported, where it is.
//
// Not reported: what MathML 3 defines with no effect on the page — <mo>'s
// fence and separator, <maction>'s actiontype and selection, an annotation's
// encoding — and <none/>, which MathML 3 draws as nothing, as Core's empty
// row is. linethickness is reported where it is read (mathLineThickness).

// mathLegacyElements are MathML 3's elements that draw something Core does
// not, and what is lost.
var mathLegacyElements = map[string]string{
	"menclose":    "its notation (a box, a circle, a strike) is not drawn",
	"mfenced":     "its brackets and separators are not drawn",
	"mglyph":      "its picture is not drawn",
	"mlabeledtr":  "its label is not set apart from the row",
	"maligngroup": "the alignment it marks is not made",
	"malignmark":  "the alignment it marks is not made",
	"mstack":      "its rows are not stacked and aligned",
	"mlongdiv":    "its long division is not drawn",
	"msgroup":     "its rows are not stacked and aligned",
	"msrow":       "its rows are not stacked and aligned",
	"msline":      "its line is not drawn",
	"mscarries":   "its carries are not drawn",
	"mscarry":     "its carries are not drawn",
}

// mathLegacyAttrs are MathML 3's (and MathML 2's) attributes that change the
// layout, by element, "*" being every element. Core ignores each.
var mathLegacyAttrs = map[string][]string{
	"*": {"fontfamily", "fontweight", "fontstyle", "fontsize", "color", "background",
		"linebreak", "lineleading", "linebreakstyle", "linebreakmultchar",
		"indentalign", "indentshift", "indenttarget", "indentalignfirst",
		"indentshiftfirst", "indentalignlast", "indentshiftlast"},
	"mtable": {"align", "rowalign", "columnalign", "groupalign", "alignmentscope",
		"columnwidth", "width", "rowspacing", "columnspacing", "rowlines",
		"columnlines", "frame", "framespacing", "equalrows", "equalcolumns",
		"side", "minlabelspacing"},
	"mtr":           {"rowalign", "columnalign", "groupalign"},
	"mtd":           {"rowalign", "columnalign", "groupalign"},
	"mfrac":         {"numalign", "denomalign", "bevelled"},
	"munder":        {"align"},
	"mover":         {"align"},
	"munderover":    {"align"},
	"mstyle":        {"scriptminsize", "scriptsizemultiplier", "infixlinebreakstyle", "decimalpoint"},
	"mo":            {"accent"},
	"ms":            {"lquote", "rquote"},
	"msub":          {"subscriptshift"},
	"msup":          {"superscriptshift"},
	"msubsup":       {"subscriptshift", "superscriptshift"},
	"mmultiscripts": {"subscriptshift", "superscriptshift"},
}

// mathLengthAttrs are the attributes Core reads as a <length-percentage>, by
// element — each of which MathML 3 let take values Core does not read.
var mathLengthAttrs = map[string][]string{
	"mspace":  {"width", "height", "depth"},
	"mpadded": {"width", "height", "depth", "lspace", "voffset"},
	"mo":      {"lspace", "rspace", "minsize", "maxsize"},
}

// mathLegacySpaces are MathML 3's named spaces.
var mathLegacySpaces = setOfNames(
	"veryverythinmathspace", "verythinmathspace", "thinmathspace", "mediummathspace",
	"thickmathspace", "verythickmathspace", "veryverythickmathspace",
	"negativeveryverythinmathspace", "negativeverythinmathspace", "negativethinmathspace",
	"negativemediummathspace", "negativethickmathspace", "negativeverythickmathspace",
	"negativeveryverythickmathspace")

// mathLegacyLength says how a length attribute's value is MathML 3's and not
// what Core reads, or "" where it is not one of MathML 3's forms: a named
// space, infinity, a length in a pseudo-unit of the content's own size, or a
// number with no unit — each of which Core does not read, and so does not
// apply — or, on <mpadded>, an increment — "+2px" is the content's size and
// two pixels to MathML 3, and two pixels to Core, which reads the "+" as a
// sign.
func mathLegacyLength(v string, increments bool) string {
	v = ascii.Lower(ascii.TrimCSSSpace(v))
	switch {
	case mathLegacySpaces[v]:
		return "a named space, MathML 3's, which MathML Core does not read, so the attribute is not applied"
	case v == "infinity":
		return "MathML 3's infinity, which MathML Core does not read, so the attribute is not applied"
	}
	for _, unit := range []string{"width", "height", "depth", "lspace"} {
		if strings.HasSuffix(v, unit) && len(v) > len(unit) {
			return "a length in MathML 3's pseudo-unit of the content's own " + unit +
				", which MathML Core does not read, so the attribute is not applied"
		}
	}
	if mathLegacyThickness(v) && strings.Trim(v, "+-.0") != "" {
		return "a number with no unit, MathML 3's, which MathML Core does not read, so the attribute is not applied"
	}
	if increments && v != "" && (v[0] == '+' || v[0] == '-') {
		return "an increment on the content's own size to MathML 3, and MathML Core reads its sign as a sign: " +
			"the size is the length alone"
	}
	return ""
}

// reportMathLegacy says what of MathML 3 an element uses that Core does not
// draw. The box builder calls it once for each MathML element it lays out.
func (b *boxBuilder) reportMathLegacy(n *html.Node) {
	report := func(rule Rule, property, message string) {
		b.rec.ReportDetail(Finding{Rule: rule, Source: sourceOf(n), Message: message, Path: PathOf(n),
			Property: property})
	}
	if lost, ok := mathLegacyElements[n.Name]; ok {
		report(RuleUnsupportedElement, n.Name, "<"+n.Name+"> is MathML 3's and not MathML Core's, "+
			"which lays it out as a row: "+lost)
	}
	var names []string
	for _, a := range n.Attrs {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	legacy := func(name string) bool {
		for _, list := range [][]string{mathLegacyAttrs["*"], mathLegacyAttrs[n.Name]} {
			for _, l := range list {
				if l == name {
					return true
				}
			}
		}
		return false
	}
	for _, name := range names {
		v, _ := n.Attr(name)
		if legacy(name) {
			report(RuleUnsupportedValue, name, name+"="+quoteValue(v)+" on <"+n.Name+
				"> is MathML 3's, which MathML Core does not have, and changes nothing here")
			continue
		}
		switch name {
		case "mathvariant":
			// §3.2.2: normal on an <mi> is Core's; every other value is not
			// applied, and the letters are drawn as written — a bold or a
			// double-struck letter is its own character in Core.
			if !ascii.EqualFold(ascii.TrimCSSSpace(v), "normal") && !mathSameAsAutomatic(n, v) {
				report(RuleUnsupportedValue, name, "mathvariant="+quoteValue(v)+" on <"+n.Name+
					"> is MathML 3's: MathML Core applies only mathvariant=\"normal\" on an <mi>, "+
					"so the text is drawn as written")
			}
		case "mathsize":
			switch ascii.Lower(ascii.TrimCSSSpace(v)) {
			case "small", "normal", "big":
				report(RuleUnsupportedValue, name, "mathsize="+quoteValue(v)+" on <"+n.Name+
					"> is MathML 3's, which MathML Core does not have, and changes nothing here")
			}
		}
		for _, l := range mathLengthAttrs[n.Name] {
			if l != name {
				continue
			}
			if why := mathLegacyLength(v, n.Name == "mpadded" && name != "voffset"); why != "" {
				report(RuleUnsupportedValue, name, name+"="+quoteValue(v)+" on <"+n.Name+"> is "+why)
			}
		}
	}
}

// mathSameAsAutomatic reports whether a mathvariant is the one Core gives the
// element anyway: italic on an <mi> whose every text is one character, which
// text-transform: math-auto sets in italic — it applies to each text node of
// a single character (§4.2), the <mi>'s own and those of anything inside it.
func mathSameAsAutomatic(n *html.Node, v string) bool {
	if n.Name != "mi" || !ascii.EqualFold(ascii.TrimCSSSpace(v), "italic") {
		return false
	}
	var single func(*html.Node) bool
	single = func(e *html.Node) bool {
		for _, c := range e.Children {
			switch c.Type {
			case html.TextNode:
				if t := strings.Trim(c.Text, " \t\n\f\r"); t != "" && len([]rune(t)) != 1 {
					return false
				}
			case html.ElementNode:
				if !single(c) {
					return false
				}
			}
		}
		return true
	}
	return single(n)
}
