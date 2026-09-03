package layout

import (
	"strings"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/paragraph"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What the engine says about text it could not set as written.
//
// A run wider than its box, a word-break or line-break value nothing implements,
// a script this engine does not shape, a character the face has no glyph for.
// None of them stops the page being produced, and all of them are things an
// author would want to know before it is printed — which is the whole argument
// for findings over errors.

// reportOverflow names content too wide for the box holding it.
//
// It is reported once per piece of text rather than once per line, because a
// paragraph containing one impossible word would otherwise complain on every
// line it wraps to.
func (l *layouter) ReportOverflow(item inlineItem, width style.Unit) {
	what := "the text " + quoteValue(item.Text)
	key := item.Text
	if item.Atomic != nil {
		// A replaced element has no text to name it by, and two different
		// images of the same width are two findings rather than one — so the
		// key is where it is in the document rather than what it says.
		what = "the image"
		key = "\x00replaced\x00" + PathOf(heldBox(item.Box).Element)
	}
	if l.reportedOverflow[key] {
		return
	}
	l.reportedOverflow[key] = true
	l.rec.ReportDetail(Finding{
		Rule: RuleUnbreakableOverflow,
		Message: what + " is " +
			fmtPx(item.Width) + " wide and cannot be broken, in a space " +
			fmtPx(width) + " wide; the part past the edge will not be drawn",
		Path: PathOf(heldBox(item.Box).Element),
	})
}

// reportWordBreak reports a word-break value this engine reads as normal.
//
// "break-all" is implemented; "keep-all" and "auto-phrase" are not, and both
// *remove* or *move* opportunities rather than adding them — keep-all stops CJK
// text breaking between two ideographs, and auto-phrase moves a Korean break to
// a phrase boundary. Ignoring either breaks a line somewhere the author said not
// to, which no amount of looking at the page reveals as a missing feature.
//
// Once per value per box, for the same reason checkScript is once per script.
func (l *layouter) reportWordBreak(b *Box, value string) {
	if l.reportedWordBreak == nil {
		l.reportedWordBreak = map[string]bool{}
	}
	if l.reportedWordBreak[value] {
		return
	}
	l.reportedWordBreak[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "word-break",
		Message: value + " was read as normal, so a line may break where the " +
			"value asked it not to",
		Path: PathOf(b.Element),
	})
}

// reportLineBreak reports a line-break value this engine reads as auto.
//
// Unlike its word-break counterpart it is conditional on the text, and the
// condition is what keeps it honest. loose, normal and strict differ from auto
// only in how strictly CJK text may break — around small kana, around iteration
// marks, before centred punctuation — and over Latin text the three provably
// change nothing. The suite says so: pre-wrap-004, -005 and -006 exist to assert
// that "XX    XX" wraps the same under all of them. Warning there would be
// crying wolf on a page that is correct.
//
// So the report is made where the difference could show, which is text with an
// ideograph in it — the only text this engine breaks by a rule the three values
// have anything to say about.
//
// What they have to say about it grew. This engine used to break CJK on one
// rule, "between two ideographs", which all three values leave alone; it now
// also refuses to begin a line with a closing bracket, an exclamation mark or a
// non-starter, which is what linebreak.go is for. That is UAX #14's default and
// so CSS's normal, and it is exactly the set loose relaxes and strict extends —
// so the difference the report warns about is now real in both directions
// rather than merely possible in one, which is what the message says.
func (l *layouter) reportLineBreak(b *Box, value string) {
	if !strings.ContainsFunc(b.Text, isIdeographic) {
		return
	}
	if l.reportedLineBreak == nil {
		l.reportedLineBreak = map[string]bool{}
	}
	if l.reportedLineBreak[value] {
		return
	}
	l.reportedLineBreak[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "line-break",
		Message: value + " was read as auto, so CJK text may break where the " +
			"value asked it not to, or hold together where it asked it to break",
		Path: PathOf(b.Element),
	})
}

// reportTextJustify reports a justification method this engine does not perform.
//
// It is called only where a line is actually being justified, which is the
// condition that makes the value matter: text-justify on a block that is not
// justified changes nothing, and warning there would be crying wolf on a page
// that is correct. The same reasoning as reportLineBreak's, and for the same
// reason — a finding nobody can act on is a finding nobody reads.
//
// What the values ask for is real and not a nuance. inter-character puts the
// slack between letters as well as between words, which is how Thai and
// Chinese are justified; a page that spread it between the words instead has
// the right margins and the wrong text.
func (l *layouter) reportTextJustify(b *Box, value string) {
	if l.reportedTextJustify == nil {
		l.reportedTextJustify = map[string]bool{}
	}
	if l.reportedTextJustify[value] {
		return
	}
	l.reportedTextJustify[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "text-justify",
		Message: value + " was read as auto, so the line was stretched between " +
			"its words rather than in the way the value asked for",
		Path: PathOf(b.Element),
	})
}

