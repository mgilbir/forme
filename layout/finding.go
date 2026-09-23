// Package layout lays HTML and CSS out onto a page and says what it drew.
//
// What comes out is a display list — text, rectangles and pictures, in the
// page's own coordinates — and the findings beside it. Nothing here writes a
// file: a backend takes the ops and puts them somewhere, and everything above
// that line is the same whichever it is. See Compose.
//
// This file is the guardrail vocabulary, and it exists before the layout engine
// on purpose. The reporting layer landed *with* the engine rather than after
// it, because a reporting layer retrofitted onto a finished engine is how it
// becomes decorative. The engine grows into this, not the other way round.
//
// # The sections this package cites
//
// A bare "§5", "§6.1" or "§7.1" in this package, with no specification named
// beside it, is a section of the design this engine was planned from. That
// document is not in this repository, so what each section says is here:
//
//   - §5 is scale-to-fit: one geometric factor applied to the finished layout
//     (see fitScale), not a second layout at a smaller size.
//   - §6 is the guardrails as a whole: every way a page can be quietly wrong is
//     a named rule a caller can act on. §6.1 is the size thresholds (MinScale,
//     the minimum font size), §6.2 layout integrity (content outside its box or
//     off the page), §6.3 what the engine does not implement, and §6.5 that
//     every rule has a test which plants a violation and watches it fire.
//   - §7.1 is the reftest signal: a pass counts only when neither document
//     reported something unsupported (see the WPT harness).
//
// A section of a specification is cited with the specification's name, as
// "CSS 2.2 §10.3" or "HTML §4.8.7", or in a file that says which one it
// follows throughout.
//
// # What the guardrails are for
//
// Layout degrades *silently*. That is its characteristic failure and the whole
// argument of §6. A clipped paragraph, a 3pt caption and a heading full of tofu
// all produce a valid PDF that a caller has no programmatic way to distrust —
// the file opens, the text is selectable, nothing errors. So every way this
// engine can quietly produce a document that is not what was asked for is a
// named rule with an identifier, and a caller can decide for each one whether it
// is worth failing over.
package layout

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mgilbir/forme/html"
)

// Rule identifies a guardrail.
//
// The identifiers are the ones §6 names, and they are strings rather than an
// enumeration because they travel: they are what a caller matches on, what a
// configuration file names, and what a report is grouped by. An integer would be
// none of those.
type Rule string

