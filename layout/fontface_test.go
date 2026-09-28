package layout

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
)

// The checks on @font-face, which is the one feature in this package that hands
// attacker-controlled bytes to a parser of attacker-controlled structure.
//
// Every cap in fontface.go has a test here that fires it, because a cap nobody
// has watched trip is one nobody knows works — and the caps are the whole of the
// security argument for the feature. They are variables so that these tests can
// lower them and watch the boundary rather than allocating their way to it.

// realFont is a font program these tests can hand the engine.
//
// It is forme's bundled Noto Sans, which is already a dependency of this
// repository through forme's font packages, so nothing new is vendored and no corpus is
// needed — these tests run in a bare checkout. It is loaded once because it is
// two megabytes and parsing it per test would be paid for a dozen times.
func realFont() []byte {
	realFontOnce.Do(func() { realFontData = notosans.Regular() })
	return realFontData
}

var (
	realFontOnce sync.Once
	realFontData []byte
)

// fileResolver serves bytes by name and records what it was asked for, so a
// test can prove a refusal happened *before* the resolver rather than inside it.
type fileResolver struct {
	files map[string][]byte
	asked []string
}

func (f *fileResolver) Resolve(ref string) ([]byte, error) {
	f.asked = append(f.asked, ref)
	data, ok := f.files[ref]
	if !ok {
		return nil, fmt.Errorf("no such file %q", ref)
	}
	return data, nil
}

// docWithFontFace wraps a stylesheet in the smallest document that uses it.
func docWithFontFace(sheet string) string {
	return "<style>" + sheet + "</style><p id=p style=\"font-family: Trial\">x</p>"
}

// TestFontFaceLoadsFromTheDocument is the feature: a document names a font, the
// font arrives through the resolver, and the family it declared resolves to it.
//
// The face is checked by measuring rather than by being non-nil, because a
// fallback is also non-nil. Noto Sans and Helvetica do not measure the same
// string to the same width, which is what makes the assertion about *which*
// face arrived.
func TestFontFaceLoadsFromTheDocument(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
		Resources: res,
	})

	face, ok := built.Fonts.Face("Trial", false, false)
	if !ok || face == nil {
		t.Fatalf("the document's own font-family did not resolve; findings: %v", built.Findings)
	}
	if _, standard := StandardFonts().Face("Trial", false, false); standard {
		t.Fatal(`the standard set answers for "Trial", so this proves nothing`)
	}
	want, err := loadRealFace()
	if err != nil {
		t.Fatalf("loading the reference face: %v", err)
	}
	if got, wantW := face.Measure("Hamburgefonstiv", 20), want.Measure("Hamburgefonstiv", 20); got != wantW {
		t.Errorf("the loaded face measures %v where the font measures %v", got, wantW)
	}
	// And a family the document did not declare still goes to the caller's set.
	if _, ok := built.Fonts.Face("Helvetica", false, false); !ok {
		t.Error("wrapping the caller's set lost the families it had")
	}
	for _, f := range built.Findings {
		if f.Unsupported() {
			t.Errorf("loading a plain @font-face reported %s: %s", f.Rule, f.Message)
		}
	}
}

// loadRealFace parses the reference font a second time, so that the measurement
// above compares the face the engine loaded against the file rather than
// against itself.
func loadRealFace() (*shape.Face, error) { return shape.Load(realFont()) }

// TestFontFaceIsNoLongerAnUnsupportedAtRule is the report half, and it is what
// the WPT harness's stripped link was standing in for.
//
// The cascade reports every at-rule it does not apply. @font-face is applied
// now, so it must not be reported — while every other at-rule still is, because
// a change that silenced all of them would look identical in the one document
// that only has this one.
func TestFontFaceIsNoLongerAnUnsupportedAtRule(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
		Resources: res,
	})
	for _, f := range built.Findings {
		if f.Rule == RuleUnsupportedAtRule {
			t.Errorf("@font-face is still reported as an at-rule that is not applied: %s", f.Message)
		}
	}

	// The control is an at-rule that is still not applied, and it has moved four
	// times now: @media until media queries were answered, @page until page
	// margins were, @supports until its conditions were, @layer until the
	// cascade took it. What makes this test worth having is that *something* is
	// still reported, so that "nothing is reported" cannot pass for "this one is
	// implemented" — and the moving is the feature working.
	other := Build(Input{HTML: `<style>@keyframes spin { from { opacity: 0 } }</style><p>x</p>`})
	found := false
	for _, f := range other.Findings {
		if f.Rule == RuleUnsupportedAtRule {
			found = true
		}
	}
	if !found {
		t.Error("@keyframes stopped being reported too, so the change was not specific to @font-face")
	}
}

