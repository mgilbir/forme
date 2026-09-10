package layout

import (
	"strconv"
	"strings"
	"unicode"

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
			fmtPx(width) + " wide" + l.overflowFate(heldBox(item.Box)),
		Path: PathOf(heldBox(item.Box).Element),
	})
}

// overflowFate says what becomes of content that leaves a box's edge, as a
// clause to hang off the end of a finding.
//
// There are two answers and this engine used to give only one of them. Both
// this rule and the table column's said "the part past the edge will not be
// drawn", and that is what happens when something clips: "overflow" is the
// property, its initial value is "visible", and a box whose overflow is visible
// draws its content wherever the content goes. Measured on a 60px box holding
// ten W's at 20px monospace, the run is emitted at the box's own origin with no
// clip on it and covers 128px — every glyph on the page, over whatever was
// beside it.
//
// Telling an author their text was cut off when it is drawn over the next
// column is the wrong finding twice: they look for missing words and find them
// all, and they do not look for the thing that is actually wrong. So the clause
// is chosen by asking, and it is exact rather than a guess — the box and its
// ancestors are right here, and overflowClips is the same question paint asks
// when it builds the clip.
//
// Both remain worth a finding. Content that overlaps its neighbour is as much a
// page nobody proofread as content that vanished.
func (l *layouter) overflowFate(b *Box) string {
	for ; b != nil; b = b.Parent {
		if l.overflowClips(b) {
			return "; the part past the edge is not drawn, because \"overflow\" " +
				"on <" + elementName(b) + "> clips it"
		}
	}
	return "; it is drawn past the edge, over whatever is beside it"
}