// checkScript reports text this engine cannot break or order correctly.
//
// It is the unsupported-script guardrail of §6.3, and it is an error by default
// for the reason given there: unbroken or unordered text still looks like text,
// so the failure mode looks like success. A paragraph of Thai run together as
// one word overflows silently; a line of Arabic laid out left to right reads as
// a rendering bug rather than as something this engine declined to do.
func (l *layouter) checkScript(b *Box) {
	for _, r := range b.Text {
		if script, bad := unsupportedScript(r); bad {
			key := script + "\x00" + b.Style["font-family"]
			if l.reportedScripts[key] {
				return
			}
			l.reportedScripts[key] = true
			l.rec.ReportDetail(Finding{
				Rule:    RuleUnsupportedScript,
				Message: script,
				Path:    PathOf(b.Element),
			})
			return
		}
	}
}

// checkGlyphs reports characters the chosen face has no glyph for.
//
// This is the glyph-missing guardrail of §6.3, an error by default because tofu
// is the purest form of silent garbage: a reader who sees a row of boxes where
// letters should be blames their PDF viewer, not the document, and the author
// never hears about it at all.
//
// It is reported once per character rather than once per occurrence, because
// what an author needs to know is *which* characters their font cannot set —
// hearing it four hundred times about the same one is not four hundred times as
// useful.
func (l *layouter) checkGlyphs(b *Box, face *shape.Face, text string) {
	// The question has to be the one *drawing* answers, and it was not.
	//
	// This asked face.GlyphID, which is whether the face has a glyph mapped to
	// a code point. Shaping asks something different and gets a different
	// answer: a no-break space has no glyph of its own and is set as a space, a
	// bidi override has none and takes no room at all, and the same goes for
	// every fixed-width Unicode space and every zero-width format control. All
	// of them draw correctly, and all of them were being reported — at Error
	// severity, the one that stops a document being produced.
	//
	// Measured over the reftest suite, that was the single most common finding
	// in the whole engine: 154 documents reported a missing glyph for the
	// no-break space alone, and 260 documents were kept out of the clean-pass
	// count by nothing else. A guardrail wrong that often is worse than no
	// guardrail, because the reports it is right about are buried.
	//
	// Shaping the whole run first is also what makes this cheap: the answer is
	// almost always that nothing is missing, and only then is it worth walking
	// the characters to find out which.
	if !missesVisible(face, text) {
		return
	}
	for _, r := range text {
		if r == '\n' || r == '\t' || marksNoPaper(r) {
			continue
		}
		if isVisibleControl(r) {
			// Drawn as a synthesized box rather than as a glyph, so no face was
			// ever asked for one and nothing is missing from the page. See
			// controlchar.go.
			continue
		}
		if _, missing := face.ShapeGlyphs(string(r)); missing == 0 {
			continue
		}
		key := string(r) + "\x00" + face.Name()
		if l.reportedGlyphs[key] {
			continue
		}
		l.reportedGlyphs[key] = true
		l.rec.ReportDetail(Finding{
			Rule: RuleGlyphMissing,
			Message: "the face " + quoteValue(face.Name()) + " has no glyph for " +
				describeRune(r) + ", which is set as a space, so the character is " +
				"missing from the page and from the text extracted out of it",
			Path: PathOf(b.Element),
		})
	}
}

// reportHyphens reports a hyphens value this engine reads as manual.
//
// There is one: "auto", which asks the engine to hyphenate words that contain
// no soft hyphen at all. Doing it needs a set of hyphenation patterns for the
// document's language — Liang's, one table per language, and the tables are
// large and are not derivable from anything Unicode publishes — so what a
// document gets is the soft hyphens it wrote and no more.
//
// That is a page missing line breaks a browser would make, which shows as
// looser lines rather than as anything obviously wrong, so it is exactly the
// kind of difference a reader cannot see and a finding has to say.
//
// "manual" and "none" are both implemented and neither is reported.
//
// # Only where a language was declared
//
// §6.1 does not ask a UA to hyphenate everything: "correct automatic
// hyphenation requires a hyphenation resource appropriate to the language of
// the text being broken. The UA is therefore only required to automatically
// hyphenate text for which the author has declared a language ... and for which
// it has an appropriate hyphenation resource."
//
// So a document that never says what language it is in gets no hyphenation from
// any conforming engine, and this one's page is not missing anything — it is the
// page the specification asks for. The suite says so in as many words:
// hyphens-auto-001 is titled "automatic hyphenation must not work without
// language tagging" and passes by *nothing* being hyphenated.
//
// Reporting it anyway was the same mistake inert.go corrects for a declaration
// at its initial value: the finding was true of the property rather than of what
// the property was being asked to do. Eight of the suite's reftests were held
// out of the clean count by a report about a page that was already right.
//
// Where a language *is* declared the gap is real and is reported as before. This
// engine has no hyphenation resource for any language, so the second half of
// §6.1's sentence would excuse it too — but that reading empties the finding
// out, and the page really does differ from the one the author asked for and the
// one every browser produces. A missing resource is a limitation worth naming; a
// document with no language to look one up by is not.
//
// Once per value per document, on the model of reportWordBreak.
func (l *layouter) reportHyphens(b *Box, value string) {
	if boxLanguage(b) == "" {
		return
	}
	if l.reportedHyphens == nil {
		l.reportedHyphens = map[string]bool{}
	}
	if l.reportedHyphens[value] {
		return
	}
	l.reportedHyphens[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "hyphens",
		Message: value + " was read as manual, so a word is broken only where a " +
			"soft hyphen asks and never where a dictionary would",
		Path: PathOf(b.Element),
	})
}