// TestFontFaceSrcIsTriedInOrder pins the fallback chain: an entry that cannot be
// loaded is the next entry's turn, not a failure of the rule.
func TestFontFaceSrcIsTriedInOrder(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"second.ttf": realFont()}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: url(missing.ttf) format("truetype"), url(second.ttf); }`),
		Resources: res,
	})
	if _, ok := built.Fonts.Face("Trial", false, false); !ok {
		t.Fatalf("the second src was not tried; findings: %v", built.Findings)
	}
	if len(res.asked) != 2 || res.asked[0] != "missing.ttf" || res.asked[1] != "second.ttf" {
		t.Errorf("the resolver was asked for %v, want missing.ttf then second.ttf", res.asked)
	}
	// A rule that ended up with a font reports nothing: three alternatives with
	// two unused is what an author writes on purpose.
	for _, f := range built.Findings {
		if f.Rule == RuleResourceBlocked || f.Rule == RuleFontUndecodable {
			t.Errorf("a src that fell through to a working entry reported %s: %s", f.Rule, f.Message)
		}
	}
}

// TestFontFaceSrcEntryGrammar pins which shapes of src entry are read and which
// are skipped, because an entry this engine cannot parse must be skipped rather
// than take the rest of the list down with it — and because the parser's
// clauses are otherwise not all reachable from the tests above.
func TestFontFaceSrcEntryGrammar(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string // the reference the loader ended up using, or "" for none
	}{
		{"an unquoted url token", `url(a.ttf)`, "a.ttf"},
		{"a quoted url function", `url("a.ttf")`, "a.ttf"},
		{"a format hint this engine reads", `url(a.ttf) format("truetype")`, "a.ttf"},
		{"an unquoted format hint", `url(a.ttf) format(opentype)`, "a.ttf"},
		// tech() narrows when an entry may be used and never widens it, so
		// ignoring it can only mean trying a font that then parses or does not.
		{"a tech list", `url(a.ttf) tech(color-COLRv1)`, "a.ttf"},
		// An entry with something in it that is not the grammar is skipped, and
		// the next one is taken — which is the behaviour that makes a
		// forward-compatible src list work at all.
		{"a nonsense entry then a real one", `nonsense(x), url(a.ttf)`, "a.ttf"},
		// A url followed by something that is not the grammar invalidates that
		// entry and only that entry. This is the case that distinguishes
		// skipping an entry from ignoring a stray token inside one, and the
		// difference shows: the second url is what must be used.
		{"a url followed by nonsense", `url(a.ttf) nonsense, url(b.ttf)`, "b.ttf"},
		{"two urls in one entry", `url(a.ttf) url(b.ttf), url(b.ttf)`, "b.ttf"},
		{"a format before its url", `format("truetype") url(a.ttf), url(b.ttf)`, "b.ttf"},
		{"nothing usable at all", `nonsense(x)`, ""},
	} {
		res := &fileResolver{files: map[string][]byte{
			"a.ttf": realFont(), "b.ttf": realFont(),
		}}
		built := Build(Input{
			HTML:      docWithFontFace(`@font-face { font-family: Trial; src: ` + tc.src + `; }`),
			Resources: res,
		})
		set, _ := built.Fonts.(*documentFonts)
		got := ""
		if set != nil && len(set.faces) == 1 {
			got = set.faces[0].ref
		}
		if got != tc.want {
			t.Errorf("%s (%s) loaded %q, want %q; findings: %v",
				tc.name, tc.src, got, tc.want, built.Findings)
		}
	}
}

// TestFontFaceLocalIsNotAFailure pins the other half of the chain. local() names
// a face the reader may have; not having it is the ordinary case, and reporting
// it would put a finding on every well-written stylesheet on the web.
func TestFontFaceLocalIsNotAFailure(t *testing.T) {
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: local("Nonesuch Sans"), local(Helvetica); }`),
	})
	face, ok := built.Fonts.Face("Trial", false, false)
	if !ok {
		t.Fatalf("local(Helvetica) did not resolve against the standard set; findings: %v",
			built.Findings)
	}
	std, _ := StandardFonts().Face("Helvetica", false, false)
	if face.Measure("Hamburgefonstiv", 20) != std.Measure("Hamburgefonstiv", 20) {
		t.Error("local(Helvetica) resolved to something that is not Helvetica")
	}
	for _, f := range built.Findings {
		if f.Rule == RuleResourceBlocked || f.Rule == RuleFontUndecodable {
			t.Errorf("a local() the set does not have reported %s: %s", f.Rule, f.Message)
		}
	}
	// No resolver was configured and none was needed: local() reads nothing.
	if _, ok := built.Fonts.(*documentFonts); !ok {
		t.Errorf("the document's set is %T, want the document's own faces", built.Fonts)
	}
}

// TestFontFaceReportsWhenNothingLoads is the finding that keeps a missing font
// from being silent: the family is simply not there afterwards, and a page set
// in something else looks like a page.
func TestFontFaceReportsWhenNothingLoads(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: local(Nonesuch), url(missing.ttf); }`),
		Resources: res,
	})
	// The set the document is laid out in is always the wrapper — it is what
	// makes the faces this document's rather than the caller's library's — so
	// what says the rule contributed nothing is that the family it declared
	// does not resolve to any of the document's own faces.
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's set is %T", built.Fonts)
	}
	if len(set.faces) != 0 {
		t.Errorf("a rule that loaded nothing still put %d faces in the document's set",
			len(set.faces))
	}
	if _, has := set.byFamily["trial"]; has {
		t.Error("a rule that loaded nothing still declared its family")
	}
	requireFinding(t, built.Findings, RuleResourceBlocked, "loaded no font")
	fired[RuleResourceBlocked] = true
}

// TestFontFaceNeedsAResolver is the deny-by-default guarantee for shape. It is
// the same promise resource.go makes for images and stylesheets, checked
// separately because a second loading path is always the one missing a check.
func TestFontFaceNeedsAResolver(t *testing.T) {
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
	})
	if _, ok := built.Fonts.Face("Trial", false, false); ok {
		t.Error("a font was loaded with no resolver configured")
	}
	requireFinding(t, built.Findings, RuleResourceBlocked, ErrNoResolver.Error())
	fired[RuleResourceBlocked] = true
}

// TestFontFaceRefusesSchemes is the no-network guarantee. The resolver here
// would serve the font if it were ever asked, so a scheme that got through would
// show up in what it was asked for as well as in the face.
func TestFontFaceRefusesSchemes(t *testing.T) {
	for _, ref := range []string{
		"http://example.invalid/x.ttf",
		"https://example.invalid/x.ttf",
		"file:///etc/x.ttf",
		"ftp://example.invalid/x.ttf",
		"c:/windows/fonts/x.ttf",
	} {
		res := &fileResolver{files: map[string][]byte{ref: realFont()}}
		built := Build(Input{
			HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url("` + ref + `"); }`),
			Resources: res,
		})
		if _, ok := built.Fonts.Face("Trial", false, false); ok {
			t.Errorf("%s was fetched", ref)
		}
		if len(res.asked) != 0 {
			t.Errorf("%s reached the resolver as %v; the refusal must happen first", ref, res.asked)
		}
		requireFinding(t, built.Findings, RuleResourceBlocked, "resolves no URLs")
	}
	fired[RuleResourceBlocked] = true
}