// reportWordBreak reports a word-break value this engine reads as normal.
//
// All four values are implemented, and this fires for the one that is
// implemented only in part: "auto-phrase" ends a line at a phrase boundary, and
// finding one takes a model of the language, of which there is one here. A
// document in another language that has phrases gets "normal" — which is what
// §5.2 asks of a UA with no model — and is told, because a line that ends in
// the middle of a phrase is not something looking at the page reveals as a
// missing feature. See paragraph.PhrasesUnfound for the three things that have
// to be true at once.
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
		if isDefaultIgnorable(r) {
			// A character that draws nothing cannot be missing from the page:
			// there is nothing of it to be missing. This finding says "the
			// character is missing from the page and from the text extracted
			// out of it", and neither half is true of a joiner or a variation
			// selector or a soft hyphen.
			//
			// The comment above says shaping answers "not missing" for all of
			// them, and it is nearly right — that was measured on Ahem, and
			// Ahem does report one: U+180E MONGOLIAN VOWEL SEPARATOR, which
			// Unicode reclassified from a space to a format character in 6.3
			// and which the suite's line-breaking-atomic-015 writes. So the
			// rule is asked directly rather than left to a coincidence about
			// how faces happen to shape.
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

// reportCaps names a request for capitals the face cannot supply.
//
// Every value of CSS Fonts 4 §6.6's font-variant-caps is a request for features
// the face declares — 'smcp' for small capitals, 'c2sc' beside it for the
// capitals too, 'pcap' and 'c2pc' for petite ones, 'unic', 'titl' — and a face
// that declares none of them sets the text in the letters it is written with, at
// the size it is written at. That is a page the document did not ask for and
// nothing about it looks wrong, which is exactly the shape of failure §6.3's
// findings exist for: a paragraph the author expects in small capitals comes out
// in lowercase and reads perfectly well.
//
// This engine synthesises none of them. A synthesised small capital is the
// uppercase letter drawn at a fraction of the size, which every browser does and
// none of them the same way, and doing it here means the run is no longer one
// run: the letters it changes are set at a different size from the ones it does
// not, so the item has to be cut, measured and drawn in pieces. Until that is
// built the honest answer is the report.
//
// # What it is asked about, and why not once per box
//
// Per face run, like checkGlyphs and for the same reason: the box's own face may
// have no small capitals while the fallback face that actually set a word does,
// or the other way round, and a report keyed on the box would be right about
// neither. The runs are the ones the items are built from, so what is checked is
// what is drawn.
//
// # Per tag, and only where the tag has something to act on
//
// A value asking for two features may get one of them. Noto Sans declares both
// 'smcp' and 'c2sc', and a face with only the first carries out half of
// "all-small-caps": the lowercase letters become small capitals and the capitals
// stay full height, which is "small-caps" and not what was asked for. Naming the
// tag that is missing is the difference between an author knowing which half of
// their line is wrong and knowing only that something is.
//
// And a tag is only reported where the text has a letter it could act on. 'smcp'
// replaces lowercase letters and 'c2sc' capitals, so a run of digits, of Han, or
// of one case where the missing tag wants the other is set identically with the
// feature and without it — and a finding about it would be this engine calling a
// correct page a failure. It is the same narrowing reportKerning makes for a
// "kern" a face has not got.
func (l *layouter) reportCaps(b *Box, face *shape.Face, text string) {
	want, unhandled := capsOf(b.Style["font-variant-caps"])
	if unhandled != "" {
		// All six of §6.6 are read, so a value outside them is either a mistake
		// the author made or a value from a level this engine has not read —
		// and nothing here can tell the two apart. It is reported as the second,
		// which is the way every other reader in this file answers a value it
		// cannot act on, and the direction to err in: an author whose typo is
		// called a missing feature looks at their stylesheet and finds it, and
		// an author whose new value is called a typo is told the opposite of
		// what is true.
		l.reportOnce("font-variant-caps:"+unhandled, Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-variant-caps",
			Message: quoteValue(unhandled) + " is not a value of font-variant-caps " +
				"this engine reads; the text was set in the letters it is " +
				"written with",
			Path: PathOf(boxElement(b)),
		})
		return
	}
	if face == nil {
		return
	}
	// §6.6's fallback first: a document asking a face with no petite capitals
	// for them gets its small ones, and what is missing is decided from what
	// the face will actually be asked. See resolveCaps.
	use := resolveCaps(want, face)
	missing := missingCapsFeatures(use, face, text)
	if len(missing) == 0 {
		return
	}
	value := strings.ToLower(strings.TrimSpace(b.Style["font-variant-caps"]))
	if capsAreSynthesised(use) {
		// The face has none of them and this engine made the capitals itself,
		// which is a page §6.6 asked for rather than a gap. It is still worth
		// saying: a scaled capital is not the one a designer would have drawn,
		// and the page carries the uppercase text. See RuleCapsSynthesised.
		l.reportOnce("caps-synthesised:"+value+":"+strings.Join(missing, ",")+":"+face.Name(), Finding{
			Rule:     RuleCapsSynthesised,
			Property: "font-variant-caps",
			Message: "font-variant-caps " + quoteValue(value) + " asks a face for " +
				strings.Join(use.Features(), " and ") + "; " + quoteValue(face.Name()) +
				" declares no " + strings.Join(missing, " or ") + ", so " +
				capsMadeHere(missing, use) + " were made out of the letters at " +
				strconv.FormatFloat(smallCapScale(face), 'g', 3, 64) +
				" of the size, and the page carries them as uppercase text",
			Path: PathOf(boxElement(b)),
		})
		return
	}
	// "that part of the text" where the face carried out some of the request: a
	// face with 'smcp' and no 'c2sc' asked for "all-small-caps" lowers the
	// lowercase letters and leaves the capitals full height, which is a line in
	// two heights of letter rather than a line in the wrong ones.
	came := "the text was set in the letters it is written with"
	if len(missing) < len(want.Features()) {
		came = "that part of the text was set in the letters it is written with"
	}
	// Keyed on the value as well as the face and the tags: two declarations can
	// fall short in the same tag — "small-caps" and "all-small-caps" over
	// lowercase text both come down to a missing 'smcp' — and an author who
	// wrote both wants to hear about both, since the message names the value
	// they wrote.
	l.reportOnce("font-variant-caps:"+value+":"+strings.Join(missing, ",")+":"+face.Name(), Finding{
		Rule:     RuleUnsupportedValue,
		Property: "font-variant-caps",
		Message: "font-variant-caps " + quoteValue(value) + " asks a face for " +
			strings.Join(want.Features(), " and ") + "; " + quoteValue(face.Name()) +
			" declares no " + strings.Join(missing, " or ") + ", so " + came +
			", because this engine uses the capitals a face draws and does not " +
			"make them out of the letters at a smaller size",
		Path: PathOf(boxElement(b)),
	})
}

