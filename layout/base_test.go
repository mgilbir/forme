package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
)

// The document's base URL: HTML §4.2.3's <base href>, and what every
// reference in the markup and in the document's own stylesheets is relative
// to. See base.go.

// markupWithEveryKindOfReference names one file through each thing that reads
// a reference relative to the document's base: the markup's attributes, a
// <style> element's url()s and @import, and a style attribute.
const markupWithEveryKindOfReference = `<link rel=stylesheet href="s.css">` +
	`<style>@import "i.css"; @font-face { font-family: F; src: url(f.ttf) }` +
	` div { background-image: url(bg.png) }</style>` +
	`<img src="a.png"><object data="o.svg"></object><video poster="p.png"></video>` +
	`<div>d</div><p style="background-image: url(attr.png); font-family: F">p</p>`

// TestEveryReferenceIsRelativeToTheDocumentBase: under <base href="assets/">
// each of them is asked for in assets/, and none beside the document — which
// is where each was read from while <base> was not read at all.
func TestEveryReferenceIsRelativeToTheDocumentBase(t *testing.T) {
	res := &servingResolver{files: map[string]string{
		// A linked sheet's own references are relative to the sheet, which
		// is in assets/ now.
		"assets/s.css": `span { background-image: url(../up.png) } em { background-image: url(in.png) }`,
	}}
	Build(Input{Resources: res, HTML: `<head><base href="assets/"></head>` +
		markupWithEveryKindOfReference + `<span>s</span><em>e</em>`})
	for _, ref := range []string{"assets/s.css", "assets/i.css", "assets/f.ttf", "assets/bg.png",
		"assets/a.png", "assets/o.svg", "assets/p.png", "assets/attr.png", "up.png", "assets/in.png"} {
		if !res.wasAsked(ref) {
			t.Errorf("the resolver was never asked for %q; it was asked for %q", ref, res.asked)
		}
	}
	for _, wrong := range []string{"s.css", "i.css", "f.ttf", "bg.png", "a.png", "o.svg", "p.png",
		"attr.png", "in.png"} {
		if res.wasAsked(wrong) {
			t.Errorf("the resolver was asked for %q, beside the document", wrong)
		}
	}
}

// TestADocumentWithNoBaseIsItsOwnBase is the control: with no <base>, or with
// one that has no href, each reference is asked for as written — and a sheet
// the caller passed is relative to its own name and never to the document's
// <base>, which is the document's word about its own references.
func TestADocumentWithNoBaseIsItsOwnBase(t *testing.T) {
	for _, head := range []string{"", `<base target="_top">`} {
		res := &servingResolver{files: map[string]string{}}
		Build(Input{Resources: res, HTML: head + markupWithEveryKindOfReference})
		for _, ref := range []string{"s.css", "i.css", "f.ttf", "bg.png", "a.png", "o.svg",
			"p.png", "attr.png"} {
			if !res.wasAsked(ref) {
				t.Errorf("%q: the resolver was never asked for %q; it was asked for %q",
					head, ref, res.asked)
			}
		}
	}
	res := &servingResolver{files: map[string]string{}}
	Build(Input{Resources: res, HTML: `<base href="assets/"><div>d</div>`,
		CSS: []Stylesheet{{Source: `div { background-image: url(caller.png) }`}}})
	if !res.wasAsked("caller.png") {
		t.Errorf("a caller's sheet was made relative to the document's <base>: %q", res.asked)
	}
}

// TestTheFirstBaseWithAnHrefIsTheBase: HTML's "the first base element with an
// href attribute in tree order". One without an href before it is passed
// over, and one after it changes nothing. Its href is read as the URL
// standard reads it, and a backslash in it is a slash.
func TestTheFirstBaseWithAnHrefIsTheBase(t *testing.T) {
	for head, want := range map[string]string{
		`<base target="_top"><base href="one/"><base href="two/">`: "one/a.png",
		`<base href=" &#9;o&#10;ne/ ">`:                            "one/a.png",
		`<base href="one\">`:                                       "one/a.png",
		`<base href="one/page.html?q#f">`:                          "one/a.png",
		`<base href="">`:                                           "a.png",
		`<base href="../">`:                                        "../a.png",
		`<base href="/root/">`:                                     "/root/a.png",
	} {
		res := &servingResolver{files: map[string]string{}}
		Build(Input{Resources: res, HTML: head + `<img src="a.png">`})
		if !res.wasAsked(want) {
			t.Errorf("%s: the resolver was asked for %q, want %q", head, res.asked, want)
		}
	}
}

