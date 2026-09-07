package layout

import (
	"path/filepath"
	"strings"
	"testing"
)

// The edges of @import, and of the list of findings it produces.

// TestAnImportCycleIsReportedAndNotFollowed.
//
// A ring of sheets is not stopped by the cache: the cache is what makes the
// second read succeed. It ran until the document-wide count refused the
// twenty-first sheet, applied the two sheets ten times each, and said nothing —
// the refusal on the cached path returns without a finding.
func TestAnImportCycleIsReportedAndNotFollowed(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "a.css"), `@import "b.css"; p { color: rgb(1, 2, 3) }`)
	writeCSS(t, filepath.Join(dir, "b.css"), `@import "a.css";`)

	built := buildLinking(t, dir, `<link rel=stylesheet href="a.css"><p id=p>x</p>`)
	if got := colourOf(t, built, "p"); got != wantColour {
		t.Errorf("the colour is %q; the sheets in the ring are still applied once", got)
	}
	var said string
	for _, f := range built.Findings {
		if strings.Contains(f.Message, "imports itself") {
			said = f.Message
		}
	}
	if said == "" {
		t.Fatalf("a ring of two stylesheets was followed and nothing said so: %v",
			built.Findings)
	}
	// The chain and not just the fact, because "a.css imports itself" and
	// "a.css imports b.css imports a.css" are different things to go and find.
	if !strings.Contains(said, "a.css imports b.css imports a.css") {
		t.Errorf("the finding is %q; it should name the chain", said)
	}
}

// TestASelfImportIsReported is the shortest ring.
func TestASelfImportIsReported(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "a.css"), `@import "a.css"; p { color: rgb(1, 2, 3) }`)
	built := buildLinking(t, dir, `<link rel=stylesheet href="a.css"><p id=p>x</p>`)
	if got := colourOf(t, built, "p"); got != wantColour {
		t.Errorf("the colour is %q; the sheet is still applied once", got)
	}
	if !hasFinding(built.Findings, "imports itself") {
		t.Errorf("a sheet importing itself was followed and nothing said so: %v",
			built.Findings)
	}
}

// TestAnImportInAUserStylesheetIsFollowed. A user stylesheet is CSS a person
// wrote, and an @import in one is the same request it is anywhere else — left
// unexpanded it was reported as an at-rule this engine does not apply, which is
// not what happens to the identical line in the document's own sheet.
func TestAnImportInAUserStylesheetIsFollowed(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "user-inner.css"), "p { color: rgb(1, 2, 3) }")
	res, err := NewDirResolver(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Close() })

	built := Build(Input{
		HTML:      `<p id=p>x</p>`,
		UserCSS:   `@import "user-inner.css";`,
		Resources: res,
	})
	if got := colourOf(t, built, "p"); got != wantColour {
		t.Errorf("the colour is %q; an @import in the user stylesheet names a "+
			"sheet like any other", got)
	}
	if hasFinding(built.Findings, "@import") {
		t.Errorf("the @import was applied and still reported: %v", built.Findings)
	}
}

// TestAnImportIsFoundWhateverCaseItIsWrittenIn. An at-rule's name is
// case-insensitive, and the fast path in front of the parse was not — so a
// sheet spelling it "@IMPORT" was handed to the cascade with the rule still in
// it, to be reported as an at-rule nothing applied.
func TestAnImportIsFoundWhateverCaseItIsWrittenIn(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "inner.css"), "p { color: rgb(1, 2, 3) }")
	writeCSS(t, filepath.Join(dir, "outer.css"), `@IMPORT "inner.css";`)

	built := buildLinking(t, dir, `<link rel=stylesheet href="outer.css"><p id=p>x</p>`)
	if got := colourOf(t, built, "p"); got != wantColour {
		t.Errorf("the colour is %q; \"@IMPORT\" is an @import", got)
	}
}

// TestTheSameMistakeInTwoSheetsIsTwoFindings.
//
// The list is deduplicated so that a stylesheet using one unimplemented
// property four hundred times is one thing to be told. Which file it is in was
// not part of what made two findings different, so the same mistake in a second
// sheet was dropped for having the same words — and an author who fixed the one
// they were shown found the finding still there.
func TestTheSameMistakeInTwoSheetsIsTwoFindings(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "one.css"), "p { mix-blend-mode: multiply }")
	writeCSS(t, filepath.Join(dir, "two.css"), "div { mix-blend-mode: multiply }")

	built := buildLinking(t, dir,
		`<link rel=stylesheet href="one.css"><link rel=stylesheet href="two.css">`+
			`<p id=p>x</p>`)
	sheets := map[string]bool{}
	for _, f := range built.Findings {
		if strings.Contains(f.Message, "mix-blend-mode") {
			sheets[f.Source.Sheet] = true
		}
	}
	if len(sheets) != 2 {
		t.Errorf("the same mistake in two stylesheets was reported for %d of them "+
			"(%v); an author fixing the one they were shown finds the other", len(sheets), sheets)
	}
}

// TestOneMistakeManyTimesInOneSheetIsOneFinding is the control, and the reason
// the offset is not part of what tells two findings apart.
func TestOneMistakeManyTimesInOneSheetIsOneFinding(t *testing.T) {
	dir := cssDir(t)
	var css strings.Builder
	for i := 0; i < 40; i++ {
		css.WriteString("p { color: rgb() }\n")
	}
	writeCSS(t, filepath.Join(dir, "one.css"), css.String())
	built := buildLinking(t, dir, `<link rel=stylesheet href="one.css"><p id=p>x</p>`)
	n := 0
	for _, f := range built.Findings {
		if f.Rule == RuleInvalidCSS {
			n++
		}
	}
	if n != 1 {
		t.Errorf("one mistake written forty times in one sheet gave %d findings, want 1", n)
	}
}

func hasFinding(fs []Finding, substr string) bool {
	for _, f := range fs {
		if strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}