// capsMadeHere names the letters the synthesis had to make, which is the half of
// the request the face did not answer.
//
// A face may answer one half: 'smcp' and no 'c2sc' asked for "all-small-caps"
// lowers the lowercase letters and leaves the capitals standing. Saying "the
// capitals were made here" then tells an author which half of their line is the
// designer's work and which is this engine's.
func capsMadeHere(missing []string, use shape.Caps) string {
	var lower, capitals bool
	for _, tag := range missing {
		switch tag {
		case use.Lowercase():
			lower = true
		case use.Capitals():
			capitals = true
		}
	}
	switch {
	case lower && capitals:
		return "both cases"
	case capitals:
		return "the capitals"
	}
	return "the small capitals"
}

// missingCapsFeatures is the tags a value needs that this face has not got and
// this text would have shown.
func missingCapsFeatures(want shape.Caps, face *shape.Face, text string) []string {
	wanted := want.Features()
	if len(wanted) == 0 {
		return nil
	}
	lower, upper := hasCase(text)
	var missing []string
	for _, tag := range wanted {
		if !capsTagWouldShow(tag, lower, upper) || faceDeclares(face, tag) {
			continue
		}
		missing = append(missing, tag)
	}
	return missing
}

// capsTagWouldShow reports whether a tag has a letter in this text to act on.
//
// The three answers are the three kinds of rule §6.6 names: one that replaces
// lowercase letters ('smcp', 'pcap'), one that replaces capitals ('c2sc',
// 'c2pc', and 'titl', which cuts the capitals differently), and 'unic', which
// puts both cases at one height and so acts on either.
func capsTagWouldShow(tag string, lower, upper bool) bool {
	switch tag {
	case "smcp", "pcap":
		return lower
	case "c2sc", "c2pc", "titl":
		return upper
	case "unic":
		return lower || upper
	}
	return false
}

// faceDeclares reports whether a face offers a feature.
//
// It is Features() and not a table lookup, because a face may offer a feature
// through a ligature or a contextual rule as well as a plain one-for-one
// substitution — see shape's TestAFeatureOfferedThroughALigatureIsListed, which
// is that case stated as a font.
func faceDeclares(face *shape.Face, tag string) bool {
	for _, got := range face.Features() {
		if got == tag {
			return true
		}
	}
	return false
}

// hasCase reports which cases the text has letters in.
//
// A letter with a form of the other case is what these features cover, so that
// is the question — not unicode.IsLower and IsUpper, which are true of
// characters no face maps anywhere, and not "is a letter", which is true of the
// scripts that have one case only.
func hasCase(text string) (lower, upper bool) {
	for _, r := range text {
		if unicode.ToUpper(r) != r {
			lower = true
		}
		if unicode.ToLower(r) != r {
			upper = true
		}
		if lower && upper {
			break
		}
	}
	return lower, upper
}

