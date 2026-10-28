package layout

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// TestADocumentSaysWhatItIs is issue #961's acceptance: the title with its
// whitespace collapsed, the root's language and direction, and the head's
// <meta> names lowercased, in order.
func TestADocumentSaysWhatItIs(t *testing.T) {
	got := Compose(Input{HTML: `<!DOCTYPE html><html lang="nl" dir="rtl"><head>` +
		"<title>  Factuur\n 42 </title>" +
		`<meta charset="utf-8">` +
		`<meta name="Author" content="X">` +
		`<meta http-equiv="content-type" content="text/html">` +
		`<meta name="description">` +
		`<meta name="KEYWORDS" content=" a, b ">` +
		`</head><body><p>x</p><meta name="body" content="not in the head"></body></html>`}, Options{}).Document

	if got.Title != "Factuur 42" || !got.HasTitle {
		t.Errorf("title %q (has %v), want \"Factuur 42\"", got.Title, got.HasTitle)
	}
	if got.Language != "nl" || !got.HasLanguage {
		t.Errorf("language %q (has %v), want \"nl\"", got.Language, got.HasLanguage)
	}
	if got.Direction != "rtl" {
		t.Errorf("direction %q, want \"rtl\"", got.Direction)
	}
	// Only a name with a content, only in the head, the name lowercased and
	// the content as written.
	var meta []string
	for _, m := range got.Meta {
		meta = append(meta, m.Name+"="+m.Content)
		if m.Source.HTMLOffset < 0 {
			t.Errorf("meta %q has no source", m.Name)
		}
	}
	if want := []string{"author=X", "keywords= a, b "}; !slices.Equal(meta, want) {
		t.Errorf("meta %q, want %q", meta, want)
	}
}

// TestNoTitleAndNoLanguageAreTold: absent is not empty. lang="" says the
// language is unknown, which is an answer; no lang is no answer.
func TestNoTitleAndNoLanguageAreTold(t *testing.T) {
	none := Compose(Input{HTML: `<html><body><p>x</p></body></html>`}, Options{}).Document
	if none.HasTitle || none.HasLanguage || none.Title != "" || none.Language != "" {
		t.Errorf("a document with no title and no lang gave %+v", none)
	}
	empty := Compose(Input{HTML: `<html lang=""><head><title> </title></head><body>x</body></html>`},
		Options{}).Document
	if !empty.HasLanguage || empty.Language != "" {
		t.Errorf("lang=\"\" gave %q (has %v), want an empty language that is declared",
			empty.Language, empty.HasLanguage)
	}
	if !empty.HasTitle || empty.Title != "" {
		t.Errorf("a blank title gave %q (has %v), want an empty title that is there",
			empty.Title, empty.HasTitle)
	}
}

// TestTheDirectionIsAValueDirTakes: "ltr" and "rtl" in any case, and nothing
// for "auto" or a value dir does not have.
func TestTheDirectionIsAValueDirTakes(t *testing.T) {
	for dir, want := range map[string]string{
		`dir="RTL"`: "rtl", `dir="ltr"`: "ltr", `dir="auto"`: "", `dir="sideways"`: "", ``: "",
	} {
		got := Compose(Input{HTML: `<html ` + dir + `><body>x</body></html>`}, Options{}).Document
		if got.Direction != want {
			t.Errorf("%s gave direction %q, want %q", dir, got.Direction, want)
		}
	}
}

// TestTheLanguageIsTheOneTheTextWasHyphenatedIn: the reason it comes from
// forme. The root's text is hyphenated in the language Document.Language
// reports — Dutch, by its lang, by xml:lang in XHTML, and by a
// Content-Language pragma — and with no language it is not hyphenated at all
// and none is reported.
func TestTheLanguageIsTheOneTheTextWasHyphenatedIn(t *testing.T) {
	const p = `<p style="hyphens: auto; width: 120px">ontwikkelingssamenwerking</p>`
	for _, tc := range []struct {
		what  string
		in    Input
		lang  string
		hyphn bool
	}{
		{"lang", Input{HTML: `<html lang="nl"><body>` + p + `</body></html>`}, "nl", true},
		{"xml:lang in XHTML", Input{XHTML: true, HTML: `<html xmlns="http://www.w3.org/1999/xhtml" ` +
			`xml:lang="nl"><head><title>t</title></head><body>` + p + `</body></html>`}, "nl", true},
		{"a Content-Language pragma", Input{HTML: `<html><head><meta http-equiv="content-language" ` +
			`content="nl"></head><body>` + p + `</body></html>`}, "nl", true},
		{"none", Input{HTML: `<html><body>` + p + `</body></html>`}, "", false},
	} {
		c := Compose(tc.in, Options{})
		hyphenated := false
		for _, op := range c.Ops {
			if d, ok := op.(DrawText); ok && d.Text == "-" {
				hyphenated = true
			}
		}
		if c.Document.Language != tc.lang || c.Document.HasLanguage != (tc.lang != "") {
			t.Errorf("%s: language %q (has %v), want %q", tc.what,
				c.Document.Language, c.Document.HasLanguage, tc.lang)
		}
		if hyphenated != tc.hyphn {
			t.Errorf("%s: hyphenated %v, want %v: the page and Document.Language disagree",
				tc.what, hyphenated, tc.hyphn)
		}
	}
}

// TestTheMetaListIsBounded: a document can hold a million <meta> elements and
// a real one holds a few dozen, so the list stops at maxMetaNames and a
// RuleLimit finding says where.
func TestTheMetaListIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><head>")
	for i := 0; i < maxMetaNames+10; i++ {
		fmt.Fprintf(&b, `<meta name="n%d" content="c">`, i)
	}
	b.WriteString("</head><body>x</body></html>")
	got := Compose(Input{HTML: b.String()}, Options{})
	if len(got.Document.Meta) != maxMetaNames {
		t.Errorf("%d meta names kept, want the bound of %d", len(got.Document.Meta), maxMetaNames)
	}
	if last := got.Document.Meta[len(got.Document.Meta)-1].Name; last != fmt.Sprintf("n%d", maxMetaNames-1) {
		t.Errorf("the last kept is %q, want the first %d in document order", last, maxMetaNames)
	}
	if !hasRule(got.Findings, RuleLimit) {
		t.Errorf("the cut was not reported: %v", ruleNames(got.Findings))
	}
}

// TestARefusedDocumentStillSaysWhatItIs: Document is read from the parse, not
// from the page, so a backend reporting a refusal can still name the document.
func TestARefusedDocumentStillSaysWhatItIs(t *testing.T) {
	got := Compose(Input{HTML: `<html lang="de"><head><title>Rechnung</title></head><body>x</body></html>`,
		Fonts: emptyFontSet{}}, Options{})
	if !got.Refused {
		t.Fatal("a document with no faces was not refused")
	}
	if got.Document.Title != "Rechnung" || got.Document.Language != "de" {
		t.Errorf("a refused document gave %+v", got.Document)
	}
}