// The rules this engine can currently report.
//
// This is deliberately *not* the whole catalogue of §6. The geometry rules —
// min-font-size, unbreakable-overflow, overflow-page and the rest — arrive with
// the layout that can violate them, each together with the test that plants a
// violation and watches it fire. §6.5 asks for exactly that, and declaring a
// rule before anything can raise it would produce the decoration it warns
// against: an identifier in a catalogue that has never been seen to fire proves
// nothing at all. TestEveryRuleIsReachable holds that line.
const (
	// RuleUnsupportedProperty is a declaration parsed and then not applied.
	// §6.3 argues this is the highest-value guardrail for the least cost, and
	// it is: an engine implementing a subset *will* ignore declarations, and a
	// page where flex-wrap was dropped is plausible and wrong.
	RuleUnsupportedProperty Rule = "unsupported-property"
	// RuleUnsupportedElement is an element the engine does not lay out.
	RuleUnsupportedElement Rule = "unsupported-element"
	// RuleUnsupportedSelector is a selector outside the implemented subset, so
	// the rule using it never applied.
	RuleUnsupportedSelector Rule = "unsupported-selector"
	// RuleUnsupportedAtRule is an at-rule the engine does not act on.
	RuleUnsupportedAtRule Rule = "unsupported-at-rule"
	// RuleUnsupportedValue is a value that is correct CSS the engine cannot
	// resolve — a unit needing font metrics, a colour in a space needing
	// conversion.
	RuleUnsupportedValue Rule = "unsupported-value"

	// RuleInvalidMarkup and RuleInvalidCSS are input the engine refused. They
	// are not the same as the unsupported rules and must not be reported as
	// them: one says the author wrote something wrong, the other says the
	// engine does not do something. An author sent to the wrong one of those
	// looks for the wrong thing.
	RuleInvalidMarkup Rule = "invalid-markup"
	RuleInvalidCSS    Rule = "invalid-css"

	// RuleFontFallback is a requested family that was not available, so the
	// text was set in something else. The metrics and the line breaks differ,
	// and nothing about the page says so.
	//
	// It is the family that could not be *resolved* — nobody has it, it did not
	// load, it is not in the set. A family that resolved and simply has no glyph
	// for the text is the rule below, and the two are separate because one is a
	// gap and the other is CSS working.
	RuleFontFallback Rule = "font-fallback"
	// RuleNoFace is the set having no face at all — not the requested family,
	// not the initial one, nothing.
	//
	// It is separate from RuleFontFallback because it is a different fact and a
	// different severity. A fallback is CSS working: the family asked for was
	// not there, another one set the text, and the page differs in its metrics.
	// This is the engine having nothing to set text with, so no text is drawn
	// at all — a page that is blank where its words should be, which is the one
	// outcome worth refusing outright.
	RuleNoFace Rule = "no-face"

	// RuleFontSubstituted is a family that resolved to a face with no glyph for
	// the text it was asked to set, so font matching went on to another face.
	//
	// It is deliberately *not* one of the unsupported rules, and the line
	// between it and the one above is where the engine stopped short against
	// where it did what CSS asks. A family nobody has is a gap: the page is set
	// in something the author never named and there was nothing better to
	// offer. A family that loaded and has no 国 in it is not a gap at all — CSS
	// Fonts §5 says to go on to the next face, every browser does, and so does
	// this.
	//
	// The distinction earns the second rule because §7.1's companion signal
	// reads it. A reftest whose two documents both fell back has not been made
	// vacuous by falling back: the substitution is the same on both sides, the
	// text is still drawn, and whatever the test is about — a line break, a
	// transform, a letter-spacing — is still being compared. That is the
	// opposite of a picture that failed to load, where the thing under test is
	// absent from both pages, and it is why that one is in the unsupported set
	// and this is not.
	//
	// It is still reported, and at Warn like the one above, because an author
	// who asked for a face and did not get it wants to know either way.
	RuleFontSubstituted Rule = "font-substituted"
	// RuleCapsSynthesised is small capitals this engine made out of the
	// capitals, because the face declares none of its own.
	//
	// CSS Fonts 4 §6.6 names the technique and leaves it optional: "if the font
	// does not support small-caps glyphs, the user agent may synthesize
	// small-caps by scaling uppercase glyphs". So a page that got them this way
	// is a page CSS asked for, which is why this is not one of the unsupported
	// rules — see the argument beside RuleFontSubstituted, which is the same
	// one. A reftest whose two documents both synthesised has not been made
	// vacuous by it: the capitals are on both pages, at the same size, and
	// whatever the test is about is still being compared.
	//
	// It is reported all the same, at Warn, and for two reasons that a
	// substituted font does not have. A designer's small capitals are drawn
	// with their own weight and spacing and a scaled capital is not, so an
	// author who chose a face for them and did not get them wants to know. And
	// the page carries the *uppercase* text: a reader copying a synthesised
	// line out of the PDF gets "FILLER" where the document said "Filler", which
	// is a consequence of the page rather than of the document and is exactly
	// what a finding is for. See layout/smallcaps.go.
	RuleCapsSynthesised Rule = "caps-synthesised"
	// RuleUnsupportedScript is text this engine cannot break or order
	// correctly. §6.3 makes it an error by default, and is right to: unbroken or
	// unordered text still looks like text, so the failure mode looks like
	// success.
	RuleUnsupportedScript Rule = "unsupported-script"
	// RuleGlyphMissing is a character no available face has a glyph for. Tofu is
	// the purest form of silent garbage — a box where a letter should be, which
	// a reader blames on their PDF viewer.
	RuleGlyphMissing Rule = "glyph-missing"

	// The size thresholds of §6.1, which are checkable exactly because §5's
	// scaling is geometric: the effective size of an element is its natural size
	// times one number, so a threshold is a multiplication rather than an
	// iteration.
	//
	// RuleMinScale is the blunt one and probably the most useful: if the content
	// had to be shrunk past half to fit, the document is wrong, and no
	// per-element threshold is needed to say so.
	RuleMinScale Rule = "min-scale"
	// RuleMinFontSize is text that would be set below a legible size.
	RuleMinFontSize Rule = "min-font-size"

	// The layout-integrity rules of §6.2, which are about the geometry rather
	// than about a size.
	//
	// RuleUnbreakableOverflow is atomic content wider than the box holding it: a
	// long URL, a nowrap run, an oversized image. §6.2 calls it the classic
	// silent clip, and it is one of two things: where something clips, the part
	// past the edge is not drawn; where nothing does — which is the initial
	// value of "overflow" and so the ordinary case — every glyph is on the page,
	// over whatever was beside it. The finding says which. See
	// layouter.overflowFate.
	RuleUnbreakableOverflow Rule = "unbreakable-overflow"
	// RuleTableColumnUnderflow is a table column narrower than the content in
	// it, so the content leaves the column.
	//
	// It is the table-shaped form of the rule above, and it has its own
	// identifier because it has its own cause and its own fix: the fixed table
	// layout of §17.5.2.1 deliberately ignores what is in the cells, so a column
	// can end up narrower than its content and the specification says so. That
	// is a trade an author may want and may not know they made — the table looks
	// tidy and a word is over the top of the next column.
	RuleTableColumnUnderflow Rule = "table-column-underflow"

	// RulePositionApproximated is a positioned box this engine placed by a
	// weaker rule than the one that applies to it.
	//
	// It is its own rule rather than an unsupported-value because the value *was*
	// supported: "position: absolute" was honoured, the box was taken out of the
	// flow and given offsets, and only the rectangle those offsets were measured
	// from is not the one §10.1 names. That produces the most deceptive shape of
	// wrongness this engine has — a box that is manifestly positioned, in a place
	// that looks deliberate, some tens of pixels from where the author put it.
	// Telling an author their declaration was ignored would send them looking for
	// a feature that is there.
	RulePositionApproximated Rule = "position-approximated"

	// RuleControlApproximated is a form control laid out as the static box a
	// printed page has, where that box is visibly not the widget a browser
	// would draw.
	//
	// It is its own rule rather than an unsupported-element because the element
	// *was* laid out: it has its size, its chrome and whatever text the markup
	// gave it, and it takes part in the flow like any other box. What is missing
	// is a widget — a slider's thumb at a position, the mark inside a checked
	// box, the options a drop-down is not showing — and every one of those is a
	// piece of information the document carries and the page does not.
	//
	// It fires only where the difference shows. A text field with a border round
	// its value is what a browser prints, so a text field says nothing; a rule
	// that fired on every control would be one nobody reads.
	RuleControlApproximated Rule = "control-approximated"

	// RuleOverflowPage is content outside the page box after scaling.
	//
	// It should be unreachable given §5: the scale is computed so that
	// everything fits. So it is a self-check as much as a guardrail — if it
	// fires, the scale computation is wrong, which is worth hearing about far
	// more than the overflow itself.
	RuleOverflowPage Rule = "overflow-page"

	// RuleResourceBlocked is a file the document referred to and this engine did
	// not load: there was no resolver, the reference named a URL scheme, it
	// pointed outside the directory the resolver was rooted at, or it could not
	// be read.
	//
	// It is its own rule rather than an unsupported-element, because the element
	// *was* laid out — an <img> whose image did not arrive is still a box, still
	// takes part in the line, and still shows its alt text. What is missing is
	// the picture, and a page with a rectangle of nothing where a chart belongs
	// is the silent failure §6 is named after.
	RuleResourceBlocked Rule = "resource-blocked"
	// RuleImageUndecodable is a resource that was loaded and did not become an
	// image: a format this engine has no decoder for, bytes that do not parse,
	// or a picture larger than it will decode.
	//
	// The last of those is the one worth having a rule for. A ten-kilobyte PNG
	// may declare sixty thousand pixels on a side, and refusing it is the only
	// safe answer — but the refusal has to be visible, or a document that a
	// caller believes contains a photograph contains a gap instead.
	RuleImageUndecodable Rule = "image-undecodable"
	// RuleFontUndecodable is a font an @font-face named, that arrived, and that
	// did not become a face: a container this engine does not unwrap, a format
	// hint naming one, or bytes that are not a font program.
	//
	// It is separate from RuleResourceBlocked for the same reason
	// image-undecodable is: "the file did not arrive" and "the file arrived and
	// was not usable" send an author to different places, and a font is the
	// case where the second is most likely — the web serves woff2 to everything
	// and this engine reads sfnt.
	RuleFontUndecodable Rule = "font-undecodable"

	// RuleLimit is a resource guard that tripped, or a run that was cancelled.
	//
	// "limit" and not "truncated" or "budget", and deliberately so: a caller
	// that distinguishes "the input is bad" from "the engine could not finish"
	// wants one identifier for the second, and every guard in this repository
	// that stops short reports under this one.
	//
	// It is spelled to match the validators this engine's findings collect
	// beside — see Finding, which is shaped for the same reason.
	RuleLimit Rule = "limit"
)

