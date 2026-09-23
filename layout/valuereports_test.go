package layout

import (
	"strings"
	"testing"
)

// text-spacing-trim is read per character, from the element the character is
// in, and the block's own value does not decide for its descendants (audit
// C145). The paragraph below asks for "space-all" — keep every full-width
// blank — and the span holding its closing bracket asks for "normal", which
// trims that bracket at the end of a line that would not otherwise hold it.
// Eight full-width characters in 7.5em fit only if it is trimmed.
func TestAnInnerElementsTrimIsHonoured(t *testing.T) {
	set := cjkFace(t)
	lines := func(block, span string) []string {
		t.Helper()
		root := layoutWithFonts(t, set,
			`<div id="p" lang="ja">国国国国国国国<span style="text-spacing-trim: `+span+
				`">）</span></div>`,
			`#p { font-size: 20px; width: 150px; text-spacing-trim: `+block+` }`)
		return lineTextsOf(t, root, "p")
	}
	// The controls: the face does trim the bracket, and "space-all" does not.
	if got := lines("normal", "normal"); len(got) != 1 {
		t.Fatalf("with \"normal\" throughout the bracket did not trim: %q", got)
	}
	if got := lines("space-all", "space-all"); len(got) != 2 {
		t.Fatalf("with \"space-all\" throughout the line still held all eight: %q", got)
	}
	if got := lines("space-all", "normal"); len(got) != 1 {
		t.Errorf("the span's own \"normal\" was not honoured inside a \"space-all\" "+
			"paragraph: %q", strings.Join(got, "|"))
	}
}

// TestAnInnerElementsTrimIsReported: the values whose line-start rule is not
// done are reported wherever they are declared, and each value once — not the
// block's value only, and not the first value met only (audit C146).
func TestAnInnerElementsTrimIsReported(t *testing.T) {
	rec := NewRecorder(nil)
	built := Build(Input{
		HTML: `<p>a<span style="text-spacing-trim: trim-start">b</span>` +
			`<span style="text-spacing-trim: space-first">c</span></p>`,
	})
	Layout(built.Root, A4.Content(), nil, rec)
	said := map[string]bool{}
	for _, f := range rec.Findings() {
		if f.Property == "text-spacing-trim" {
			for _, v := range []string{"trim-start", "space-first"} {
				if strings.Contains(f.Message, v) {
					said[v] = true
					if !strings.HasSuffix(f.Path, "span") {
						t.Errorf("the finding about %s names %q, not the span that "+
							"declares it", v, f.Path)
					}
				}
			}
		}
	}
	for _, v := range []string{"trim-start", "space-first"} {
		if !said[v] {
			t.Errorf("text-spacing-trim: %s on an inner element was not reported", v)
		}
	}
}

// TestEachUnappliedFeatureValueIsReported: two different font-feature-settings
// values are two different problems, and the report was keyed on the property
// alone, so the second value met was never mentioned (audit C146).
func TestEachUnappliedFeatureValueIsReported(t *testing.T) {
	rec := NewRecorder(nil)
	built := Build(Input{
		HTML: `<p><span class="a">a</span><span class="b">b</span></p>`,
		CSS: []Stylesheet{{Source: `.a { font-feature-settings: "smcp" }
			.b { font-feature-settings: "liga" 0 }`}},
	})
	Layout(built.Root, A4.Content(), nil, rec)
	said := map[string]bool{}
	for _, f := range rec.Findings() {
		if f.Property == "font-feature-settings" {
			for _, v := range []string{`smcp`, `liga`} {
				if strings.Contains(f.Message, v) {
					said[v] = true
				}
			}
		}
	}
	for _, v := range []string{`smcp`, `liga`} {
		if !said[v] {
			t.Errorf("font-feature-settings naming %q was not reported: %v", v, rec.Findings())
		}
	}
}
