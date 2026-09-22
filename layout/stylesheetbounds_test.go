package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// The token bounds on stylesheets, applied to every source a stylesheet comes
// from rather than only to the ones that are fetched.
//
// Each case sets a bound so that it falls between two sheets it can tell apart,
// and requires the one inside it to apply and the one past it not to — a bound
// that refused everything would satisfy half of each, and one that refused
// nothing the other half. The refused sheet has to be reported by name.

// withStylesheetTokens runs a test with the two token bounds set, and puts them
// back.
func withStylesheetTokens(t *testing.T, perSheet, perDocument int) {
	t.Helper()
	oldSheet, oldDoc := maxStylesheetTokens, maxDocumentStylesheetTokens
	t.Cleanup(func() { maxStylesheetTokens, maxDocumentStylesheetTokens = oldSheet, oldDoc })
	maxStylesheetTokens, maxDocumentStylesheetTokens = perSheet, perDocument
}

// tokens is how many tokens a sheet is, as the bounds count them.
func tokens(src string) int { return css.CountTokens(src, 1<<30) }

// TestAnOversizedStyleElementIsRefusedAndTheRestRenders is maxStylesheetTokens
// on a <style> element, at its boundary.
//
// Only fetched sheets were bounded, so a <style> of sixteen megabytes was read
// and parsed, and was killed for memory under a four-gigabyte limit. The
// document around the refused sheet is still laid out, with every other sheet
// it carries: refusing one stylesheet is not refusing the page.
func TestAnOversizedStyleElementIsRefusedAndTheRestRenders(t *testing.T) {
	big := "#big { color: rgb(1, 2, 3); margin: 0 0 0 0 }"
	small := "#small { color: rgb(1, 2, 3) }"
	doc := "<style>" + big + "</style><style>" + small + "</style>" +
		"<p id=big>big</p><p id=small>small</p>"
	if tokens(small) >= tokens(big) {
		t.Fatal("the fixture's small sheet is not smaller")
	}

	// Exactly at the bound, the larger sheet is read.
	withStylesheetTokens(t, tokens(big), 1<<20)
	built := Build(Input{HTML: doc})
	if got := colourOf(t, built, "big"); got != wantColour {
		t.Errorf("a <style> of exactly the bound was refused: %v", built.Findings)
	}

	// One token over, with the same document: only the bound moved.
	withStylesheetTokens(t, tokens(big)-1, 1<<20)
	built = Build(Input{HTML: doc})
	if got := colourOf(t, built, "big"); got == wantColour {
		t.Error("a <style> one token over the bound was read")
	}
	if got := colourOf(t, built, "small"); got != wantColour {
		t.Errorf("the <style> inside the bound did not apply when the other was refused: %q", got)
	}
	requireFinding(t, built.Findings, RuleLimit, "this <style> element was not applied")
	fired[RuleLimit] = true
	// The rest of the document is there to be laid out.
	if built.Root == nil || findBox(t, built.Root, "big") == nil {
		t.Error("the document was not built around the refused stylesheet")
	}
}

// TestALargeStyleElementOfFewTokensIsRead is why the bound is on tokens and not
// on bytes: a <style> carrying a font or an image as a data: URL is megabytes of
// text in one string token, costs its bytes once, and is the ordinary way a
// single-file document brings what it needs. A byte bound refused it.
func TestALargeStyleElementOfFewTokensIsRead(t *testing.T) {
	sheet := `#p { color: rgb(1, 2, 3); background-image: url("data:image/png;base64,` +
		strings.Repeat("A", 2<<20) + `") }`
	withStylesheetTokens(t, tokens(sheet), tokens(sheet))
	built := Build(Input{HTML: "<style>" + sheet + "</style><p id=p>x</p>"})
	if got := colourOf(t, built, "p"); got != wantColour {
		t.Errorf("a %d-byte <style> of %d tokens was refused: %v",
			len(sheet), tokens(sheet), built.Findings)
	}
}

// TestTheCallersStylesheetsAreBoundedToo is the same bound on the sheets a
// caller passes, which are no cheaper to parse for having come from the caller.
func TestTheCallersStylesheetsAreBoundedToo(t *testing.T) {
	sheet := "#p { color: rgb(1, 2, 3) }"
	for _, tc := range []struct {
		name string
		in   Input
		says string
	}{
		{"a named sheet", Input{CSS: []Stylesheet{{Name: "theme.css", Source: sheet}}}, `"theme.css"`},
		{"an unnamed sheet", Input{CSS: []Stylesheet{{Source: sheet}}}, "Input.CSS[0]"},
		{"the user sheet", Input{UserCSS: sheet}, "the user stylesheet"},
	} {
		tc.in.HTML = "<p id=p>x</p>"

		withStylesheetTokens(t, tokens(sheet), 1<<20)
		built := Build(tc.in)
		if got := colourOf(t, built, "p"); got != wantColour {
			t.Errorf("%s: a sheet of exactly the bound was refused: %v", tc.name, built.Findings)
		}

		withStylesheetTokens(t, tokens(sheet)-1, 1<<20)
		built = Build(tc.in)
		if got := colourOf(t, built, "p"); got == wantColour {
			t.Errorf("%s: a sheet one token over the bound was read", tc.name)
		}
		requireFinding(t, built.Findings, RuleLimit, tc.says)
	}
}