// TestFontFaceRefusesEscapingTheResolver pins that a font goes through
// DirResolver's containment like everything else. The engine's policy lives in
// one place and this is the check that fonts did not get a second one.
func TestFontFaceRefusesEscapingTheResolver(t *testing.T) {
	dir := t.TempDir()
	res, err := NewDirResolver(dir)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	defer res.Close()
	for _, ref := range []string{"../outside.ttf", "/etc/fonts/x.ttf"} {
		built := Build(Input{
			HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url("` + ref + `"); }`),
			Resources: res,
		})
		if _, ok := built.Fonts.Face("Trial", false, false); ok {
			t.Errorf("%s was loaded", ref)
		}
		requireFinding(t, built.Findings, RuleResourceBlocked, "was not loaded")
	}
	fired[RuleResourceBlocked] = true
}

// TestFontFaceDataURI pins that the one exception to "no scheme" behaves like
// the exception it is: the bytes were in the document already, and they go
// through the same capped decoder as an image's.
func TestFontFaceDataURI(t *testing.T) {
	uri := "data:font/ttf;base64," + base64.StdEncoding.EncodeToString(realFont())
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url("` + uri + `"); }`),
	})
	if _, ok := built.Fonts.Face("Trial", false, false); !ok {
		t.Fatalf("a data: font did not load; findings: %v", built.Findings)
	}

	// And the data URI cap is the same one, at the same place.
	saved := maxDataURIBytes
	maxDataURIBytes = 16
	defer func() { maxDataURIBytes = saved }()
	built = Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url("` + uri + `"); }`),
	})
	if _, ok := built.Fonts.Face("Trial", false, false); ok {
		t.Error("a data: font past the cap was still decoded")
	}
	requireFinding(t, built.Findings, RuleResourceBlocked, "loaded no font")
}

// TestFontFileCapFires is the per-file cap. The font is a real one and the cap
// is lowered under it, so what is being watched is the comparison rather than an
// allocation — and the refusal happens before shape.Load, which is the point of
// having it at all.
func TestFontFileCapFires(t *testing.T) {
	saved := maxFontBytes
	maxFontBytes = len(realFont()) - 1
	defer func() { maxFontBytes = saved }()

	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
		Resources: res,
	})
	if _, ok := built.Fonts.Face("Trial", false, false); ok {
		t.Error("a font larger than the cap was parsed")
	}
	requireFinding(t, built.Findings, RuleResourceBlocked, "this engine will parse")

	// One byte the other way and it loads, so the cap is a boundary rather than
	// a refusal of everything.
	maxFontBytes = len(realFont())
	built = Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
		Resources: res,
	})
	if _, ok := built.Fonts.Face("Trial", false, false); !ok {
		t.Errorf("a font of exactly the cap was refused; findings: %v", built.Findings)
	}
	fired[RuleResourceBlocked] = true
}

// TestDocumentFontByteBudgetFires is the document-wide budget: a per-file cap
// does not bound a document, and this is what does.
func TestDocumentFontByteBudgetFires(t *testing.T) {
	saved := maxDocumentFontBytes
	maxDocumentFontBytes = len(realFont()) + 1
	defer func() { maxDocumentFontBytes = saved }()

	res := &fileResolver{files: map[string][]byte{
		"one.ttf": realFont(), "two.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: One; src: url(one.ttf); }
			@font-face { font-family: Two; src: url(two.ttf); }
		</style><p>x</p>`,
		Resources: res,
	})
	if _, ok := built.Fonts.Face("One", false, false); !ok {
		t.Errorf("the first font was refused; findings: %v", built.Findings)
	}
	if _, ok := built.Fonts.Face("Two", false, false); ok {
		t.Error("the second font was parsed past the document's byte budget")
	}
	requireFinding(t, built.Findings, RuleLimit, "for one document")
	requireFinding(t, built.Findings, RuleResourceBlocked, "budget was already spent")
	fired[RuleLimit] = true
	fired[RuleResourceBlocked] = true
}

