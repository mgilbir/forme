package layout

import (
	"strings"
	"testing"
)

// A reference as the URL standard reads it.
//
// The engine's boundary is that nothing naming a scheme reaches a resolver. It
// was asked of the string as the document wrote it, which is not what any URL
// parser reads: the URL standard strips control characters and spaces from
// either end and removes every tab and newline before it looks for a scheme. So
// "ht<tab>tp://evil/" had no scheme here and one everywhere else — and a
// resolver handed it would have its parser find it. And a reference with no
// scheme at all can still name a host: "//169.254.169.254/latest" is a
// network-path reference, which a resolver resolving against its document's
// URL — as the ResourceResolver documentation invites it to — turns into
// "https://169.254.169.254/latest" (audit C85).

// TestNoHostEverReachesAResolver is TestNoSchemeEverReachesAResolver for the
// references that name a host without a scheme, and for schemes hidden by the
// characters a URL parser removes.
func TestNoHostEverReachesAResolver(t *testing.T) {
	for _, tc := range []struct{ name, html, css string }{
		{"an image on a host", `<img src="//169.254.169.254/latest/meta-data/">`, ""},
		{"an image on a host, with backslashes", `<img src="\\host\x.png">`, ""},
		{"an image on a host, with one of each", `<img src="/\host/x.png">`, ""},
		{"an image on a host, with the other of each", `<img src="\/host/x.png">`, ""},
		{"a stylesheet on a host", `<link rel=stylesheet href="//evil.example/a.css">`, ""},
		{"a background on a host", `<div id="d"></div>`,
			`#d { background-image: url(//evil.example/x.png); width: 10px; height: 10px }`},
		{"a font on a host", `<p>x</p>`,
			`@font-face { font-family: T; src: url(//evil.example/t.ttf) }`},
		{"an import on a host", `<p>x</p>`, `@import "//evil.example/a.css";`},
		{"a scheme split by a tab", "<img src=\"ht\ttp://evil.example/x.png\">", ""},
		{"a scheme split by a newline", "<img src=\"ht\ntp://evil.example/x.png\">", ""},
		{"a scheme after a control character", "<img src=\"\x01http://evil.example/x.png\">", ""},
		{"a host split by a tab", "<img src=\"/\t/evil.example/x.png\">", ""},
	} {
		if asked := askedFor(tc.html, tc.css); len(asked) != 0 {
			t.Errorf("%s: the resolver was handed %q", tc.name, asked)
		}
		built := Build(Input{HTML: tc.html, Resources: &recordingResolver{},
			CSS: []Stylesheet{{Source: tc.css}}})
		if !hasRule(built.Findings, RuleResourceBlocked) {
			t.Errorf("%s: raised %v, want the blocked-resource finding",
				tc.name, ruleNames(built.Findings))
		}
	}
}

// TestAHostInANamedSheetIsNotMadeAPath: resolving against a sheet must not join
// a host onto the sheet's directory, which would hide it inside a path that no
// check afterwards could recognise.
func TestAHostInANamedSheetIsNotMadeAPath(t *testing.T) {
	res := &recordingResolver{}
	Build(Input{HTML: `<div id=d>x</div>`, Resources: res, CSS: []Stylesheet{{Name: "css/a.css",
		Source: `@import "//evil.example/a.css"; #d { background-image: url(//evil.example/x.png) }`}}})
	for _, ref := range res.asked {
		if strings.Contains(ref, "evil") {
			t.Errorf("the resolver was handed %q", ref)
		}
	}
}

// TestDirResolverRefusesHosts: a caller calling DirResolver directly gets the
// same boundary, with the reason named.
func TestDirResolverRefusesHosts(t *testing.T) {
	_, res := planted(t)
	for _, ref := range []string{"//inside.txt", `\\inside.txt`, `/\inside.txt`, "ht\ttp://x/inside.txt"} {
		_, err := res.Resolve(ref)
		if err == nil {
			t.Errorf("%q was accepted; a reference naming a host must be refused", ref)
			continue
		}
		if !strings.Contains(err.Error(), "host") && !strings.Contains(err.Error(), "scheme") {
			t.Errorf("%q was refused for the wrong reason: %v", ref, err)
		}
	}
}

// TestAReferenceIsReadAsTheURLStandardReadsIt: what a resolver is handed has
// lost exactly what a URL parser removes, and nothing else.
func TestAReferenceIsReadAsTheURLStandardReadsIt(t *testing.T) {
	for in, want := range map[string]string{
		"logo.png":            "logo.png",
		"  logo.png \n":       "logo.png",
		"lo\tgo.png":          "logo.png",
		"lo\ngo\r.png":        "logo.png",
		"\x00\x1f logo.png":   "logo.png",
		"a b.png":             "a b.png",
		"caf\xc3\xa9.png":     "caf\xc3\xa9.png",
		"raw\xff\tbyte":       "raw\xffbyte",
		"\x7flogo.png":        "\x7flogo.png",
		"data:,a\n\tb":        "data:,ab",
		"data:,100% \x20wide": "data:,100%  wide",
	} {
		if got := referenceText(in); got != want {
			t.Errorf("%q read as %q, want %q", in, got, want)
		}
	}
	if asked := askedFor("<img src=\" lo\tgo.png\n\">", ""); len(asked) != 1 || asked[0] != "logo.png" {
		t.Errorf("the resolver was handed %q, want [\"logo.png\"]", asked)
	}
}

