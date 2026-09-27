package diag

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextShowsWhatTheEngineRead(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"السلام", "السلام"},
		// The crasher the scheduled fuzz run found: a byte that begins no
		// character, one replacement character for it.
		{"a\x93", "a�"},
		// One per byte, as the reader walks them, and not one per run.
		{"\x93\x93", "��"},
		// A truncated sequence is bytes that begin no character too.
		{"x\xe2\x82", "x��"},
		// A replacement character the author wrote is text, and kept.
		{"�", "�"},
		{"a\x00b\tc\nd\x1b[2Je\x7f", "a?b?c?d?[2Je?"},
		// C1: U+0085 NEXT LINE and U+009B, the CSI of an escape sequence.
		{"a\u0085b\u009bc", "a?b?c"},
		// U+00A0 is the first code point past C1 and is not a control.
		{"a b", "a b"},
	} {
		if got := Text(tc.in); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCutAndQuote(t *testing.T) {
	for _, tc := range []struct {
		in       string
		max      int
		cut, quo string
	}{
		{"abc", 3, "abc", `"abc"`},
		{"abcd", 3, "abc...", `"abc..."`},
		// The cut is at a character, never inside one.
		{"aé", 2, "aé", `"aé"`},
		{"aéb", 2, "aé...", `"aé..."`},
		{"a\x93b", 2, "a�...", "\"a�...\""},
		{"\x01", 40, "?", `"?"`},
	} {
		if got := Cut(tc.in, tc.max); got != tc.cut {
			t.Errorf("Cut(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.cut)
		}
		if got := Quote(tc.in, tc.max); got != tc.quo {
			t.Errorf("Quote(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.quo)
		}
	}
}

// TestWhatComesOutIsText holds the three functions to their one promise over
// random bytes, weighted towards the ones that break it.
func TestWhatComesOutIsText(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{"a", "é", "\x93", "\xe2\x82", "\x00", "\n", "\x7f",
		"\u0085", "\u009f", " ", "�", "😀"}
	for n := 0; n < 20000; n++ {
		var b strings.Builder
		for k := rng.Intn(12); k > 0; k-- {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		s := b.String()
		for _, out := range []string{Text(s), Cut(s, rng.Intn(10)), Quote(s, rng.Intn(10))} {
			if !utf8.ValidString(out) {
				t.Fatalf("%q came out as %q, which is not UTF-8", s, out)
			}
			for _, r := range out {
				if r < 0x20 || r >= 0x7f && r <= 0x9f {
					t.Fatalf("%q came out as %q, which holds the control %U", s, out, r)
				}
			}
			if !IsText(out) {
				t.Fatalf("%q came out as %q, which IsText refuses", s, out)
			}
		}
		if IsText(s) != (Text(s) == s) {
			t.Fatalf("IsText(%q) = %v, and Text gives %q", s, IsText(s), Text(s))
		}
		if Text(Text(s)) != Text(s) {
			t.Fatalf("Text is not idempotent on %q", s)
		}
	}
}

// TestTextAllocatesNothingForText is what lets a sink pass every message
// through it.
func TestTextAllocatesNothingForText(t *testing.T) {
	msg := "the tag <a is never closed — «ok» 😀"
	if n := testing.AllocsPerRun(100, func() { _ = Text(msg) }); n != 0 {
		t.Fatalf("Text allocated %v times for a message with nothing to change", n)
	}
}