// TestABaseCannotTakeAReferenceOutOfBounds is resource.go's boundary under a
// base: one naming a scheme, a host or a data: URL makes every relative
// reference name it too, so none reaches the resolver and each is reported —
// and a reference that is a whole URL, or data:, is read as it always was.
func TestABaseCannotTakeAReferenceOutOfBounds(t *testing.T) {
	for _, base := range []string{
		"https://cdn.example/assets/",
		"//cdn.example/assets/",
		`\\cdn.example\assets\`,
		"file:///etc/",
		"data:text/plain,x",
	} {
		res := &servingResolver{files: map[string]string{}}
		built := Build(Input{Resources: res, HTML: `<base href="` + base + `">` +
			`<link rel=stylesheet href="s.css"><style>div { background-image: url(bg.png) }</style>` +
			`<img src="a.png"><img src="/b.png"><div style="background-image: url(c.png)">d</div>` +
			`<img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=">`})
		if len(res.asked) != 0 {
			t.Errorf("%s: the resolver was asked for %q", base, res.asked)
		}
		for _, ref := range []string{"a.png", "/b.png", "s.css", "bg.png", "c.png"} {
			requireFinding(t, built.Findings, RuleResourceBlocked, quoteValue(ref)+" is relative")
		}
	}
}

// TestALinkIsRelativeToTheDocumentBase: a link is not read, so a base with a
// scheme is what it is for, and the target is the URL it resolves to — held
// to the schemes a link may have afterwards, so a base cannot make one out of
// what a link may not be.
func TestALinkIsRelativeToTheDocumentBase(t *testing.T) {
	type link struct{ href, want string }
	for _, tc := range []struct {
		base  string
		links []link
	}{
		{"docs/", []link{
			{"intro.html", "docs/intro.html"},
			{"../up.html?a=1", "up.html?a=1"},
			{"/root.html", "/root.html"},
			// A fragment, and nothing, are the document the base names.
			{"#top", "docs/#top"},
			{"", "docs/"},
			{"https://example.org/x", "https://example.org/x"},
			{"mailto:a@example.org", "mailto:a@example.org"},
		}},
		{"docs/page.html#here", []link{
			{"#top", "docs/page.html#top"},
			{"?q=1", "docs/page.html?q=1"},
		}},
		{"https://example.com/docs/index.html?x=1", []link{
			{"intro.html", "https://example.com/docs/intro.html"},
			{"../up.html", "https://example.com/up.html"},
			{"/root.html", "https://example.com/root.html"},
			{"#top", "https://example.com/docs/index.html?x=1#top"},
			{"", "https://example.com/docs/index.html?x=1"},
			{"?q", "https://example.com/docs/index.html?q"},
			// A host with the scheme left out is the base's scheme's, and so
			// a link like any https one.
			{"//cdn.example/x", "https://cdn.example/x"},
			// The URL standard's two readings for a special scheme.
			{`sub\page.html`, "https://example.com/docs/sub/page.html"},
			{"https:rel.html", "https://example.com/docs/rel.html"},
			{"http://other.example/", "http://other.example/"},
		}},
	} {
		var body strings.Builder
		for _, l := range tc.links {
			body.WriteString(`<p><a href="` + l.href + `">w</a></p>`)
		}
		got, _ := linksIn(Compose(Input{HTML: `<base href="` + tc.base + `">` + body.String()},
			Options{}).Ops)
		if len(got) != len(tc.links) {
			t.Fatalf("%s: %d links, want %d: %+v", tc.base, len(got), len(tc.links), got)
		}
		for i, l := range tc.links {
			if got[i].Href != l.want {
				t.Errorf("%s: %q links to %q, want %q", tc.base, l.href, got[i].Href, l.want)
			}
		}
	}
}

// TestABaseCannotMakeALinkOfWhatALinkMayNotBe: under a base with any scheme
// but http and https — javascript:, file:, data: — a relative link would be a
// URL with that scheme, and is refused and reported; so is one relative to a
// base naming a host with no scheme. A link that is a whole URL is untouched.
func TestABaseCannotMakeALinkOfWhatALinkMayNotBe(t *testing.T) {
	for _, base := range []string{"javascript:alert(1)//", "file:///home/", "data:text/html,x",
		"//cdn.example/", "mailto:a@example.org"} {
		out := Compose(Input{HTML: `<base href="` + base + `"><p><a href="x.html">a</a>` +
			`<a href="#top">b</a><a href="https://example.org/">c</a></p>`}, Options{})
		got, _ := linksIn(out.Ops)
		if len(got) != 1 || got[0].Href != "https://example.org/" {
			t.Errorf("%s: the links are %+v, want only the whole URL", base, got)
		}
		requireFinding(t, out.Findings, RuleLinkRefused, `"x.html"`)
		requireFinding(t, out.Findings, RuleLinkRefused, `"#top"`)
	}
}