// reportKerning names a request about a font's own rules that this engine cannot
// carry out.
//
// "font-kerning: none" is not one of them any more — it is applied, see
// layout/fontfeatures.go — and what is left is font-feature-settings, which asks
// for a named feature by tag.
//
// The narrowing it keeps is worth stating, because it is the same one inert.go
// makes for a declaration at its initial value, one step further along: the
// question is not what the property is but what it is being asked to *do*, and
// here the answer depends on the font. "font-feature-settings: \"kern\" off"
// asks for nothing at all when the face has no kerning in it, and the fourteen
// standard PDF faces are that case — their metrics carry no kern pairs.
//
// That is not a corner of the suite. Five of its reftests write the declaration
// over text in the default serif face, and every one of them was held out of the
// clean count by a finding about a page that is right.
//
// The property is judged only by the tags it names. "kern" is the one this can
// answer, because a face's kerning is a thing the shaping layer knows about; any
// other tag is a feature this engine neither applies nor can ask the face for,
// so a value naming one is reported whatever the face has in it.
func (l *layouter) reportKerning(b *Box, face *shape.Face) {
	kerns := face != nil && face.HasKerning()
	if value := b.Style["font-feature-settings"]; !inertFontFeatures(value, kerns) {
		l.reportOnce("font-feature-settings", Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-feature-settings",
			Message: "font-feature-settings " + quoteValue(value) + " was not applied; " +
				"this engine applies the features a face declares for the script and " +
				"takes no direction about which",
			Path: PathOf(boxElement(b)),
		})
	}
}

// inertFontFeatures reports whether a font-feature-settings value asks for the
// page that is already there.
//
// "normal" asks for nothing by definition. Otherwise the value is a list of tags
// with a setting each, and it is inert when every tag in it is one the face
// cannot act on — which this can answer for "kern" and for nothing else.
func inertFontFeatures(value string, kerns bool) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "normal" {
		return true
	}
	for _, part := range strings.Split(value, ",") {
		tag := strings.TrimSpace(part)
		// The tag is a quoted string and the setting follows it. Only the tag
		// is read: "kern" is inert on a face with no kerning whether it was
		// asked for or turned off, because neither can change the page.
		tag = strings.TrimLeft(tag, "\"'")
		if i := strings.IndexAny(tag, "\"'"); i >= 0 {
			tag = tag[:i]
		}
		if tag != "kern" || kerns {
			return false
		}
	}
	return true
}

// reportAutospace names the part of text-autospace this engine does not do.
//
// §8.1's grammar has a third class of boundary — "punctuation", which asks for
// spacing around full-width punctuation — and a second half that says what to do
// where the author already wrote a space: "insert" adds spacing where there is
// none and "replace" exchanges the space for it. The two ideograph classes and
// "insert" are implemented; the rest is read, dropped and named.
//
// Once per value per document, on the model of reportWordBreak.
func (l *layouter) reportAutospace(b *Box, value string) {
	if l.reportedAutospace == nil {
		l.reportedAutospace = map[string]bool{}
	}
	if l.reportedAutospace[value] {
		return
	}
	l.reportedAutospace[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "text-autospace",
		Message: quoteValue(value) + " in text-autospace was not applied; the " +
			"spacing between an ideograph and a letter or a number is inserted " +
			"and the rest of the property is not",
		Path: PathOf(b.Element),
	})
}