// Severity is what a rule does when it fires.
type Severity uint8

const (
	// Ignore drops the finding entirely. It is not the default for anything —
	// a caller has to ask for silence.
	Ignore Severity = iota
	// Warn records the finding and lets the render finish.
	Warn
	// Error records the finding and makes the render fail, so no document is
	// returned. This is for the cases where a produced document would be worse
	// than none: one that looks finished and is not.
	Error
)

func (s Severity) String() string {
	switch s {
	case Ignore:
		return "ignore"
	case Warn:
		return "warn"
	case Error:
		return "error"
	}
	return "unknown"
}

// defaultSeverity is what each rule does unless a caller says otherwise.
//
// Most are Warn: an unsupported property produces a page that is wrong in a way
// the author can see and decide about. The two that are Error are the ones §6.3
// names, where the wrongness is invisible — text in the wrong order, or a row of
// boxes where letters should be, both of which a reader blames on their viewer
// rather than on the document. The remaining Error defaults belong to the size
// thresholds of §6.1 and arrive with the layout that can produce them.
var defaultSeverity = map[Rule]Severity{
	RuleUnsupportedProperty: Warn,
	RuleUnsupportedElement:  Warn,
	RuleUnsupportedSelector: Warn,
	RuleUnsupportedAtRule:   Warn,
	RuleUnsupportedValue:    Warn,
	RuleFontFallback:        Warn,
	RuleFontSubstituted:     Warn,
	// Synthesised small capitals warn for the reason the rule's declaration
	// gives: the page is what CSS asked for and is not what the author chose a
	// face for, and it carries the uppercase text.
	RuleCapsSynthesised: Warn,
	// The two errors. Both produce a page that looks finished and is not, which
	// is the case where returning no document is better than returning one.
	RuleUnsupportedScript: Error,
	RuleGlyphMissing:      Error,
	// A document that only fitted by being made illegible is one where no
	// document is better than the document.
	RuleMinScale:    Error,
	RuleMinFontSize: Error,
	// A clip nobody asked for removes content from the page, which is the
	// failure §6.2 is named after.
	RuleUnbreakableOverflow: Error,
	// A clipped column warns rather than failing, and the difference from the
	// rule above is who asked for it. "table-layout: fixed" is a declaration
	// that says in as many words "lay this table out without looking at what is
	// in it", so a column too narrow for its content is the author's arrangement
	// working as specified — worth being told about, not worth refusing to
	// produce a document over.
	RuleTableColumnUnderflow: Warn,
	// A box in the wrong place is visible, and the author can see where it
	// landed — which is why this warns rather than failing the render. The
	// argument for Error is that the page is plausible and wrong; the argument
	// against is that the case it fires on, an absolutely positioned box inside
	// a relatively positioned inline, is the most common tooltip idiom on the
	// web, and a default that refuses to produce a document for it would be
	// turned off wholesale and take the rest of the catalogue with it.
	RulePositionApproximated: Warn,
	// A control drawn as a box is visible and the reader can see what is there;
	// what they cannot see is what is missing, which is why this is reported at
	// all. It warns rather than failing for the same reason the rule above does:
	// refusing to produce a document because a page has a checkbox on it would
	// be a default turned off wholesale.
	RuleControlApproximated: Warn,
	// A missing image is visible: the page has a gap where the picture was, and
	// the alt text says what it was of. That is why these warn rather than
	// failing the render — and why a caller producing invoices with a logo on
	// them should raise both to Error, which is the case the policy exists for.
	RuleResourceBlocked:  Warn,
	RuleImageUndecodable: Warn,
	// A font that did not load is a page set in something else, which is
	// visible and which RuleFontFallback then says out loud where the family
	// was used. Refusing to produce the document over it would be a default
	// turned off wholesale by anyone whose fonts are woff2.
	RuleFontUndecodable: Warn,
	// Nothing to set text in is not a degraded page, it is an empty one. A
	// caller shown a blank sheet with no finding on it has no way to tell that
	// from a document that said nothing.
	RuleNoFace: Error,
	// A self-check: this firing means the scale computation is wrong, and a
	// document produced from a wrong scale is worse than none.
	RuleOverflowPage:  Error,
	RuleInvalidMarkup: Warn,
	RuleInvalidCSS:    Warn,
	RuleLimit:         Warn,
}