// reportNumeric names a request about the figures the face cannot carry out.
//
// Every keyword of CSS Fonts 4 §6.7 is a request for a feature the face
// declares, and a face that declares none of them sets the digits it has: a
// column of figures that will not line up, a fraction written as three
// characters, a zero that cannot be told from a capital O. None of that looks
// wrong on the page, which is the shape of failure §6.3's findings exist for.
//
// # Nothing here is synthesised, and that is not a gap in this file
//
// An oldstyle figure is a shape a designer drew, and so is a slashed zero and a
// stacked fraction. There is nothing to make one out of — the letter at a
// smaller size, which is what small capitals are synthesised from, has no
// counterpart here — and no browser makes one either. So this reports where
// smallcaps.go produces, and the finding is the whole of what the engine can do
// about it.
//
// # Per tag, per run, and only where the tag has something to act on
//
// The first two for the reasons reportCaps gives. The third is narrower than it
// looks: every one of §6.7's features acts on digits, so a run with none in it
// is set identically with them and without, and a finding about it would be this
// engine calling a correct page a failure. A digit is a necessary condition and
// not a sufficient one — 'ordn' wants letters after one and 'frac' a slash
// between two — and it is where the line is drawn, because the rest is the
// font's business and cannot be known from here.
//
// # The two the face may already be doing
//
// "tabular-nums" asks for digits that all take the same room, and almost every
// text face draws them that way to begin with: the fourteen standard PDF faces
// do, and so does Noto Sans. A face whose digits already share an advance is
// being asked for the page it is already setting, and reporting it would hold a
// correct document out of the clean count for ever — which is the narrowing
// reportKerning makes for a "kern" a face has not got, arrived at from the other
// side. "proportional-nums" is the same question with the answer reversed.
//
// The other six cannot be answered this way. Whether a face's default figures
// are lining or oldstyle, whether its zero is slashed, whether it builds a
// fraction — none of that is in the metrics, and guessing would be worse than
// the report.
func (l *layouter) reportNumeric(b *Box, face *shape.Face, text string) {
	want, unhandled := numericOf(b.Style["font-variant-numeric"])
	if unhandled != "" {
		// All eight of §6.7 are read, so a word outside them is either a
		// mistake the author made or a value from a level this engine has not
		// read, and nothing here can tell the two apart. See reportCaps, which
		// makes the same choice for the same reason.
		l.reportOnce("font-variant-numeric:"+unhandled, Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-variant-numeric",
			Message: quoteValue(unhandled) + " is not a value of " +
				"font-variant-numeric this engine reads; the figures were set " +
				"as the face draws them",
			Path: PathOf(boxElement(b)),
		})
		return
	}
	if want == 0 || face == nil || !hasDigit(text) {
		return
	}
	var missing []string
	for _, tag := range want.Features() {
		if faceDeclares(face, tag) || numericIsInert(tag, face) {
			continue
		}
		missing = append(missing, tag)
	}
	if len(missing) == 0 {
		return
	}
	value := strings.ToLower(strings.TrimSpace(b.Style["font-variant-numeric"]))
	l.reportOnce("font-variant-numeric:"+value+":"+strings.Join(missing, ",")+":"+face.Name(),
		Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-variant-numeric",
			Message: "font-variant-numeric " + quoteValue(value) + " asks a face for " +
				strings.Join(want.Features(), " and ") + "; " + quoteValue(face.Name()) +
				" declares no " + strings.Join(missing, " or ") + ", and this engine " +
				"does not draw a figure a designer did not — so that much of the " +
				"text was set in the figures the face has",
			Path: PathOf(boxElement(b)),
		})
}

// numericIsInert reports whether a tag asks for the page the face is already
// setting.
//
// Two of the eight can be answered from the metrics, and they are the two an
// author is most likely to write. See reportNumeric.
func numericIsInert(tag string, face *shape.Face) bool {
	switch tag {
	case "tnum":
		return digitsShareAnAdvance(face)
	case "pnum":
		return !digitsShareAnAdvance(face)
	}
	return false
}