// hyphenCharacter is what a broken word ends with, which the document may say.
//
// CSS Text §6.3's hyphenate-character is "auto | <string>". The keyword leaves
// the choice to the engine, which is hyphenTextFor below; a string is printed
// as it stands, and the empty string is one of them — "hyphenate-character: \"\""
// asks for words to be broken with no mark at all, which the suite's
// hyphenate-character-001 tests by name. So "the author said nothing" and "the
// author said nothing is to be printed" are two different answers and cannot
// both be the empty string.
//
// Anything that is not a keyword and not a single string is invalid and is
// treated as the keyword, which is what the cascade does with a declaration it
// cannot parse.
func hyphenCharacter(value string, face *shape.Face) string {
	if strings.TrimSpace(value) == "" || strings.EqualFold(strings.TrimSpace(value), "auto") {
		return hyphenTextFor(face)
	}
	vals, errs := css.ParseComponentValues(value)
	if len(errs) != 0 {
		return hyphenTextFor(face)
	}
	found, seen := "", false
	for _, v := range vals {
		if !v.IsToken() {
			return hyphenTextFor(face)
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.String:
			if seen {
				return hyphenTextFor(face)
			}
			found, seen = v.Token.Value, true
		default:
			return hyphenTextFor(face)
		}
	}
	if !seen {
		return hyphenTextFor(face)
	}
	return found
}

// hyphenTextFor is the character a broken word ends with when the document has
// not said which.
//
// CSS Text §6.1 leaves it to the engine, and the note the suite's own
// hyphens-manual-011 carries says what the choice is: "user agents may use
// U+2010 HYPHEN when the font has the glyph, or may use U+002D HYPHEN-MINUS
// otherwise". That test names two references, one for each, because the two are
// different glyphs in some faces — so either answer is right and neither may be
// assumed.
//
// U+2010 is the typographically correct character and is what this asks for
// first. A face without it would otherwise draw a missing glyph, which is a box
// where a hyphen should be, so the fallback is not a nicety.
func hyphenTextFor(face *shape.Face) string {
	const hyphen, hyphenMinus = "‐", "-"
	if face == nil {
		return hyphenMinus
	}
	// missing rather than the glyph count, and the difference is the whole of
	// this function. A face that cannot set a character still returns a glyph
	// for it — the standard PDF faces substitute a space, which is what a
	// reader shows for an undefined code — so a run of "has it drawn anything"
	// says yes for every character there is. Courier is exactly that case:
	// U+2010 is outside WinAnsi, and asking the wrong question put a space
	// where the hyphen belongs and left the word looking unbroken.
	//
	// The synthetic item the line breaking appends carries the face of the text
	// beside it and is not put through the family walk, so there is no fallback
	// behind this: the character chosen here has to be one this face can set.
	if _, missing := face.ShapeGlyphs(hyphen); missing == 0 {
		return hyphen
	}
	return hyphenMinus
}

// reportHangingPunctuation reports a hanging-punctuation value this engine
// reads as none.
//
// "first", "last" and "allow-end" are implemented. "force-end" is not: it hangs
// a stop or a comma at the end of *every* line whether or not the line would
// otherwise hold it, which is a decision about every line rather than about the
// one that overflowed — and the one that overflowed is the only one the fill has
// a reason to ask about.
//
// What they change is where a line breaks, and that shows as a word moved to
// the next line with nothing on the page to say why, so it is exactly the kind
// of difference a reader cannot diagnose and a finding has to state.
func (l *layouter) reportHangingPunctuation(b *Box, value string) {
	if l.reportedHanging == nil {
		l.reportedHanging = map[string]bool{}
	}
	if l.reportedHanging[value] {
		return
	}
	l.reportedHanging[value] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "hanging-punctuation",
		Message: value + " was not applied, so a stop or a comma at the end of a " +
			"line takes room the value asked it to give up",
		Path: PathOf(b.Element),
	})
}

// boxElement is the element a box belongs to: its own, or the nearest one above
// it.
//
// A text box has none. This engine gives the box holding a text node no element
// of its own, so a finding raised about one and pointed at b.Element points at
// nothing — and every such finding in a document then has the same empty path,
// which is enough for the recorder to take them all for one. That is a finding
// that cannot say where it is about, and it looks exactly like a finding that is
// correctly raised once.
func boxElement(b *Box) *html.Node {
	for cur := b; cur != nil; cur = cur.Parent {
		if cur.Element != nil {
			return cur.Element
		}
	}
	return nil
}

// boxLanguage is the language in force at a box: the nearest lang attribute at
// or above the nearest element.
//
// The walk up the *box* tree is what a text box needs. A text node has no
// attributes and this engine gives its box no element either, so asking
// languageAt about one asks about nothing; the answer is on the element that
// holds the text, which is the first box above it that has one.
func boxLanguage(b *Box) paragraph.Language {
	return languageAt(boxElement(b))
}

// boxWritingSystem is boxLanguage's neighbour for the rules that ask what a text
// is *typeset* as rather than what language it is in. See
// paragraph.WritingSystemOf, and writingSystemAt for the walk.
func boxWritingSystem(b *Box) paragraph.WritingSystem {
	return writingSystemAt(boxElement(b))
}
