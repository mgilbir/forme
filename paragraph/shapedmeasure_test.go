package paragraph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What the line breaker measures has to be what the page draws.
//
// It measures a word to decide whether the word fits, and the backend then
// shapes the same word to draw it. If the two disagree the line is filled to one
// width and painted at another, and nothing in either call's output shows it —
// which is the fault MeasureShaped's own comment was written to warn about, in a
// function that had no callers.
//
// A ligature, a kern pair and a contextual substitution are all invisible to a
// per-rune sum and all change the advance. Over the HarfBuzz corpus the sum was
// recorded as wrong for 1920 of 5911 strings, by up to 17% on a Devanagari
// conjunct.

func shapedFace(t *testing.T, name string) *shape.Face {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that shapes")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Skipf("%s is not in the checkout: %v", name, err)
	}
	f, err := shape.Load(data)
	if err != nil {
		t.Fatalf("%s did not load: %v", name, err)
	}
	return f
}

// TestMeasuringAgreesWithShaping is the property, stated as the agreement rather
// than as a number: whatever the width comes to, the two have to say the same
// thing about it.
func TestMeasuringAgreesWithShaping(t *testing.T) {
	for _, name := range []string{
		"NotoSansDevanagari-Regular.ttf",
		"NotoSansArabic-Regular.ttf",
		"NotoSans-Regular.ttf",
	} {
		f := shapedFace(t, name)
		br := NewBreaker(nil)
		size, _ := style.FromPx(16)
		for _, text := range []string{
			"क्ष", "हिन्दी", "कि",
			"العربية", "لا",
			"AVATAR", "fi", "Wo", "To",
			"x", "",
		} {
			got := br.MeasureSpaced(f, text, size, TextSpacing{})
			want, _ := style.FromPx(f.MeasureShaped(text, size.Px()))
			if got != want {
				t.Errorf("%s: %q measured %v and shapes to %v", name, text, got, want)
			}
		}
	}
}

// TestShapingChangesTheAnswerSomewhere is what keeps the test above from being
// vacuous. If the two were the same function, the agreement would prove nothing
// — so at least one of these strings has to be a string the per-rune sum gets
// wrong, and this says which.
func TestShapingChangesTheAnswerSomewhere(t *testing.T) {
	f := shapedFace(t, "NotoSansDevanagari-Regular.ttf")
	differed := 0
	for _, text := range []string{"क्ष", "हिन्दी", "कि", "क्", "द्ध"} {
		if f.Measure(text, 16) != f.MeasureShaped(text, 16) {
			differed++
		}
	}
	if differed == 0 {
		t.Errorf("the per-rune sum and the shaped measurement agree on every string " +
			"here, so TestMeasuringAgreesWithShaping is measuring nothing")
	}
}

// TestTheMemoDoesNotConfuseTwoFaces. Shaping is expensive enough that the answer
// is remembered, and the key has to hold everything that changes it — which is
// what makes the memo safe to keep now that the answer costs more to compute.
func TestTheMemoDoesNotConfuseTwoFaces(t *testing.T) {
	a := shapedFace(t, "NotoSans-Regular.ttf")
	b := shapedFace(t, "NotoSansDevanagari-Regular.ttf")
	br := NewBreaker(nil)
	size, _ := style.FromPx(16)
	first := br.MeasureSpaced(a, "x", size, TextSpacing{})
	second := br.MeasureSpaced(b, "x", size, TextSpacing{})
	wantA, _ := style.FromPx(a.MeasureShaped("x", 16))
	wantB, _ := style.FromPx(b.MeasureShaped("x", 16))
	if first != wantA || second != wantB {
		t.Errorf("two faces measured %v and %v, want %v and %v", first, second, wantA, wantB)
	}
	// And the size, which scales the answer.
	big, _ := style.FromPx(32)
	if got := br.MeasureSpaced(a, "x", big, TextSpacing{}); got == first {
		t.Errorf("the same text at two sizes measured %v both times", got)
	}
}