// TestDocumentFaceCountCapFires is the count. It bounds how many font programs
// one document can make this engine parse, which the byte budget alone does not:
// a thousand small fonts are a thousand parses and few bytes.
func TestDocumentFaceCountCapFires(t *testing.T) {
	saved := maxDocumentFaces
	maxDocumentFaces = 2
	defer func() { maxDocumentFaces = saved }()

	files := map[string][]byte{}
	var sheet strings.Builder
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("f%d.ttf", i)
		files[name] = realFont()
		fmt.Fprintf(&sheet, "@font-face { font-family: F%d; src: url(%s); }\n", i, name)
	}
	res := &fileResolver{files: files}
	built := Build(Input{HTML: "<style>" + sheet.String() + "</style><p>x</p>", Resources: res})

	var have int
	for i := 0; i < 4; i++ {
		if _, ok := built.Fonts.Face(fmt.Sprintf("F%d", i), false, false); ok {
			have++
		}
	}
	if have != maxDocumentFaces {
		t.Errorf("%d of 4 faces loaded, want the cap of %d", have, maxDocumentFaces)
	}
	requireFinding(t, built.Findings, RuleLimit, "font faces this engine will parse")
	fired[RuleLimit] = true
}

// TestFontFaceRuleCapFires is the cap that answers a page declaring a thousand
// @font-face rules.
//
// The other two caps do not: a rule whose file is missing costs a resolver call
// and no bytes and no face, so a thousand of them would be a thousand system
// calls with every other guard still showing green. The files here are all
// missing for exactly that reason.
func TestFontFaceRuleCapFires(t *testing.T) {
	saved := maxFontFaceRules
	maxFontFaceRules = 3
	defer func() { maxFontFaceRules = saved }()

	var sheet strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&sheet, "@font-face { font-family: F%d; src: url(f%d.ttf); }\n", i, i)
	}
	res := &fileResolver{files: map[string][]byte{}}
	built := Build(Input{HTML: "<style>" + sheet.String() + "</style><p>x</p>", Resources: res})

	if len(res.asked) != maxFontFaceRules {
		t.Errorf("the resolver was asked %d times for 10 rules with a cap of %d",
			len(res.asked), maxFontFaceRules)
	}
	requireFinding(t, built.Findings, RuleLimit, "this engine will look at")
	requireFinding(t, built.Findings, RuleResourceBlocked, "was not read")
	fired[RuleLimit] = true
	fired[RuleResourceBlocked] = true
}

// TestFontSrcListCapFires bounds one rule's fallback chain, so that a single
// @font-face cannot be a loop.
func TestFontSrcListCapFires(t *testing.T) {
	saved := maxFontSources
	maxFontSources = 2
	defer func() { maxFontSources = saved }()

	var srcs []string
	for i := 0; i < 8; i++ {
		srcs = append(srcs, fmt.Sprintf("url(f%d.ttf)", i))
	}
	res := &fileResolver{files: map[string][]byte{}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: ` +
			strings.Join(srcs, ", ") + `; }`),
		Resources: res,
	})
	if len(res.asked) != maxFontSources {
		t.Errorf("the resolver was asked %d times for 8 sources with a cap of %d",
			len(res.asked), maxFontSources)
	}
	requireFinding(t, built.Findings, RuleLimit, "src entries")
	fired[RuleLimit] = true
}

// TestFontUndecodable separates "the file did not arrive" from "the file
// arrived and was not a font", which is the distinction the rule exists for.
func TestFontUndecodable(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"junk.ttf":  []byte("this is not a font program at all"),
		"empty.ttf": {},
	}}
	built := Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(junk.ttf); }`),
		Resources: res,
	})
	requireFinding(t, built.Findings, RuleResourceBlocked, "not one this engine can read")
	fired[RuleFontUndecodable] = true

	built = Build(Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(empty.ttf); }`),
		Resources: res,
	})
	requireFinding(t, built.Findings, RuleResourceBlocked, "is empty")

	// A format hint for a container this engine does not unwrap is refused
	// before the read, which is why the resolver is never asked for it. SVG
	// fonts are the case: they are not an sfnt in a wrapper, they are XML, and
	// there is nothing here that could read one.
	res = &fileResolver{files: map[string][]byte{"x.svg": realFont()}}
	built = Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: url(x.svg) format("svg"); }`),
		Resources: res,
	})
	if len(res.asked) != 0 {
		t.Errorf("an svg font was fetched before its format hint was read: %v", res.asked)
	}
	requireFinding(t, built.Findings, RuleResourceBlocked, "which this engine does not read")
}

// TestAWoff2HintIsNotARefusal.
//
// woff2 was on the refused list until the decoder for it existed, and stayed
// there after it did. That is the failure mode a format hint has: nothing tries
// the bytes, so nothing finds out they would have parsed, and the entry goes on
// being skipped for a reason that stopped being true.
//
// It is the format the web serves, so the cost was not a corner: a document
// declaring one got a blocked-resource finding and a fallback face, and the
// suite's own tests — which supply a font precisely so that the shaping and the
// metrics are pinned — were compared against whatever face was lying about.
func TestAWoff2HintIsNotARefusal(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"x.woff2": realFont()}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: url(x.woff2) format("woff2"); }`),
		Resources: res,
	})
	if len(res.asked) == 0 {
		t.Errorf("a woff2 was refused before the resolver was asked for it")
	}
	for _, f := range built.Findings {
		if strings.Contains(f.Message, "which this engine does not read") {
			t.Errorf("a woff2 was reported as a format this engine does not read: %s",
				f.Message)
		}
	}
	// And the family really is available, which is the half a missing finding
	// does not prove: the bytes here are a plain sfnt, and the hint is a hint —
	// what is read is what arrived.
	if set, ok := built.Fonts.(*documentFonts); !ok || len(set.faces) == 0 {
		t.Errorf("the @font-face loaded no face")
	}
}

// TestARealWoff2LoadsThroughFontFace is the end-to-end half, over a file that is
// actually in the format rather than an sfnt wearing its name.
//
// The decoding itself is the font package's, and is tested there against the
// reference implementation. What this adds is that the two meet: a document that
// declares a woff2 and serves one gets the face, with the glyphs and the
// positional forms the file carries.
func TestARealWoff2LoadsThroughFontFace(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wpt", "fonts", "noto",
		"NotoNaskhArabic-regular.woff2"))
	if err != nil {
		t.Skip("no woff2 in the corpus: ", err)
	}
	res := &fileResolver{files: map[string][]byte{"arabic.woff2": data}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial;
			src: url(arabic.woff2) format("woff2"); }`),
		Resources: res,
	})
	set, ok := built.Fonts.(*documentFonts)
	if !ok || len(set.faces) == 0 {
		t.Fatalf("the woff2 loaded no face; findings: %v", built.Findings)
	}
	face := set.faces[0].face
	if face == nil {
		t.Fatal("the face is nil")
	}
	if !face.HasJoiningForms() {
		t.Errorf("the face carries no positional forms; the tables did not survive " +
			"the transform")
	}
	if g, missing := face.ShapeGlyphs("ععع"); len(g) != 3 || missing != 0 {
		t.Errorf("shaping three Arabic letters gave %d glyphs and %d missing",
			len(g), missing)
	}
}