// TestAJoinOntoTheBaseIsChargedToTheWorkBudget: the base is the document's to
// choose and every reference is joined onto it, so a long base under many
// short references is work a small input multiplies. What the budget refuses
// is reported, and is neither fetched from somewhere else nor a link.
//
// The base is in the document, and every byte of a document earns work, so
// what exceeds the budget is references enough that their joins cost more than
// the base and the references earned between them: a base of 64KiB earns 128
// joins of itself, and 512 references ask for four times that.
func TestAJoinOntoTheBaseIsChargedToTheWorkBudget(t *testing.T) {
	// The floor every document has, whatever its size, is lowered so that
	// what is left is what this document's bytes earned.
	saved := workFloor
	workFloor = 1 << 16
	defer func() { workFloor = saved }()

	base := strings.Repeat("d", 64<<10) + "/"
	for _, tc := range []struct{ what, one string }{
		{"an image", `<img src=x>`},
		{"a style attribute", `<i style="background:url(x)"></i>`},
		{"a <style> element", `i{background:url(x)}`},
		{"an @import in a <style> element", `@import "x";`},
		{"a <link>", `<link rel=stylesheet href=x>`},
	} {
		t.Run(tc.what, func(t *testing.T) {
			res := &servingResolver{files: map[string]string{}}
			many := strings.Repeat(tc.one, 512)
			if strings.HasPrefix(tc.one, "i{") || strings.HasPrefix(tc.one, "@import") {
				many = "<style>" + many + "</style>"
			}
			built := Build(Input{Resources: res, HTML: `<base href="` + base + `">` + many})
			requireFinding(t, built.Findings, RuleLimit, "past that point")
			if res.wasAsked("x") {
				t.Error("a reference the budget refused was fetched relative to the document instead")
			}
		})
	}
	// A link, joined as a path, as the document the base names, and as a URL.
	for _, tc := range []struct{ base, href string }{
		{base, "x"},
		{base, "#x"},
		{"https://example.test/" + base, "x"},
	} {
		out := Compose(Input{HTML: `<base href="` + tc.base + `">` +
			strings.Repeat(`<a href=`+tc.href+`>w</a>`, 512)}, Options{})
		requireFinding(t, out.Findings, RuleLimit, "the references resolved against the document's base URL")
		for _, l := range func() []Link { got, _ := linksIn(out.Ops); return got }() {
			if l.Href == tc.href {
				t.Errorf("a link the budget refused to resolve is relative to the document: %+v", l)
			}
		}
	}
}

// TestResolveAgainstABaseNamingAHost is the resolver's refusal on its own: a
// base naming a host is refused before anything is joined onto it, the
// root-relative reference included, which against a host is that host's root.
func TestResolveAgainstABaseNamingAHost(t *testing.T) {
	for _, ref := range []string{"a.png", "/a.png", "../a.png", "?q"} {
		if got, why := resolveAgainst(ref, "//cdn.example/x/", baseOf, nil); got != "" || why == "" {
			t.Errorf("%q against a host came to %q (%q)", ref, got, why)
		}
	}
}

// TestARefusedJoinCostsNothing: a join is paid for before it is made, so once
// the budget is spent each reference left costs what the reference does and
// not a copy of the base. Charged after the join, a refused join still made
// its copy, and a document doubled in size — twice the base and twice the
// references — allocated four times as much for every reference past the
// budget: the product of the two, quadratic in the document.
//
// The allocation is counted rather than the time, because the defect is a
// copy. Linear comes out at about four here, and the join made before it was
// refused at about fifteen.
func TestARefusedJoinCostsNothing(t *testing.T) {
	saved := workFloor
	workFloor = 1 << 16
	defer func() { workFloor = saved }()

	build := func(k int) func() {
		doc := `<base href="` + strings.Repeat("d", k*16<<10) + `/">` +
			strings.Repeat(`<img src=x>`, k*512)
		return func() { Build(Input{HTML: doc, Resources: &servingResolver{files: map[string]string{}}}) }
	}
	if r := costtest.Allocated(t, "references joined onto a long base", build(1), build(4)); r > 8 {
		t.Errorf("four times the document allocated %.1f times as much; a join the budget "+
			"refused is being made before it is refused", r)
	}
}
