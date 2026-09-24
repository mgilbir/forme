package ascii

import (
	"math/rand"
	"strings"
	"testing"
)

// The characters Unicode calls white space and CSS and HTML do not, and one,
// the byte order mark, that neither calls white space and that a reader might
// expect to be.
const (
	nbsp     = "\u00a0" // NO-BREAK SPACE
	emSpace  = "\u2003" // EM SPACE
	ideoSp   = "\u3000" // IDEOGRAPHIC SPACE
	nel      = "\u0085" // NEXT LINE
	lineSep  = "\u2028" // LINE SEPARATOR
	bom      = "\ufeff" // ZERO WIDTH NO-BREAK SPACE, which is not White_Space either
	vertical = "\v"     // LINE TABULATION, which unicode.IsSpace takes and neither syntax does
)

// TestTheSpaceSetsAreTheSpecifications holds both predicates to their five
// bytes exactly, over every byte.
func TestTheSpaceSetsAreTheSpecifications(t *testing.T) {
	want := map[byte]bool{' ': true, '\t': true, '\n': true, '\f': true, '\r': true}
	for c := 0; c < 256; c++ {
		if got := IsSpace(byte(c)); got != want[byte(c)] {
			t.Errorf("IsSpace(%#02x) = %v, want %v", c, got, want[byte(c)])
		}
		if got := IsCSSSpace(byte(c)); got != want[byte(c)] {
			t.Errorf("IsCSSSpace(%#02x) = %v, want %v", c, got, want[byte(c)])
		}
	}
}

func TestTrimTakesOnlyTheSyntaxsWhiteSpace(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"solid", "solid"},
		{" \t\n\f\rsolid \t\n\f\r", "solid"},
		{"a b", "a b"},
		{"   ", ""},
		// None of these is white space to CSS or HTML, and each stays.
		{"solid" + emSpace, "solid" + emSpace},
		{nbsp + "2", nbsp + "2"},
		{ideoSp + "a" + ideoSp, ideoSp + "a" + ideoSp},
		{nel + "x", nel + "x"},
		{"x" + lineSep, "x" + lineSep},
		{bom + "x", bom + "x"},
		{vertical + "x" + vertical, vertical + "x" + vertical},
		// Inside the white space that is trimmed, they stop the trim.
		{" " + nbsp + " x ", nbsp + " x"},
	} {
		if got := TrimSpace(tc.in); got != tc.want {
			t.Errorf("TrimSpace(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := TrimCSSSpace(tc.in); got != tc.want {
			t.Errorf("TrimCSSSpace(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFieldsSplitsOnlyOnTheSyntaxsWhiteSpace(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{" \t ", nil},
		{"a", []string{"a"}},
		{" a  b\tc\nd\fe\rf ", []string{"a", "b", "c", "d", "e", "f"}},
		{"a" + nbsp + "b", []string{"a" + nbsp + "b"}},
		{"a" + emSpace + "b c", []string{"a" + emSpace + "b", "c"}},
		{ideoSp, []string{ideoSp}},
		{"x" + bom + " y", []string{"x" + bom, "y"}},
		{"stylesheet" + nbsp, []string{"stylesheet" + nbsp}},
	} {
		for name, f := range map[string]func(string) []string{"Fields": Fields, "CSSFields": CSSFields} {
			got := f(tc.in)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Errorf("%s(%q) = %q, want %q", name, tc.in, got, tc.want)
			}
		}
	}
}

// TestTheSplitsAgreeWithGosOnASCII holds Fields and TrimSpace to the standard
// library's where the two sets meet: on text with no white space outside the
// five bytes, and none of the ASCII controls Unicode adds (U+000B and U+001C
// to U+001F), Go's answer is the specification's, and an implementation that
// lost a field, kept an empty one or trimmed a byte too many would part from
// it here. The invalid byte is there because strings.Fields reads it as
// U+FFFD, which is not white space either, so the answers must still agree.
func TestTheSplitsAgreeWithGosOnASCII(t *testing.T) {
	alphabet := []string{" ", "\t", "\n", "\f", "\r", "a", "b", "-", "é", "漢", "\xff"}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for n := r.Intn(12); n > 0; n-- {
			b.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		s := b.String()
		if got, want := Fields(s), strings.Fields(s); strings.Join(got, "\x00") != strings.Join(want, "\x00") || len(got) != len(want) {
			t.Fatalf("Fields(%q) = %q, strings.Fields = %q", s, got, want)
		}
		if got, want := TrimSpace(s), strings.TrimSpace(s); got != want {
			t.Fatalf("TrimSpace(%q) = %q, strings.TrimSpace = %q", s, got, want)
		}
	}
}

func TestTrimAndFieldsDoNotCopy(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() { sink = TrimCSSSpace("  solid  ") }); n != 0 {
		t.Errorf("TrimCSSSpace allocated %v times", n)
	}
	// One allocation, the slice; the words are the input's own bytes.
	if n := testing.AllocsPerRun(100, func() { sinks = CSSFields(" a b c ") }); n != 1 {
		t.Errorf("CSSFields allocated %v times, want 1", n)
	}
}

var sinks []string