// TestFontUndecodableIsItsOwnFinding checks the rule reaches the report rather
// than only the message inside the rule-level one. The per-entry reason is
// folded into the @font-face finding, so the loader is driven directly.
func TestFontUndecodableIsItsOwnFinding(t *testing.T) {
	l := &fontFaceLoader{
		rec: NewRecorder(nil), base: StandardFonts(),
		res:    &fileResolver{files: map[string][]byte{"junk.ttf": []byte("nope")}},
		loaded: map[string]*shape.Face{}, failed: map[string]bool{},
		budget: maxDocumentFontBytes,
	}
	_, fail := l.load(pendingFontFace{}, fontFaceRule{family: "Trial"},
		fontSource{ref: "junk.ttf"})
	if fail == nil || fail.rule != RuleFontUndecodable {
		t.Fatalf("a font program that does not parse gave %+v, want %s", fail, RuleFontUndecodable)
	}
	fired[RuleFontUndecodable] = true
}

// TestFontFaceWeightAndStyleChoose pins that the descriptors decide which of a
// family's faces a box gets.
//
// The two faces are the same file under two names, so nothing about the *font*
// can be what distinguishes them — only the descriptors, which is what is being
// tested. Which face came back is read off the reference the loader recorded.
func TestFontFaceWeightAndStyleChoose(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"regular.ttf": realFont(), "bold.ttf": realFont(),
		"italic.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: Trial; src: url(regular.ttf); }
			@font-face { font-family: Trial; font-weight: 700; src: url(bold.ttf); }
			@font-face { font-family: Trial; font-style: italic; src: url(italic.ttf); }
		</style><p>x</p>`,
		Resources: res,
	})
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's set is %T; findings: %v", built.Fonts, built.Findings)
	}
	refOf := func(bold, italic bool) string {
		face, ok := set.Face("Trial", bold, italic)
		if !ok {
			t.Fatalf("Trial did not resolve for bold=%v italic=%v", bold, italic)
		}
		for _, df := range set.faces {
			if df.face == face {
				return df.ref
			}
		}
		return "<unknown>"
	}
	// Three files, one face each: the loader shares a face between references
	// only when the reference is the same, so the identity really distinguishes
	// them.
	if len(set.faces) != 3 {
		t.Fatalf("%d faces loaded, want 3", len(set.faces))
	}
	for _, tc := range []struct {
		bold, italic bool
		want         string
	}{
		{false, false, "regular.ttf"},
		{true, false, "bold.ttf"},
		{false, true, "italic.ttf"},
		// Bold italic: nothing is both, and the slant is the more visible
		// difference, so the italic face wins over the bold one.
		{true, true, "italic.ttf"},
	} {
		if got := refOf(tc.bold, tc.italic); got != tc.want {
			t.Errorf("bold=%v italic=%v chose %s, want %s", tc.bold, tc.italic, got, tc.want)
		}
	}
}

// TestWeightRankFollowsTheSpecification pins the one part of font matching that
// is not "closest number", because it is the part that would look like a bug.
func TestWeightRankFollowsTheSpecification(t *testing.T) {
	pick := func(want float64, weights ...valueRange) float64 {
		v, ok := search(weights, weightProbes(want)...)
		if !ok {
			t.Fatalf("no weight was found among %v", weights)
		}
		return v
	}
	one := func(w float64) valueRange { return valueRange{w, w} }
	// At 400, a 500 face beats a 300 one even though 300 is the same distance.
	if got := pick(400, one(500), one(300)); got != 500 {
		t.Errorf("at a desired weight of 400, the 500 and 300 faces gave %v; 500 must be preferred", got)
	}
	// At 400, anything above 500 loses to anything below 400.
	if got := pick(400, one(600), one(100)); got != 100 {
		t.Errorf("at 400, the 600 and 100 faces gave %v; the 600 must lose", got)
	}
	// Above 500 it is heavier-first.
	if got := pick(700, one(900), one(600)); got != 900 {
		t.Errorf("at 700, the 900 and 600 faces gave %v; 900 must be preferred", got)
	}
	// Below 400 it is lighter-first.
	if got := pick(300, one(100), one(400)); got != 100 {
		t.Errorf("at 300, the 100 and 400 faces gave %v; 100 must be preferred", got)
	}
	// An exact match always wins.
	for _, d := range []float64{100, 400, 450, 500, 700, 900} {
		if got := pick(d, one(d-50), one(d), one(d+50)); got != d {
			t.Errorf("an exact match at %v gave %v", d, got)
		}
	}
	// A range covers everything inside it, and a weight past it reaches its
	// nearer end.
	if got := pick(650, valueRange{100, 900}); got != 650 {
		t.Errorf("a weight inside a declared range gave %v, want itself", got)
	}
	if got := pick(900, valueRange{100, 400}); got != 400 {
		t.Errorf("a weight above a declared range gave %v, want its top", got)
	}
	// Between 400 and 500 the search goes up to 500 and no further before it
	// turns: at 450 a 480 face beats a 420 one, and a 420 one a 520 one.
	if got := pick(450, one(420), one(480)); got != 480 {
		t.Errorf("at 450, the 420 and 480 faces gave %v, want 480", got)
	}
	if got := pick(450, one(420), one(520)); got != 420 {
		t.Errorf("at 450, the 420 and 520 faces gave %v, want 420", got)
	}
}

// TestFontWeightAndStyleDescriptorGrammar pins what the descriptors take,
// including the range form a variable font declares and the values that are
// refused — a descriptor this engine cannot read falls back to the default and
// says so, rather than being silently treated as normal.
func TestFontWeightAndStyleDescriptorGrammar(t *testing.T) {
	weight := func(s string) (faceRange, bool) {
		vals, _ := css.ParseComponentValues(s)
		return parseWeightDescriptor(vals)
	}
	for _, tc := range []struct {
		in        string
		low, high float64
		auto, ok  bool
	}{
		{"normal", 400, 400, false, true},
		{"bold", 700, 700, false, true},
		{"350", 350, 350, false, true},
		{"100 900", 100, 900, false, true},
		{"1 1000", 1, 1000, false, true},
		{"auto", 0, 0, true, true},
		{"AUTO", 0, 0, true, true},
		// A reversed range is swapped, as §4.4 says; it was refused.
		{"900 100", 100, 900, false, true},
		{"bold normal", 400, 700, false, true},
		// Refused: out of range, relative, three values, auto in a range, and
		// not a number at all.
		{"0", 0, 0, false, false},
		{"1001", 0, 0, false, false},
		{"bolder", 0, 0, false, false},
		{"lighter", 0, 0, false, false},
		{"400 500 600", 0, 0, false, false},
		{"auto 400", 0, 0, false, false},
		{"", 0, 0, false, false},
		{"heavy", 0, 0, false, false},
	} {
		r, ok := weight(tc.in)
		if ok != tc.ok || (ok && (r.auto != tc.auto || !r.auto && (r.lo != tc.low || r.hi != tc.high))) {
			t.Errorf("font-weight %q gave %+v %v, want %v..%v auto %v %v",
				tc.in, r, ok, tc.low, tc.high, tc.auto, tc.ok)
		}
	}

	width := func(s string) (faceRange, bool) {
		vals, _ := css.ParseComponentValues(s)
		return parseWidthDescriptor(vals)
	}
	for _, tc := range []struct {
		in        string
		low, high float64
		auto, ok  bool
	}{
		{"auto", 0, 0, true, true},
		{"normal", 100, 100, false, true},
		{"condensed", 75, 75, false, true},
		{"ultra-condensed ultra-expanded", 50, 200, false, true},
		{"75% 125%", 75, 125, false, true},
		{"125% 75%", 75, 125, false, true},
		{"0%", 0, 0, false, true},
		{"-1%", 0, 0, false, false},
		{"75", 0, 0, false, false},
		{"squashed", 0, 0, false, false},
		{"50% 75% 100%", 0, 0, false, false},
	} {
		r, ok := width(tc.in)
		if ok != tc.ok || (ok && (r.auto != tc.auto || !r.auto && (r.lo != tc.low || r.hi != tc.high))) {
			t.Errorf("font-width %q gave %+v %v, want %v..%v auto %v %v",
				tc.in, r, ok, tc.low, tc.high, tc.auto, tc.ok)
		}
	}

	style := func(s string) (faceStyle, bool) {
		vals, _ := css.ParseComponentValues(s)
		return parseStyleDescriptor(vals)
	}
	for _, tc := range []struct {
		in   string
		want faceStyle
		ok   bool
	}{
		{"auto", faceStyle{kind: styleAuto}, true},
		{"normal", faceStyle{kind: styleNormal}, true},
		{"italic", faceStyle{kind: styleItalic}, true},
		{"left", faceStyle{kind: styleItalic}, true},
		{"right", faceStyle{kind: styleItalic}, true},
		// An oblique face is oblique, at 14 degrees where it names no angle.
		{"oblique", faceStyle{kind: styleOblique, angles: valueRange{14, 14}}, true},
		{"oblique 14deg", faceStyle{kind: styleOblique, angles: valueRange{14, 14}}, true},
		{"oblique 0deg", faceStyle{kind: styleOblique}, true},
		{"oblique 0", faceStyle{kind: styleOblique}, true},
		{"oblique -20deg 30deg", faceStyle{kind: styleOblique, angles: valueRange{-20, 30}}, true},
		{"oblique 30deg -20deg", faceStyle{kind: styleOblique, angles: valueRange{-20, 30}}, true},
		{"oblique 0.25turn", faceStyle{kind: styleOblique, angles: valueRange{90, 90}}, true},
		{"", faceStyle{}, false},
		{"slanted", faceStyle{}, false},
		{"italic 20deg", faceStyle{}, false},
		{"oblique 20px", faceStyle{}, false},
		{"oblique 91deg", faceStyle{}, false},
		{"oblique 1deg 2deg 3deg", faceStyle{}, false},
	} {
		got, ok := style(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("font-style %q gave %+v %v, want %+v %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}

	// And an unreadable one is reported and defaulted rather than taking the
	// rule down.
	res := &fileResolver{files: map[string][]byte{"a.ttf": realFont()}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url(a.ttf);
			font-weight: heavy; font-style: slanted; font-stretch: squashed; }`),
		Resources: res,
	})
	requireFinding(t, built.Findings, RuleInvalidCSS, "font-weight")
	requireFinding(t, built.Findings, RuleInvalidCSS, "font-style")
	requireFinding(t, built.Findings, RuleInvalidCSS, "font-stretch")
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("an unreadable descriptor took the rule down; findings: %v", built.Findings)
	}
	if r := set.faces[0].rule; !r.weight.auto || !r.width.auto || r.style.kind != styleAuto {
		t.Errorf("the defaults after an unreadable descriptor are %+v, want auto, auto, auto", r)
	}
	fired[RuleInvalidCSS] = true
}