// TestTheDocumentsStylesheetTokensAreBounded is maxDocumentStylesheetTokens,
// which is on the sum of every sheet a document applies, from wherever each
// came.
//
// The sheets are one of each kind, in the order the pipeline applies them, and
// each is exactly one unit of tokens, so a bound of n units and a little admits
// the first n and refuses the next — which is the boundary, crossed by each kind
// in turn. A bound kept per source would admit all of them: each kind alone is
// one unit.
//
// Then the file is linked a second time, which is five units in all, under a
// bound of four and a bit: the caller's sheet, last, is refused only if the
// second link was charged — and it has to be, because a sheet is parsed and
// held each time it is applied, whether or not it was read again.
func TestTheDocumentsStylesheetTokensAreBounded(t *testing.T) {
	unit := func(id string) string { return "#" + id + " { color: rgb(1, 2, 3) }" }
	u := tokens(unit("user"))
	for _, id := range []string{"inline", "linked", "caller"} {
		if tokens(unit(id)) != u {
			t.Fatalf("the fixture's sheets are not all one unit: %q", id)
		}
	}
	res := &countingResolver{body: unit("linked")}
	in := func() Input {
		return Input{
			UserCSS: unit("user"),
			HTML: "<style>" + unit("inline") + "</style>" +
				`<link rel=stylesheet href=a.css>` +
				"<p id=user>u</p><p id=inline>i</p><p id=linked>l</p><p id=caller>c</p>",
			CSS:       []Stylesheet{{Name: "caller.css", Source: unit("caller")}},
			Resources: res,
		}
	}
	order := []string{"user", "inline", "linked", "caller"}
	for admitted := 0; admitted < len(order); admitted++ {
		withStylesheetTokens(t, u, u*admitted+u-1)
		built := Build(in())
		for i, id := range order {
			got := colourOf(t, built, id)
			if i < admitted && got != wantColour {
				t.Errorf("with room for %d sheets, the %s sheet did not apply: %v",
					admitted, id, built.Findings)
			}
			if i >= admitted && got == wantColour {
				t.Errorf("with room for %d sheets, the %s sheet applied", admitted, id)
			}
		}
		switch order[admitted] {
		case "linked":
			requireFinding(t, built.Findings, RuleResourceBlocked, "a.css")
			requireFinding(t, built.Findings, RuleLimit, "stylesheets are more than")
		case "user":
			requireFinding(t, built.Findings, RuleLimit, "the user stylesheet was not applied")
		case "inline":
			requireFinding(t, built.Findings, RuleLimit, "this <style> element was not applied")
		case "caller":
			requireFinding(t, built.Findings, RuleLimit, `"caller.css" was not applied`)
		}
	}

	withStylesheetTokens(t, u, u*4+u-1)
	twice := in()
	twice.HTML = `<link rel=stylesheet href=a.css>` + twice.HTML
	built := Build(twice)
	if got := colourOf(t, built, "linked"); got != wantColour {
		t.Errorf("the file linked twice did not apply: %v", built.Findings)
	}
	if got := colourOf(t, built, "caller"); got == wantColour {
		t.Error("a sheet linked twice was charged once")
	}
}

// TestAnOversizedStyleAttributeIsNotRead is maxStylesheetTokens on a style
// attribute, which is a stylesheet without a selector and costs what one does.
//
// The attribute is still there, empty, so "[style]" still selects the element;
// what is not read is the declarations in it. Another element's attribute,
// inside the bound, is read as ever.
func TestAnOversizedStyleAttributeIsNotRead(t *testing.T) {
	decl := "color: rgb(1, 2, 3); padding-left: 0px"
	rule := "[style] { border-top: solid }"
	doc := "<style>" + rule + "</style>" +
		`<p id=big style="` + decl + `;">big</p><p id=small style="` + decl + `">small</p>`
	if tokens(rule) > tokens(decl) {
		t.Fatal("the fixture's <style> is larger than the bound it sets, so it is refused too")
	}

	// A bound below the attribute's length in bytes, or the attribute is
	// never counted: no token is shorter than a byte.
	withStylesheetTokens(t, tokens(decl), 1<<20)
	if len(decl) <= maxStylesheetTokens {
		t.Fatal("the fixture's attribute is no longer in bytes than the bound is in tokens")
	}
	built := Build(Input{HTML: doc})
	if got := colourOf(t, built, "big"); got == wantColour {
		t.Error("a style attribute over the bound was read")
	}
	if got := colourOf(t, built, "small"); got != wantColour {
		t.Errorf("a style attribute of exactly the bound was not read: %q", got)
	}
	requireFinding(t, built.Findings, RuleLimit, "style attribute is more than")
	if got := findBox(t, built.Root, "big").Style.Get("border-top-style"); got != "solid" {
		t.Errorf("the element whose style attribute was refused no longer matches [style]: "+
			"border-top-style is %q", got)
	}
}