// TestPercentDecodeLeavesWhatIsNotAnEscape is the URL standard's
// percent-decode: a "%" and two hexadecimal digits is a byte, and anything
// else is itself.
func TestPercentDecodeLeavesWhatIsNotAnEscape(t *testing.T) {
	for in, want := range map[string]string{
		"100%":      "100%",
		"%":         "%",
		"%4":        "%4",
		"%41":       "A",
		"%4a%4A":    "JJ",
		"%zz":       "%zz",
		"%%41":      "%A",
		"a%2":       "a%2",
		"%25%32%30": "%20",
		"%E2%82%AC": "\u20ac",
		"%ff":       "\xff",
	} {
		if got := percentDecode(in); got != want {
			t.Errorf("%q decoded to %q, want %q", in, got, want)
		}
	}
	// And a file name DirResolver is asked for.
	if got, err := resourcePath("100%.png"); err != nil || got != "100%.png" {
		t.Errorf("\"100%%.png\" became %q, %v; want the file of that name", got, err)
	}
}

// TestADataURLIsReadAsFetchReadsIt is the Fetch standard's data: URL
// processor, one clause at a time (audit C82).
func TestADataURLIsReadAsFetchReadsIt(t *testing.T) {
	for _, tc := range []struct {
		url, body, mime string
		fails           bool
	}{
		// The body is percent-decoded, and a "%" that begins no escape stays.
		{url: "data:,100%", body: "100%", mime: "text/plain"},
		{url: "data:,a%2", body: "a%2", mime: "text/plain"},
		{url: "data:,%41%zz", body: "A%zz", mime: "text/plain"},
		// The fragment is not part of it.
		{url: "data:,a#b", body: "a", mime: "text/plain"},
		{url: "data:,a%23b", body: "a#b", mime: "text/plain"},
		// A query is.
		{url: "data:,a?b", body: "a?b", mime: "text/plain"},
		// The type, and its default.
		{url: "data:IMAGE/SVG+XML;charset=utf-8,x", body: "x", mime: "image/svg+xml"},
		{url: "data: image/png ,x", body: "x", mime: "image/png"},
		{url: "data:;charset=utf-8,x", body: "x", mime: "text/plain"},
		{url: "data:nonsense,x", body: "x", mime: "text/plain"},
		// base64 when the type ends in it, and only then.
		{url: "data:text/plain;base64,QUJD", body: "ABC", mime: "text/plain"},
		{url: "data:text/plain; BASE64,QUJD", body: "ABC", mime: "text/plain"},
		{url: "data:;base64,QUJD", body: "ABC", mime: "text/plain"},
		{url: "data:text/plain;base64;charset=x,QUJD", body: "QUJD", mime: "text/plain"},
		{url: "data:text/plain;xbase64,QUJD", body: "QUJD", mime: "text/plain"},
		// The forgiving decode: white space anywhere, padding optional, and
		// the escapes decoded first.
		{url: "data:;base64,QU JD", body: "ABC", mime: "text/plain"},
		{url: "data:;base64,QQ", body: "A", mime: "text/plain"},
		{url: "data:;base64,QQ==", body: "A", mime: "text/plain"},
		{url: "data:;base64,QUI=", body: "AB", mime: "text/plain"},
		{url: "data:;base64,QU%4AD", body: "ABC", mime: "text/plain"},
		{url: "data:;base64,QR==", body: "A", mime: "text/plain"},
		// And what it refuses.
		{url: "data:;base64,QQ=", fails: true},
		{url: "data:;base64,Q", fails: true},
		{url: "data:;base64,QQ===", fails: true},
		{url: "data:;base64,Q=Q=", fails: true},
		{url: "data:;base64,QU-D", fails: true},
		{url: "data:;base64,QU_D", fails: true},
		{url: "data:text/plain", fails: true},
	} {
		body, mime, fail := decodeDataURI(tc.url, "image", RuleImageUndecodable)
		switch {
		case tc.fails && fail == nil:
			t.Errorf("%q decoded to %q; want it refused", tc.url, body)
		case !tc.fails && fail != nil:
			t.Errorf("%q was refused: %s", tc.url, fail.message)
		case !tc.fails && (string(body) != tc.body || mime != tc.mime):
			t.Errorf("%q decoded to %q as %q, want %q as %q", tc.url, body, mime, tc.body, tc.mime)
		}
	}
}

// TestAHandWrittenSVGDataURLIsDrawn is the document the decode was refusing:
// an SVG written straight into the attribute, as people write them, with a
// percentage in it — in an <img> and in a background.
func TestAHandWrittenSVGDataURLIsDrawn(t *testing.T) {
	const svg = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='50' height='50'>` +
		`<rect width='100%' height='100%' fill='rgb(0,128,0)'/></svg>`
	for _, doc := range []string{
		`<img id=i src="` + svg + `">`,
		`<div id=i style="width: 50px; height: 50px; background-image: url(&quot;` + svg + `&quot;)"></div>`,
		// An attribute wrapped across lines is one URL, as a URL parser reads it.
		"<img id=i src=\"data:image/svg+xml;base64,\n" +
			"PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIHdpZHRoPSc1MCcgaGVpZ2h0\n" +
			"PSc1MCc+PHJlY3Qgd2lkdGg9JzEwMCUnIGhlaWdodD0nMTAwJScgZmlsbD0ncmdiKDAsMTI4LDAp\n" +
			"Jy8+PC9zdmc+\">",
	} {
		built := Build(Input{HTML: doc})
		if hasRule(built.Findings, RuleImageUndecodable) || hasRule(built.Findings, RuleResourceBlocked) {
			t.Errorf("%.60q…: %v", doc, built.Findings)
			continue
		}
		ops := paintWith(t, nil, doc)
		green := false
		for _, op := range ops {
			if f, ok := op.(FillRect); ok && f.Color.G == 128 && f.Color.R == 0 {
				green = true
			}
		}
		if !green {
			t.Errorf("%.60q…: nothing green was painted; ops %v", doc, ops)
		}
	}
}
