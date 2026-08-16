package layout

import (
	"strings"

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
// marks, before centred punctuation — and this engine's whole CJK rule is
// "between two ideographs", which all three leave alone. Over Latin text the
// three values provably change nothing, and the suite says so: pre-wrap-004,
// -005 and -006 exist to assert that "XX    XX" wraps the same under loose,
// normal and strict as under auto. Warning there would be crying wolf on a page
// that is correct.
//
// So the report is made where the difference could show, which is text with an
// ideograph in it — the only text this engine breaks by a rule the three values
// have anything to say about.
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
			"value asked it not to",
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
