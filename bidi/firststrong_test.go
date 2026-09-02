package bidi

import "testing"

// P2 and P3 asked separately, which is what a caller with information of its own
// needs.
//
// Resolve answers P3's "otherwise, set it to zero" itself, and that is right for
// a paragraph nobody knows anything else about. "unicode-bidi: plaintext" is the
// case that knows something else: css-writing-modes gives a paragraph with no
// strong character the direction of the paragraph before it, and of the
// containing block where there is none.

func TestFirstStrongFindsTheDirection(t *testing.T) {
	for _, tc := range []struct {
		text  string
		dir   Direction
		found bool
		what  string
	}{
		{"abc", LeftToRight, true, "Latin"},
		{"שלום", RightToLeft, true, "Hebrew"},
		{"سلام", RightToLeft, true, "Arabic, which is class AL"},
		{"! abc", LeftToRight, true, "a neutral before a Latin letter"},
		{"! سلام", RightToLeft, true, "a neutral before an Arabic letter"},
		{"123 abc", LeftToRight, true, "digits, which are weak, before a letter"},
		{"!", LeftToRight, false, "nothing but a neutral"},
		{"123", LeftToRight, false, "nothing but digits"},
		{"", LeftToRight, false, "no text at all"},
		{" \t ", LeftToRight, false, "nothing but white space"},
	} {
		dir, found := FirstStrong([]rune(tc.text))
		if found != tc.found {
			t.Errorf("%s: FirstStrong(%q) found=%v, want %v", tc.what, tc.text, found, tc.found)
			continue
		}
		if found && dir != tc.dir {
			t.Errorf("%s: FirstStrong(%q) = %v, want %v", tc.what, tc.text, dir, tc.dir)
		}
	}
}

func TestFirstStrongSkipsAnIsolate(t *testing.T) {
	// P2's own words: the contents of an isolate say nothing about the text
	// around it. A Hebrew word inside one does not make the paragraph
	// right-to-left, and a Latin one after the isolate still does.
	const rli, pdi = "⁧", "⁩"
	if dir, found := FirstStrong([]rune(rli + "שלום" + pdi + "abc")); !found || dir != LeftToRight {
		t.Errorf("an isolated Hebrew word before Latin gave %v found=%v, want "+
			"left to right", dir, found)
	}
	if _, found := FirstStrong([]rune(rli + "שלום" + pdi)); found {
		t.Error("a paragraph that is nothing but an isolate found a strong " +
			"character; its contents are isolated from the paragraph")
	}
}

func TestFirstStrongAgreesWithResolve(t *testing.T) {
	// Where P3's fallback is not needed the two must give the same level, or a
	// caller that asks this and then calls Resolve with the answer would get a
	// different paragraph from one that asked Resolve for Auto.
	for _, text := range []string{"abc", "שלום", "! سلام", "123 abc"} {
		dir, found := FirstStrong([]rune(text))
		if !found {
			t.Fatalf("%q has a strong character and none was found", text)
		}
		want := Resolve([]rune(text), Auto).Level()
		got := Resolve([]rune(text), dir).Level()
		if got != want {
			t.Errorf("%q: resolving at the reported direction gives level %d and "+
				"Auto gives %d", text, got, want)
		}
	}
}