// AllRules returns every rule this engine can report, in a fixed order.
//
// It exists so that a caller can enumerate what it might be told, and so that
// the tests can require each one to have been seen to fire.
func AllRules() []Rule {
	out := make([]Rule, 0, len(defaultSeverity))
	for r := range defaultSeverity {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Policy is a caller's choice of severity per rule. A rule absent from it keeps
// its default.
type Policy map[Rule]Severity

// severityOf returns the severity a policy gives a rule.
func (p Policy) severityOf(r Rule) Severity {
	if s, ok := p[r]; ok {
		return s
	}
	if s, ok := defaultSeverity[r]; ok {
		return s
	}
	// A rule with no default is one nothing declared. Warning is the safe
	// answer: silence would hide it, and failing would make adding a rule a
	// breaking change.
	return Warn
}

// Source says where in the input a finding came from.
//
// Both offsets are byte offsets into the document the caller supplied, and both
// are -1 when they do not apply. That is what lets a caller point an author at
// the markup or the stylesheet rather than at a description of it, and it cannot
// be recovered afterwards — which is why the html, css and style packages carry
// offsets at all.
type Source struct {
	// HTMLOffset is a byte offset into the HTML, or -1.
	HTMLOffset int
	// CSSOffset is a byte offset into the stylesheet, or -1.
	CSSOffset int
	// Sheet names which stylesheet CSSOffset is in, when there is more than
	// one. It is empty for the document's own.
	Sheet string
}

// NoSource is a finding that is not tied to a place in the input.
var NoSource = Source{HTMLOffset: -1, CSSOffset: -1}

// placed is the source a finding is recorded and rendered with: s, unless s is
// the zero Source, which is no place and is read as NoSource.
//
// The zero value says "byte nought of the markup and byte nought of a
// stylesheet", which no finding is — a finding is in one input or in none —
// and it is what a Finding literal written without a Source gets. A finding
// about the whole document, which is in no file, is written without one and
// means exactly that. There were
// thirty of those: invalid options, the @page geometry, the scale and
// font-size floors, a failed @import, the text checks. Each rendered as
// "[html byte 0]" and sent an author to the top of the file for something
// that was not there (audit C87). A finding really at the first byte of the
// markup is AtHTML(0), whose CSS offset is -1, and is left where it is.
func (s Source) placed() Source {
	if s.HTMLOffset == 0 && s.CSSOffset == 0 {
		return Source{HTMLOffset: -1, CSSOffset: -1, Sheet: s.Sheet}
	}
	return s
}

// sourceOf is where an element was written, or NoSource for none — the source
// of a finding about a box, to go with PathOf's path.
func sourceOf(n *html.Node) Source {
	if n == nil {
		return NoSource
	}
	return AtHTML(n.Offset)
}

// AtHTML and AtCSS build the two common cases.
func AtHTML(offset int) Source { return Source{HTMLOffset: offset, CSSOffset: -1} }

func AtCSS(offset int) Source { return Source{HTMLOffset: -1, CSSOffset: offset} }

// Finding is one guardrail firing.
//
// It satisfies a Violation interface — error, RuleID and ObjectNum — so that a
// consumer collecting findings from several stages puts these in the same slice
// as the rest. That interface belongs to whatever consumes a render and is
// deliberately not imported: this package does not depend on the one that
// documents it, and satisfying it structurally is what keeps that true.
//
// Which is why the three methods are pinned by a test that declares the shape
// locally. See TestFindingSatisfiesViolation.
//
// ObjectNum is always 0, which the interface already documents as "not tied to a
// specific object": a layout finding is about a paragraph in the source, not
// about an object in the file it became.
type Finding struct {
	// Rule is which guardrail fired.
	Rule Rule
	// Severity is what it did, after the caller's policy was applied.
	Severity Severity
	// Message says what happened, in terms of the author's input.
	Message string
	// Source is where in the input it happened.
	Source Source

	// Path is the DOM path of the element concerned, such as
	// "html > body > div > p", or empty. It is what makes a finding actionable
	// when the offset points at a stylesheet shared by many elements.
	Path string
	// Selector is the selector responsible, or empty.
	Selector string
	// Property is the declaration responsible, or empty.
	Property string
}

// Error renders the finding for a person, leading with the rule so that a list
// of them can be read down.
func (f Finding) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", f.Rule, f.Message)
	if f.Path != "" {
		fmt.Fprintf(&b, " (at %s)", f.Path)
	}
	switch src := f.Source.placed(); {
	case src.HTMLOffset >= 0:
		fmt.Fprintf(&b, " [html byte %d]", src.HTMLOffset)
	case src.CSSOffset >= 0:
		if src.Sheet != "" {
			fmt.Fprintf(&b, " [%s byte %d]", src.Sheet, src.CSSOffset)
		} else {
			fmt.Fprintf(&b, " [css byte %d]", src.CSSOffset)
		}
	}
	return b.String()
}

// unsupportedRules are the rules that say "this engine does not do that", as
// against the ones that say "the input is wrong" or "we stopped short".
//
// The distinction is what §7.1's companion signal needs: a reftest passes
// vacuously when the engine ignores the same thing in both documents, and the
// only way to tell that apart from a real pass is to know whether anything went
// unimplemented.
var unsupportedRules = map[Rule]bool{
	RuleUnsupportedProperty: true,
	RuleUnsupportedElement:  true,
	RuleUnsupportedSelector: true,
	RuleUnsupportedAtRule:   true,
	RuleUnsupportedValue:    true,
	RuleUnsupportedScript:   true,
	RuleFontFallback:        true,
	// RuleFontSubstituted is not here, and must not be added. See its
	// declaration: a family that resolved and lacks the glyph is §5's font
	// matching working, not something this engine declined to do, and the two
	// documents of a reftest that both went through it are still comparing the
	// thing they are about.
	//
	// RuleCapsSynthesised is not here either, and for the same reason twice
	// over: §6.6 names the synthesis as a thing a user agent may do, so a page
	// that got its small capitals that way is a page CSS asked for — and the
	// capitals are on both documents of a reftest, at the same size, so nothing
	// about the comparison has been made vacuous.
	RuleGlyphMissing: true,
	// This one says "the engine does not form that containing block", which is a
	// statement about the engine and not about the input — so a reftest whose
	// two documents both trip it has not demonstrated anything, and §7.1's
	// companion signal has to see it.
	RulePositionApproximated: true,
	// And this one says "the page does not show a widget the document has", for
	// which the same argument holds: two documents that both draw a slider as an
	// empty box agree about a picture neither of them drew.
	RuleControlApproximated: true,
	// These two say "the page does not show everything the document says",
	// which is not quite "this engine does not implement that" — a corrupt PNG
	// is the input being wrong. They are here anyway, and the reason is the one
	// §7.1 gives: a reftest whose two documents both failed to load their image
	// paint two blank rectangles that match perfectly and demonstrate nothing.
	// The companion signal has to see a document that did not draw what it was
	// asked to, whichever side the fault was on.
	RuleResourceBlocked:  true,
	RuleImageUndecodable: true,
}

// Unsupported reports whether the finding is about something this engine does
// not implement.
func (f Finding) Unsupported() bool { return unsupportedRules[f.Rule] }

// RuleID is the identifier of the violated rule, for the Violation interface.
func (f Finding) RuleID() string { return string(f.Rule) }

// ObjectNum is 0: a layout finding is not tied to a PDF object.
func (f Finding) ObjectNum() int { return 0 }

// maxFindings bounds one render's report. A document that trips a rule on every
// element would otherwise produce a list nobody can read, and a report nobody
// reads is not a report.
//
// A var and not a const so that a test can lower it, which is the only way to
// reach the bound: the recorder deduplicates hard enough that a document cannot
// honestly produce five hundred distinct findings, and the code that runs when
// the list fills is worth a test rather than a reading. It is unexported and
// nothing outside this package's tests writes it — a bound, not a knob.
var maxFindings = 500

// Recorder collects findings under a policy.
//
// One render's recorder is its own. Nothing here takes a lock, because nothing
// in this package starts a goroutine: a Recorder is written to from the one that
// called Build or Layout, and two renders at once are two recorders. Sharing one
// across them is a data race, and it is the caller's to avoid — which is worth
// stating, because the type reads like a collector something might hand around.
//
// It applies the policy at the point of recording rather than at the end, so a
// rule set to Ignore costs nothing to raise — which matters because the callers
// are the inner loops of layout, and a guardrail that is expensive to check is
// one that gets checked less often than it should.
type Recorder struct {
	policy Policy

	findings []Finding
	// counts is how many times each rule fired, including the ones dropped for
	// being duplicates or past the bound. It is what lets a report say "and 4000
	// more" rather than implying there were 500.
	counts map[Rule]int
	// seen suppresses repeats of the same rule and message. A stylesheet using
	// one unimplemented property four hundred times is one thing to be told.
	//
	// It holds a digest of each finding and not the finding's text, and only
	// of the findings in the list. Both were otherwise, and both were the
	// same mistake — a memo the bound on the list did not bound. The key was
	// the text: every distinct finding's path, message and sheet name,
	// concatenated and kept, and a path is as long as the document makes its
	// ids. Two hundred and fifty nested <div>s with two-thousand-character ids
	// gave each of three thousand leaves a half-megabyte path, and 608 KB of
	// markup held two gigabytes of keys (audit C18). And it was built before
	// it was looked up, so a finding that was a duplicate still copied its
	// sheet's name, which for a data: stylesheet was the whole stylesheet.
	seen map[findingDigest]bool
	// failed records that something fired at Error severity.
	failed bool
	// truncated records that the bound was reached.
	truncated bool
	// unsupported is how many of findings are Unsupported. See record:
	// the bound is not allowed to leave it at zero when one was raised.
	unsupported int

	// work is the document's work budget. See budget.go for why it lives here:
	// the recorder is the one object every stage of a render is handed, and
	// its lifetime is exactly one render.
	work workBudget
}

// findingDigest is a finding's identity for deduplication: a SHA-256 digest of
// the fields a reader tells two findings apart by, cut to 128 bits.
//
// A cryptographic digest rather than a fast hash, because a collision here is
// a finding silently dropped, and the fields are the document's — an author who
// could make two findings collide could hide one behind the other. At 128 bits
// that takes a collision attack on SHA-256, which is not a thing a stylesheet
// can mount.
type findingDigest [16]byte

// NewRecorder prepares to collect findings under a policy. A nil policy uses the
// defaults.
//
// The recorder carries the render's work budget, so a new recorder is a new
// allowance: Build and Compose make one per document, and so should a caller
// that runs the stages itself.
func NewRecorder(p Policy) *Recorder {
	return &Recorder{
		policy: p,
		counts: map[Rule]int{},
		seen:   map[findingDigest]bool{},
		work:   newWorkBudget(),
	}
}

// Report records a finding, applying the policy.
//
// It reports whether the finding was at Error severity, which is what a caller
// in a position to stop early wants to know.
func (r *Recorder) Report(rule Rule, src Source, message string) bool {
	return r.ReportDetail(Finding{Rule: rule, Source: src, Message: message})
}

// ReportDetail records a finding that carries more than a message.
//
// The Severity field of f is ignored — it is filled in from the policy, because
// a caller raising a finding should not be able to decide how serious it is.
// That decision belongs to whoever is rendering.
func (r *Recorder) ReportDetail(f Finding) bool {
	return r.record(f, true)
}

// record is ReportDetail, with the charge to the work budget made optional for
// the one finding that cannot pay it: the budget's own. See Recorder.refuse.
func (r *Recorder) record(f Finding, charged bool) bool {
	severity := r.policy.severityOf(f.Rule)
	r.counts[f.Rule]++
	if severity == Ignore {
		return false
	}
	if severity == Error {
		r.failed = true
	}
	f.Severity = severity
	f.Source = f.Source.placed()

	// What deduplicating costs is reading the finding once, so that is what
	// is charged. It is the only work here that grows with the document, and
	// the stages that raise findings do so from their inner loops. A finding
	// refused is still counted and still decides whether the render failed —
	// both happened above — and the list says it is not the whole story.
	if charged && !r.charge(int64(len(f.Message)+len(f.Path)+len(f.Property)+
		len(f.Selector)+len(f.Source.Sheet))*costFindingByte, "some findings") {
		r.truncated = true
		return severity == Error
	}

	// Deduplicate on everything a reader would use to tell two findings apart.
	// Two identical messages about two different elements are two findings; two
	// identical messages about the same place are one.
	//
	// The *file* is one of those things and the offset is not. A stylesheet that
	// uses one unimplemented property four hundred times is one thing to be told
	// and four hundred offsets to be told it at, which is what the count beside
	// the list is for — but the same mistake in two stylesheets is two mistakes,
	// in two files, and the second was silently dropped for having the same
	// words as the first. An author fixing the one they were shown found the
	// finding still there.
	key := keyOf(f)
	if r.seen[key] {
		return severity == Error
	}
	// Once the list is full nothing more is remembered. A finding that is not
	// a repeat of one in the list is one the list does not hold, which is all
	// the truncation flag has to know, and remembering it would be a memo that
	// grows past the bound on the thing it deduplicates.
	if len(r.findings) >= maxFindings {
		r.truncated = true
		// Whether a page is clean is read off this list — a finding that is
		// Unsupported says the page lacks something, and the WPT ratchet and
		// any caller like it count on seeing one — so the bound may cut how
		// many there are and not whether there are any. Audit C58 found the
		// styling stage's own bound doing exactly that. If the list holds
		// none, the first one past the bound takes the last place: a list
		// that says Truncated is already missing findings, and one more
		// missing that is not Unsupported costs a reader less than a page
		// with nothing unsupported on it that has something.
		if f.Unsupported() && r.unsupported == 0 && len(r.findings) > 0 {
			r.findings[len(r.findings)-1] = f
			r.unsupported++
		}
		return severity == Error
	}
	r.seen[key] = true
	if f.Unsupported() {
		r.unsupported++
	}
	r.findings = append(r.findings, f)
	return severity == Error
}

// keyOf is a finding's deduplication key. See findingDigest.
//
// The fields are written into the digest one at a time and separated, rather
// than concatenated first: the concatenation was the copy of the whole sheet
// name that a duplicate paid for.
func keyOf(f Finding) findingDigest {
	h := sha256.New()
	for _, s := range [...]string{string(f.Rule), f.Message, f.Path, f.Property,
		f.Selector, f.Source.Sheet} {
		// The length first, so that no two different lists of fields write
		// the same bytes: "a" then "bc" is not "ab" then "c".
		var n [8]byte
		binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		io.WriteString(h, s)
	}
	var key findingDigest
	copy(key[:], h.Sum(nil))
	return key
}

// Findings returns what was recorded, in a deterministic order.
//
// The order is by rule, then by where in the input the finding came from, then
// by message. Two runs over the same document must produce the same slice, and
// several of the stages above range over maps, so the order is imposed here
// rather than left to whatever the walk happened to do.
func (r *Recorder) Findings() []Finding {
	out := append([]Finding(nil), r.findings...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Source.HTMLOffset != b.Source.HTMLOffset {
			return a.Source.HTMLOffset < b.Source.HTMLOffset
		}
		if a.Source.CSSOffset != b.Source.CSSOffset {
			return a.Source.CSSOffset < b.Source.CSSOffset
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Message < b.Message
	})
	return out
}

// Failed reports whether anything fired at Error severity.
func (r *Recorder) Failed() bool { return r.failed }

// Truncated reports whether the bound stopped findings being recorded, so a
// caller never presents a cut list as a complete one.
func (r *Recorder) Truncated() bool { return r.truncated }

// Count returns how many times a rule fired, including occurrences that were
// deduplicated or dropped past the bound.
//
// This is what lets a report say "flex-wrap was dropped 412 times" while showing
// the finding once, which is more useful than either the one or the four hundred
// on their own.
func (r *Recorder) Count(rule Rule) int { return r.counts[rule] }

// Counts is every rule that fired and how often, copied so that a caller
// holding it cannot change what the recorder goes on counting.
func (r *Recorder) Counts() map[Rule]int {
	out := make(map[Rule]int, len(r.counts))
	for rule, n := range r.counts {
		out[rule] = n
	}
	return out
}