// TestFontFaceUnicodeRangeIsKept. The descriptor used to be parsed, reported and
// thrown away; it is kept now and honoured, so what this holds is that it
// survives the load and that a range restricting nothing leaves no trace.
func TestFontFaceUnicodeRangeIsKept(t *testing.T) {
	load := func(rng string) Built {
		res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
		return Build(Input{
			HTML: docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf);
				unicode-range: ` + rng + `; }`),
			Resources: res,
		})
	}

	// The full range restricts nothing, and is dropped rather than carried: a
	// face with no restriction must ask no questions per character, and an
	// empty list is what says so.
	built := load("U+0-10FFFF")
	for _, f := range built.Findings {
		if f.Property == "unicode-range" {
			t.Errorf("a unicode-range covering the whole of Unicode was reported: %s", f.Message)
		}
	}
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's fonts are %T; findings: %v", built.Fonts, built.Findings)
	}
	if got := set.faces[0].rule.ranges; len(got) != 0 {
		t.Errorf("a full range was kept as %v; it restricts nothing and should be nil", got)
	}

	// A restricted one is kept, and is not reported: it is honoured now, and a
	// finding would be telling an author a declaration did not take effect.
	built = load("U+0025-00FF, U+4??")
	for _, f := range built.Findings {
		if f.Property == "unicode-range" {
			t.Errorf("a restricted unicode-range was reported: %s", f.Message)
		}
	}
	set, ok = built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's fonts are %T", built.Fonts)
	}
	want := []unicodeSpan{{0x25, 0xFF}, {0x400, 0x4FF}}
	if got := set.faces[0].rule.ranges; !reflect.DeepEqual(got, want) {
		t.Errorf("the ranges came through as %v, want %v", got, want)
	}
	if _, ok := built.Fonts.Face("Trial", false, false); !ok {
		t.Error("a face with a restricted unicode-range was dropped")
	}

	// A range this engine cannot read is a descriptor error, and the default —
	// the whole of Unicode — is what is used, which is what CSS says to do with
	// an invalid descriptor.
	built = load("U+ZZZZ")
	requireFinding(t, built.Findings, RuleInvalidCSS, "unicode-range")
	if _, ok := built.Fonts.Face("Trial", false, false); !ok {
		t.Error("an unreadable unicode-range took the whole rule down with it")
	}
	fired[RuleInvalidCSS] = true
}

// TestUnicodeRangeGrammar exercises the three forms and the refusals, because
// the descriptor is reconstructed from tokens that were never meant to carry it
// and a parser that quietly accepted everything would report nothing.
func TestUnicodeRangeGrammar(t *testing.T) {
	parse := func(s string) ([]unicodeSpan, bool) {
		vals, _ := css.ParseComponentValues(s)
		return parseUnicodeRange(vals)
	}
	for _, tc := range []struct {
		in   string
		want []unicodeSpan
	}{
		{"U+26", []unicodeSpan{{0x26, 0x26}}},
		{"u+A5", []unicodeSpan{{0xA5, 0xA5}}},
		{"U+0-10FFFF", []unicodeSpan{{0, 0x10FFFF}}},
		{"U+4??", []unicodeSpan{{0x400, 0x4FF}}},
		{"U+???", []unicodeSpan{{0, 0xFFF}}},
		{"U+0025-00FF, U+4??", []unicodeSpan{{0x25, 0xFF}, {0x400, 0x4FF}}},
	} {
		got, ok := parse(tc.in)
		if !ok {
			t.Errorf("%q did not parse", tc.in)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q gave %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q gave %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
	for _, bad := range []string{
		"", "26", "U+", "U+ZZ", "U+1234567", "U+100-50", "U+4?0",
		"U+110000", "U+0-110000", "U+26,", "url(x)",
	} {
		if got, ok := parse(bad); ok {
			t.Errorf("%q was accepted as %v", bad, got)
		}
	}
	// Coverage is a union rather than a single span, so two halves that meet
	// cover everything and two that do not, do not.
	if !coversAllOfUnicode([]unicodeSpan{{0, 0xFFFF}, {0x10000, 0x10FFFF}}) {
		t.Error("two spans that meet were not seen to cover Unicode")
	}
	if coversAllOfUnicode([]unicodeSpan{{0, 0xFFFF}, {0x10001, 0x10FFFF}}) {
		t.Error("two spans with a gap between them were seen to cover Unicode")
	}
	if coversAllOfUnicode([]unicodeSpan{{0, 0x10FFFE}}) {
		t.Error("a span one short of the end was seen to cover Unicode")
	}
}

// TestFontFaceDescriptorsThatChangeMetricsAreReported pins that a descriptor
// this engine drops is not dropped silently. size-adjust and the override
// descriptors move the text on the page.
func TestFontFaceDescriptorsThatChangeMetricsAreReported(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf);
			size-adjust: 120%; ascent-override: 90%; }`),
		Resources: res,
	})
	requireFinding(t, built.Findings, RuleUnsupportedProperty, "size-adjust")
	requireFinding(t, built.Findings, RuleUnsupportedProperty, "ascent-override")
	fired[RuleUnsupportedProperty] = true

	// font-display is not one of them: it says what to show while a font is
	// downloading, and nothing here downloads.
	built = Build(Input{
		HTML: docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf);
			font-display: swap; }`),
		Resources: res,
	})
	for _, f := range built.Findings {
		if f.Property == "font-display" {
			t.Errorf("font-display was reported: %s", f.Message)
		}
	}
}

// TestFontFaceWithoutFamilyOrSrc pins that a rule which cannot mean anything is
// reported as malformed rather than as a feature this engine lacks. An author
// sent to the wrong one of those looks for the wrong thing.
func TestFontFaceWithoutFamilyOrSrc(t *testing.T) {
	built := Build(Input{HTML: docWithFontFace(`@font-face { src: url(x.ttf); }`)})
	requireFinding(t, built.Findings, RuleInvalidCSS, "names no font-family")

	built = Build(Input{HTML: docWithFontFace(`@font-face { font-family: Trial; }`)})
	requireFinding(t, built.Findings, RuleInvalidCSS, "names no usable src")

	built = Build(Input{HTML: docWithFontFace(`@font-face { font-family: Trial; src: none; }`)})
	requireFinding(t, built.Findings, RuleInvalidCSS, "names no usable src")
	fired[RuleInvalidCSS] = true
}

// TestFontFaceOneReadPerReference pins that a document naming one file in
// several rules reads and parses it once. Otherwise the caps above would be
// bypassable by repetition, which is how a cache becomes an amplifier.
func TestFontFaceOneReadPerReference(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: One; src: url(trial.ttf); }
			@font-face { font-family: Two; src: url(trial.ttf); }
			@font-face { font-family: Three; src: url(trial.ttf); }
		</style><p>x</p>`,
		Resources: res,
	})
	if len(res.asked) != 1 {
		t.Errorf("one file named by three rules was read %d times", len(res.asked))
	}
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's set is %T", built.Fonts)
	}
	for _, name := range []string{"One", "Two", "Three"} {
		if _, ok := set.Face(name, false, false); !ok {
			t.Errorf("%s did not resolve", name)
		}
	}
	// And a failed reference is likewise attempted once.
	res = &fileResolver{files: map[string][]byte{}}
	Build(Input{
		HTML: `<style>
			@font-face { font-family: One; src: url(gone.ttf); }
			@font-face { font-family: Two; src: url(gone.ttf); }
		</style><p>x</p>`,
		Resources: res,
	})
	if len(res.asked) != 1 {
		t.Errorf("one missing file named by two rules was attempted %d times", len(res.asked))
	}
}

