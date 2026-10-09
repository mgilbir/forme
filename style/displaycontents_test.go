package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// computedDisplays styles a document under one author sheet and returns each
// element's computed display, keyed by its id.
func computedDisplays(t *testing.T, src, sheet string) map[string]string {
	t.Helper()
	author, errs := css.ParseStylesheet(sheet)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, src)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: author}})
	out := map[string]string{}
	doc.Walk(func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			if id, ok := n.Attr("id"); ok {
				out[id] = got.Styles[n].Get("display")
			}
		}
		return true
	})
	return out
}

// TestDisplayContentsComputesToNoneOnUnusualElements is css-display-3's
// Appendix B, through the cascade: on a replaced element or a form control
// "display: contents computes to display: none", and on everything else the
// value stands — including the four elements the Appendix names to say so.
//
// It used to stand everywhere. Layout then refused it on the replaced
// elements and the controls, laying each out as an inline box — an <img>
// drawn where the author had said it had no box — and honoured it on a
// MathML element, whose children were hoisted out of the formula.
func TestDisplayContentsComputesToNoneOnUnusualElements(t *testing.T) {
	src := `<p id="p">` +
		`<br id="br"><wbr id="wbr"><canvas id="canvas">f</canvas>` +
		`<object id="object">f</object><iframe id="iframe"></iframe>` +
		`<img id="img" src="x.png"><video id="video">f</video>` +
		`<input id="input"><textarea id="textarea"></textarea>` +
		`<select id="select"><option id="option">a</option></select>` +
		`<svg id="svg"></svg>` +
		`<math id="math"><mi id="mi">x</mi></math>` +
		`<button id="button">b</button><span id="span">s</span></p>` +
		`<fieldset id="fieldset"><legend id="legend">l</legend></fieldset>` +
		`<div id="div">d</div>`
	got := computedDisplays(t, src, `* { display: contents }`)
	for id, want := range map[string]string{
		"br": "none", "wbr": "none", "canvas": "none", "object": "none",
		"iframe": "none", "img": "none", "video": "none", "input": "none",
		"textarea": "none", "select": "none", "svg": "none", "math": "none",
		"mi": "none",
		// "These elements don't have any special behavior; display: contents
		// simply removes their principal box", and a legend "reacts to
		// display: contents normally". An <option> is no control of its own.
		"button": "contents", "fieldset": "contents", "legend": "contents",
		"option": "contents", "span": "contents", "div": "contents", "p": "contents",
	} {
		g, ok := got[id]
		if !ok {
			t.Errorf("no element #%s in the parsed document", id)
			continue
		}
		if g != want {
			t.Errorf("#%s computed display %q, want %q", id, g, want)
		}
	}

	// A value other than "contents" is the author's on these elements, as on
	// any other: the rule is about one value and not about the elements.
	got = computedDisplays(t, `<img id="img" src="x.png"><svg id="svg"></svg>`,
		`#img { display: block } #svg { display: inline-block }`)
	if got["img"] != "block" || got["svg"] != "inline-block" {
		t.Errorf("a display that is not contents was changed: %v", got)
	}
}

// TestDisplayContentsComputesToBlockOnTheRoot is §2.8: "a display of contents
// computes to block on the root element". It is a computed value, so a child
// that inherits display takes "block" and not "contents".
func TestDisplayContentsComputesToBlockOnTheRoot(t *testing.T) {
	got := computedDisplays(t, `<html id="root"><body id="body">x</body></html>`,
		`html { display: contents } body { display: inherit }`)
	if got["root"] != "block" {
		t.Errorf("the root's display computed to %q, want block", got["root"])
	}
	if got["body"] != "block" {
		t.Errorf("a child inheriting the root's display got %q; inheritance takes "+
			"the computed value, which is block", got["body"])
	}
	// Only the root: the same value one level down is the body's own.
	got = computedDisplays(t, `<html id="root"><body id="body">x</body></html>`,
		`body { display: contents }`)
	if got["body"] != "contents" {
		t.Errorf("the body's display computed to %q; it is not the root", got["body"])
	}
}

// TestEveryElementAppendixBListsComputesToNone covers the list itself,
// including the elements the parser does not put in a tree where the cascade
// test above could reach them — <embed>, <audio>, <meter> and <progress> are
// dropped with a finding, and <frame> and <frameset> are not body content.
func TestEveryElementAppendixBListsComputesToNone(t *testing.T) {
	for _, name := range []string{"br", "wbr", "meter", "progress", "canvas", "embed",
		"object", "audio", "iframe", "img", "video", "frame", "frameset", "input",
		"textarea", "select", "IMG"} {
		n := &html.Node{Type: html.ElementNode, Name: name, Namespace: html.NamespaceHTML}
		if got := unusualDisplayContents(n, false); got != "none" {
			t.Errorf("<%s>: %q, want none", name, got)
		}
		// The root rule is asked first.
		if got := unusualDisplayContents(n, true); got != "block" {
			t.Errorf("<%s> as the root: %q, want block", name, got)
		}
	}
	for _, name := range []string{"legend", "button", "details", "fieldset", "div",
		"summary", "slot"} {
		n := &html.Node{Type: html.ElementNode, Name: name, Namespace: html.NamespaceHTML}
		if got := unusualDisplayContents(n, false); got != "" {
			t.Errorf("<%s>: %q, want the value to stand", name, got)
		}
	}
	// A MathML element that shares a name with an HTML one is MathML's.
	n := &html.Node{Type: html.ElementNode, Name: "img", Namespace: html.NamespaceMathML}
	if got := unusualDisplayContents(n, false); got != "none" {
		t.Errorf("a MathML element: %q, want none", got)
	}
	n = &html.Node{Type: html.ElementNode, Name: "img", Namespace: html.NamespaceOther}
	if got := unusualDisplayContents(n, false); got != "" {
		t.Errorf("an element in an unknown namespace: %q, want the value to stand", got)
	}
}
