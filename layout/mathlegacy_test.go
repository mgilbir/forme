package layout

import (
	"strings"
	"testing"
)

// TestMathML3sElementsAreReported: each element MathML 3 draws and Core lays
// out as a row says what is lost; <none/>, drawn as nothing by both, does not,
// and nor does an element that is not displayed.
func TestMathML3sElementsAreReported(t *testing.T) {
	for name := range mathLegacyElements {
		_, findings := mathLayout(t, `<math><`+name+`><mn>1</mn></`+name+`></math>`)
		if !mathFinding(findings, RuleUnsupportedElement, "<"+name+"> is MathML 3's") {
			t.Errorf("<%s> is not reported: %v", name, findings)
		}
	}
	for _, doc := range []string{
		`<math><mmultiscripts><mn>1</mn><none/><mn>2</mn></mmultiscripts></math>`,
		`<math><menclose style="display: none"><mn>1</mn></menclose></math>`,
		`<math><semantics><mn>1</mn><annotation-xml><menclose></menclose></annotation-xml></semantics></math>`,
	} {
		_, findings := mathLayout(t, doc)
		if mathFinding(findings, RuleUnsupportedElement, "MathML 3") {
			t.Errorf("%s: reported %v", doc, findings)
		}
	}
}

// TestMathML3sAttributesAreReported: attributes Core does not have that
// change the layout in MathML 3 are reported, on the elements they belong to;
// those with no effect in MathML 3 are not, and nor is an attribute of the
// same name on an element it means nothing on.
func TestMathML3sAttributesAreReported(t *testing.T) {
	for _, tc := range []struct {
		doc, attr string
		reported  bool
	}{
		{`<mtable columnalign="left"><mtr><mtd><mn>1</mn></mtd></mtr></mtable>`, "columnalign", true},
		{`<mtable rowlines="solid"><mtr><mtd><mn>1</mn></mtd></mtr></mtable>`, "rowlines", true},
		{`<mfrac bevelled="true"><mn>1</mn><mn>2</mn></mfrac>`, "bevelled", true},
		{`<mstyle scriptminsize="8pt"><mn>1</mn></mstyle>`, "scriptminsize", true},
		{`<mn color="red">1</mn>`, "color", true},
		{`<mi fontweight="bold">x</mi>`, "fontweight", true},
		{`<mo linebreak="newline">+</mo>`, "linebreak", true},
		{`<msub subscriptshift="1em"><mn>1</mn><mn>2</mn></msub>`, "subscriptshift", true},
		{`<mo fence="true">(</mo>`, "fence", false},
		{`<mo separator="true">,</mo>`, "separator", false},
		{`<mfrac align="left"><mn>1</mn><mn>2</mn></mfrac>`, "align", false},
		{`<mi mathvariant="bold">x</mi>`, "mathvariant", true},
		{`<mi mathvariant="italic">sin</mi>`, "mathvariant", true},
		{`<mi mathvariant="italic">x</mi>`, "mathvariant", false},
		{`<mi mathvariant="italic">x<span>y</span></mi>`, "mathvariant", false},
		{`<mi mathvariant="italic">x<span>yz</span></mi>`, "mathvariant", true},
		{`<mn mathvariant="italic">1</mn>`, "mathvariant", true},
		{`<mi mathvariant="NORMAL">x</mi>`, "mathvariant", false},
		{`<mn mathsize="big">1</mn>`, "mathsize", true},
		{`<mn mathsize="2em">1</mn>`, "mathsize", false},
	} {
		_, findings := mathLayout(t, `<math>`+tc.doc+`</math>`)
		got := false
		for _, f := range findings {
			got = got || f.Rule == RuleUnsupportedValue && strings.HasPrefix(f.Message, tc.attr+"=")
		}
		if got != tc.reported {
			t.Errorf("%s: %s reported %v, want %v: %v", tc.doc, tc.attr, got, tc.reported, findings)
		}
	}
}

// TestMathML3sLengthsAreReported: the lengths MathML 3 took and Core does not
// read are reported — and <mpadded>'s increments, which Core reads as a signed
// length — and every length Core does read is not.
func TestMathML3sLengthsAreReported(t *testing.T) {
	for _, tc := range []struct {
		doc, want string
	}{
		{`<mspace width="thickmathspace"></mspace>`, "a named space"},
		{`<mspace width="negativethinmathspace"></mspace>`, "a named space"},
		{`<mspace height="2"></mspace>`, "a number with no unit"},
		{`<mspace height="-1.5"></mspace>`, "a number with no unit"},
		{`<mpadded width="150%height"><mn>1</mn></mpadded>`, "pseudo-unit of the content's own height"},
		{`<mpadded lspace="2width"><mn>1</mn></mpadded>`, "pseudo-unit of the content's own width"},
		{`<mpadded width="+2px"><mn>1</mn></mpadded>`, "an increment"},
		{`<mpadded depth="-1em"><mn>1</mn></mpadded>`, "an increment"},
		{`<mo maxsize="infinity">(</mo>`, "infinity"},
		{`<mo lspace="thinmathspace">+</mo>`, "a named space"},
		{`<mspace width="0"></mspace>`, ""},
		{`<mspace width="0.0"></mspace>`, ""},
		{`<mspace width="2px" height="1em" depth="10%"></mspace>`, ""},
		{`<mpadded voffset="-2px"><mn>1</mn></mpadded>`, ""},
		{`<mo lspace="+2px">+</mo>`, ""},
		{`<mi width="thickmathspace">x</mi>`, ""},
	} {
		_, findings := mathLayout(t, `<math>`+tc.doc+`</math>`)
		var got []string
		for _, f := range findings {
			if f.Rule == RuleUnsupportedValue {
				got = append(got, f.Message)
			}
		}
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%s: reported %q", tc.doc, got)
		case tc.want != "" && (len(got) != 1 || !strings.Contains(got[0], tc.want)):
			t.Errorf("%s: reported %q, want one finding saying %q", tc.doc, got, tc.want)
		}
	}
}