// TestFontFaceLastDeclarationWins pins the tie-break, which is the cascade's
// last term applied to a rule that is not in the cascade: a document that
// redeclares a family later means the later one.
func TestFontFaceLastDeclarationWins(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"first.ttf": realFont(), "second.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: Trial; src: url(first.ttf); }
			@font-face { font-family: Trial; src: url(second.ttf); }
		</style><p>x</p>`,
		Resources: res,
	})
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's set is %T", built.Fonts)
	}
	face, _ := set.Face("Trial", false, false)
	for _, df := range set.faces {
		if df.face == face && df.ref != "second.ttf" {
			t.Errorf("the family resolved to %s, want the later declaration", df.ref)
		}
	}
}

// TestFontFaceKeepsTheFallbackInterface pins that wrapping the caller's set does
// not lose what it could do. A FallbackFontSet that stopped answering the
// coverage question would leave every script the standard faces lack reporting a
// missing glyph, which is a large and silent regression.
func TestFontFaceKeepsTheFallbackInterface(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	in := Input{
		HTML:      docWithFontFace(`@font-face { font-family: Trial; src: url(trial.ttf); }`),
		Resources: res,
	}

	built := Build(in)
	if _, ok := built.Fonts.(FallbackFontSet); ok {
		t.Error("wrapping a plain set invented a fallback it cannot answer")
	}

	in.Fonts = fallbackOnly{StandardFonts()}
	built = Build(in)
	fb, ok := built.Fonts.(FallbackFontSet)
	if !ok {
		t.Fatal("wrapping a fallback set lost the interface")
	}
	if _, ok := fb.FaceFor("anything", false, false); !ok {
		t.Error("the wrapped set did not pass the coverage question through")
	}
}

// fallbackOnly is a FontSet that also answers the coverage question, for the
// wrapping test above.
type fallbackOnly struct{ FontSet }

func (f fallbackOnly) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	return f.FontSet.Face("sans-serif", false, false)
}
