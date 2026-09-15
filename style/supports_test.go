package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// styledBy applies a sheet to one element and answers what colour it ended up
// with, which is how these tests tell an applied block from a dropped one.
func styledBy(t *testing.T, src string) (string, []Finding) {
	t.Helper()
	doc := parseDoc(t, `<p id="target">x</p>`)
	got := Apply(doc, []Sheet{author(t, `#target { color: blue } `+src)})
	return got.Styles[doc.Element("p")]["color"], got.Findings
}

// TestASupportsBlockAppliesWhenTheEngineUnderstandsTheDeclaration is the half
// that was missing: a document written with a fallback outside the block and the
// better version inside it got the fallback, whatever this engine could do.
func TestASupportsBlockAppliesWhenTheEngineUnderstandsTheDeclaration(t *testing.T) {
	for _, c := range []struct {
		what, condition string
		applied         bool
	}{
		// A property the engine implements.
		{"an implemented property", `(color: red)`, true},
		{"a property it does not", `(transform: rotate(1deg))`, false},
		{"a property nothing has ever heard of", `(zzz-not-a-property: 1)`, false},
		// A custom property is supported by anything that parses CSS, which §2
		// says in as many words.
		{"a custom property", `(--brand: red)`, true},

		// not
		{"not, of something it has", `not (color: red)`, false},
		{"not, of something it has not", `not (transform: rotate(1deg))`, true},

		// and
		{"and, both known", `(color: red) and (display: block)`, true},
		{"and, one unknown", `(color: red) and (transform: rotate(1deg))`, false},

		// or
		{"or, one known", `(transform: rotate(1deg)) or (color: red)`, true},
		{"or, neither known", `(transform: rotate(1deg)) or (filter: blur(1px))`, false},

		// Grouping, which is the only way "and" and "or" may be mixed.
		{"a group", `((color: red) or (transform: x)) and (display: block)`, true},
		{"a group that fails", `((transform: x) or (filter: y)) and (color: red)`, false},

		// A declaration with no value does not parse, so nothing supports it.
		{"an empty value", `(color:)`, false},
	} {
		t.Run(c.what, func(t *testing.T) {
			colour, _ := styledBy(t, `@supports `+c.condition+` { #target { color: red } }`)
			want := "blue"
			if c.applied {
				want = "red"
			}
			if colour != want {
				t.Errorf("@supports %s left the colour %q, want %q (%q means the "+
					"block was applied)", c.condition, colour, want, "red")
			}
		})
	}
}

// TestASupportsConditionThatIsFalseSaysNothing.
//
// The block is the version an author wrote for an engine that understands the
// declaration, and the fallback they wrote outside it is what this page gets.
// Reporting that would be reporting the rule working, on every stylesheet that
// uses one.
func TestASupportsConditionThatIsFalseSaysNothing(t *testing.T) {
	_, findings := styledBy(t, `@supports (transform: rotate(1deg)) { #target { color: red } }`)
	for _, f := range findings {
		if f.Property == "@supports" {
			t.Errorf("a condition this engine answered reported %q; the answer is "+
				"the rule working", f.Message)
		}
	}
}

// TestASupportsConditionItCannotReadIsReported is the other half of the same
// rule, and the reason it is not silence everywhere.
//
// selector() and font-tech() ask about facilities rather than about a
// declaration. A browser printing the same document may apply rules this page
// does not have, which is exactly what a media query naming an unanswerable
// feature is reported for.
func TestASupportsConditionItCannotReadIsReported(t *testing.T) {
	for _, condition := range []string{
		`selector(p > a)`,
		`font-tech(color-COLRv1)`,
		`(color: red) and selector(p)`,
		// Not a condition at all: §2 requires the parentheses, and a bare
		// declaration is a shape this cannot read rather than one it answers.
		`color: red`,
		`red`,
		`(color: red) (display: block)`,
	} {
		colour, findings := styledBy(t, `@supports `+condition+` { #target { color: red } }`)
		if colour != "blue" {
			t.Errorf("@supports %s applied its block; a condition this engine "+
				"cannot read is not a licence to apply it", condition)
		}
		found := false
		for _, f := range findings {
			if f.Property == "@supports" && f.Unsupported {
				found = true
			}
		}
		if !found {
			t.Errorf("@supports %s was dropped without a word; a browser printing "+
				"this document may apply rules this page does not have", condition)
		}
	}
}