// digitsShareAnAdvance reports whether every digit in the face takes the same
// room, which is what "tabular" means and what a column of figures needs.
//
// A face missing a digit answers false, which is the safe direction: it is not
// the face a document setting figures wants, the request cannot be shown to be
// inert, and the report says so.
func digitsShareAnAdvance(face *shape.Face) bool {
	first, ok := face.Advance('0')
	if !ok {
		return false
	}
	for r := '1'; r <= '9'; r++ {
		got, ok := face.Advance(r)
		if !ok || got != first {
			return false
		}
	}
	return true
}

// hasDigit reports whether §6.7's features would have anything to act on.
//
// The decimal digits and nothing wider. unicode.IsDigit is true of every
// script's digits, and a face's 'onum' or 'tnum' covers the European ones it
// drew — so a run of Devanagari numerals is set identically either way, and a
// report about it would name a feature that could not have changed it.
func hasDigit(text string) bool {
	for _, r := range text {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// reportEastAsian names a request about the East Asian forms the face cannot
// carry out.
//
// Every keyword of CSS Fonts 4 §6.9 is a request for a feature the face
// declares: the Japanese national standards, the two forms of a simplified
// character, the ideographic advance against the character's own, and the kana
// an annotation is set in. A face that declares none of them sets the forms it
// has — a page of ideographs in whichever revision the designer drew, Latin
// letters on their own advance where a grid was asked for — and none of it looks
// wrong, which is the shape of failure §6.3's findings exist for.
//
// Nothing here is synthesised. A JIS78 ideograph is a shape a designer drew, and
// so is a ruby kana; a full-width Latin letter is a second drawing of the same
// letter on the ideographic advance, and centring the proportional one in an em
// would be this engine inventing a typeface. No browser does either.
//
// # Which characters each tag could act on
//
// Three classes, and they are coarser than the nine features because what can be
// known here is coarser. The six national forms and 'ruby' need an East Asian
// character — an ideograph or a kana — and a run of Latin is set identically
// with them and without.
//
// The two widths are the ones that reach Latin text, and that is the whole point
// of them: a Japanese font draws the ASCII letters twice, and "full-width" asks
// for the wide drawing. So 'fwid' needs a character that *has* a full-width form
// and 'pwid' one that *is* one — read from the table text-transform's own
// full-width value is applied from, so the two cannot drift apart. An East Asian
// character counts for both, because a font may set its kana proportionally and
// a halfwidth kana is a width pair as well as a kana.
func (l *layouter) reportEastAsian(b *Box, face *shape.Face, text string) {
	want, unhandled := eastAsianOf(b.Style["font-variant-east-asian"])
	if unhandled != "" {
		l.reportOnce("font-variant-east-asian:"+unhandled, Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-variant-east-asian",
			Message: quoteValue(unhandled) + " is not a value of " +
				"font-variant-east-asian this engine reads; the text was set in " +
				"the forms the face draws",
			Path: PathOf(boxElement(b)),
		})
		return
	}
	if want == 0 || face == nil {
		return
	}
	ideographs, wide, narrow := eastAsianCharacters(text)
	var missing []string
	for _, tag := range want.Features() {
		if faceDeclares(face, tag) || !eastAsianTagWouldShow(tag, ideographs, wide, narrow) {
			continue
		}
		missing = append(missing, tag)
	}
	if len(missing) == 0 {
		return
	}
	value := strings.ToLower(strings.TrimSpace(b.Style["font-variant-east-asian"]))
	l.reportOnce("font-variant-east-asian:"+value+":"+strings.Join(missing, ",")+":"+face.Name(),
		Finding{
			Rule:     RuleUnsupportedValue,
			Property: "font-variant-east-asian",
			Message: "font-variant-east-asian " + quoteValue(value) + " asks a face for " +
				strings.Join(want.Features(), " and ") + "; " + quoteValue(face.Name()) +
				" declares no " + strings.Join(missing, " or ") + ", and this engine " +
				"does not draw a form a designer did not — so that much of the " +
				"text was set in the forms the face has",
			Path: PathOf(boxElement(b)),
		})
}

// eastAsianTagWouldShow reports whether a tag has anything in this run to act
// on. See reportEastAsian for the three classes.
func eastAsianTagWouldShow(tag string, ideographs, wide, narrow bool) bool {
	switch tag {
	case "fwid":
		return ideographs || narrow
	case "pwid":
		return ideographs || wide
	}
	return ideographs
}

// eastAsianCharacters is what a run holds that §6.9's features could act on: an
// East Asian character, one that is a full-width form, and one that has a
// full-width form.
//
// All three in one pass, because a run of Japanese with Latin words in it is
// every one of them and asking three times would walk the text three times.
func eastAsianCharacters(text string) (ideographs, wide, narrow bool) {
	for _, r := range text {
		switch {
		case paragraph.IsAutospaceIdeograph(r):
			ideographs = true
		case paragraph.IsFullWidthForm(r):
			wide = true
		case paragraph.HasFullWidthForm(r):
			narrow = true
		}
		if ideographs && wide && narrow {
			break
		}
	}
	return ideographs, wide, narrow
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
//
// The second result says the author supplied one, which is what lets a language
// fill the gap where they did not: §6.3.1 asks a UA to use "the appropriate
// language-specific hyphenation character(s)", and §6.3.2 lets the document
// overrule it. Returning "the author said nothing" as the empty string would
// not do — "hyphenate-character: \"\"" asks for no mark at all.
func hyphenCharacter(value string) (string, bool) {
	if strings.TrimSpace(value) == "" || strings.EqualFold(strings.TrimSpace(value), "auto") {
		return "", false
	}
	vals, errs := css.ParseComponentValues(value)
	if len(errs) != 0 {
		return "", false
	}
	found, seen := "", false
	for _, v := range vals {
		if !v.IsToken() {
			return "", false
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.String:
			if seen {
				return "", false
			}
			found, seen = v.Token.Value, true
		default:
			return "", false
		}
	}
	return found, seen
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

// boxHyphenation is boxLanguage's neighbour for the one rule that is keyed on
// the script as well as the language. See paragraph.HyphenationOf.
func boxHyphenation(b *Box) paragraph.Language {
	return hyphenationAt(boxElement(b))
}

// boxWritingSystem is boxLanguage's neighbour for the rules that ask what a text
// is *typeset* as rather than what language it is in. See
// paragraph.WritingSystemOf, and writingSystemAt for the walk.
func boxWritingSystem(b *Box) paragraph.WritingSystem {
	return writingSystemAt(boxElement(b))
}

// reportSpacingTrim reports a text-spacing-trim value whose rule this engine
// does not follow.
//
// §8.2's values differ in what they do at the *start* of a line — whether a
// full-width opening bracket keeps the half em of blank in front of it, and on
// which lines — and that is the half of the property this engine does not do.
// So "space-first" and "trim-start" are reported and the other two are not:
// "space-all" asks for full-width everywhere, which is what an engine that
// trims only at the end of a line already gives it, and "normal" is the initial
// value.
//
// Not reporting the initial value is a decision and not an oversight. Every
// document that holds CJK text has it, so a finding would appear on documents
// whose author never wrote the property and never depended on the clause; what
// it would say is "this engine does not do all of §8.2", which is a fact about
// the engine and not about the page. The clause that is missing takes room away
// at the start of a line, and a document that needs it says so.
func (l *layouter) reportSpacingTrim(b *Box, value string) {
	l.reportOnce("text-spacing-trim", Finding{
		Rule:     RuleUnsupportedValue,
		Property: "text-spacing-trim",
		Message: "text-spacing-trim " + quoteValue(value) + " was not applied at the " +
			"start of a line, so a full-width opening bracket keeps the half em " +
			"of blank in front of it",
		Path: PathOf(b.Element),
	})
}