// TestNotOfAnUnreadableConditionIsStillUnreadable.
//
// An unreadable condition is false, and the negation of a thing that could not
// be read is not true — it is still unread. Saying otherwise would turn
// "not (something unanswerable)" into a licence to apply the block, which is
// the one direction this must not err in.
func TestNotOfAnUnreadableConditionIsStillUnreadable(t *testing.T) {
	colour, findings := styledBy(t, `@supports not selector(p) { #target { color: red } }`)
	if colour != "blue" {
		t.Error("\"not\" of a condition this engine cannot read applied the block")
	}
	found := false
	for _, f := range findings {
		if f.Property == "@supports" {
			found = true
		}
	}
	if !found {
		t.Error("it was dropped without a word")
	}
}

// TestASupportsBlockCascadesWhereItIsWritten.
//
// §3: the rules inside add no specificity and no priority, so they are ordered
// among their neighbours by where the block sits. A block that wins over a rule
// above it must lose to one below it.
func TestASupportsBlockCascadesWhereItIsWritten(t *testing.T) {
	colour, _ := styledBy(t,
		`@supports (color: red) { #target { color: red } } #target { color: green }`)
	if colour != "green" {
		t.Errorf("a rule written after the block lost to it, leaving %q; an "+
			"@supports adds no priority of its own", colour)
	}
}

// TestASupportsConditionNamingAnUnappliedValueIsStillYes states the narrowing,
// because it is the one place this answers differently from a browser.
//
// §2 tests whether the declaration would parse, and this engine has no single
// place that says whether a value parses — that is decided per property, by the
// stage that reads it. So a condition is answered about the property, and
// "(position: sticky)" is yes where position is implemented and sticky is not.
//
// It is sound rather than merely convenient, and the reason is where the report
// goes: the block let in by this answer holds the declaration itself, and a
// value this engine cannot act on is reported by the stage that could not act
// on it — for sticky, by layout rather than here. What changes is that the rest
// of the block is applied rather than dropped along with it.
//
// The other half of that claim is a layout test, because that is where the
// finding lands: TestASupportsBlockLetsThroughADeclarationThatStillReports.
func TestASupportsConditionNamingAnUnappliedValueIsStillYes(t *testing.T) {
	colour, _ := styledBy(t,
		`@supports (position: sticky) { #target { color: red } }`)
	if colour != "red" {
		t.Errorf("the block was dropped, leaving %q; a condition is answered about "+
			"the property, and the declaration inside reports its own value",
			colour)
	}
}

// TestSupportsAnswersNoForARegisteredButUnimplementedProperty pins the half of
// the oracle that cannot fire today.
//
// unimplementedProperties is empty, so every registered property is an
// implemented one and this iterates over nothing. It is here for the state the
// engine has been in within the day — a property registered so that its value
// cascades, before anything reads it — because that is when an @supports
// condition would start answering yes about a property the very next
// declaration is reported for.
//
// A vacuous assertion is worth keeping only when the thing it waits for is a
// state the code can return to, and this one is: the registry and the
// implemented set are separate lists by design.
func TestSupportsAnswersNoForARegisteredButUnimplementedProperty(t *testing.T) {
	red := declValue(t, "red")
	for name := range unimplementedProperties {
		if supportsDeclaration(name, red) {
			t.Errorf("@supports answers yes for %q, which the cascade reports as "+
				"not implemented; the two read the same pair of facts", name)
		}
	}
	// And the control, so that an empty loop is not the whole test: a property
	// that is implemented answers yes.
	if !supportsDeclaration("color", red) {
		t.Error("@supports answers no for color, which is implemented")
	}
}

// declValue parses a value the way a declaration's would be.
func declValue(t *testing.T, src string) []css.ComponentValue {
	t.Helper()
	vals, errs := css.ParseComponentValues(src)
	if len(errs) != 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	return vals
}
