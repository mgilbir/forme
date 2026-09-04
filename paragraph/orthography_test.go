package paragraph

import "testing"

// CSS Text §6.3's language-specific hyphenation, over the characters at a break.
//
// §6.3.1 says it of "hyphens: manual" in one sentence — the UA "must use the
// appropriate language-specific hyphenation character(s) and should apply any
// appropriate spelling changes just as for automatic hyphenation at the same
// point" — and §6.3 gives a table of five languages. Three of them are rules
// over the characters either side of the break and are here; the other two are
// in the file comment, with why they are not.

func TestALanguageTagIsReadAsTheRulesItsWordsAreBrokenBy(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want Orthography
		what string
	}{
		{"hu", OrthographyHungarian, "the primary subtag is enough"},
		{"HU-hu", OrthographyHungarian, "case and a region change nothing"},
		{"zh-Latn-pinyin", OrthographyPinyin, "the suite's own tag"},
		{"zh-Latn", OrthographyPinyin, "romanised without naming the romanisation"},
		{"zh", OrthographyPlain, "Chinese in its own script has no apostrophe to drop"},
		{"zh-Hant", OrthographyPlain, "and neither has it with the script written out"},
		{"ug", OrthographyUyghur, "Uyghur, which is written in the Arabic script"},
		{"ug-Arab-CN", OrthographyUyghur, "with the script and a region"},
		{"ug-Latn", OrthographyPlain, "romanised Uyghur is hyphenated like Latin"},
		{"en", OrthographyPlain, "English, which is the row that needs nothing"},
		{"", OrthographyPlain, "no language at all"},
		{"nl", OrthographyPlain, "Dutch, whose rule is a dictionary — see the file comment"},
	} {
		if got := OrthographyOf(tc.tag); got != tc.want {
			t.Errorf("%q: got %v, want %v — %s", tc.tag, got, tc.want, tc.what)
		}
	}
}

// Hungarian: "Összeg" divides as "Ösz-" and "szeg". The word is written with one
// "s" and one "sz" and pronounced with two "sz", so the break writes the digraph
// out on both sides. It is the suite's hyphens-i18n-manual-002.
func TestHungarianWritesADoubledDigraphOutOnBothSides(t *testing.T) {
	for _, tc := range []struct {
		before, after, want, what string
	}{
		{"Ös", "szeg", "z", "the suite's own word"},
		{"ÖS", "SZEG", "Z", "in the case the document wrote it in"},
		{"me", "gy", "", "a digraph whose first letter is not the one before it"},
		{"meg", "gyek", "y", "a doubled gy"},
		{"kes", "sz", "z", "at the very end of the text"},
		{"viz", "zsel", "s", "zs, which doubles as z + zs"},
		{"vis", "zsel", "", "an s in front of a zs is not a doubled digraph"},
		{"", "szeg", "", "nothing before the break"},
		{"Ös", "s", "", "a single letter is not a digraph"},
		{"Ös", "zeg", "", "sz is the digraph, not sz's second letter alone"},
	} {
		got := OrthographyHungarian.HyphenateBetween(tc.before, tc.after)
		if got.Restored != tc.want {
			t.Errorf("%q|%q: restored %q, want %q — %s",
				tc.before, tc.after, got.Restored, tc.want, tc.what)
		}
		if got.Character != "" || got.Dropped != 0 || got.Lead != "" {
			t.Errorf("%q|%q: Hungarian asked for more than a spelling change: %+v",
				tc.before, tc.after, got)
		}
	}
	// The last three rows are the rule's own edges, and the pair above them is
	// what it turns on: a doubled digraph is written with its *first* letter
	// twice, so what identifies one is that the letter before the break is the
	// letter the digraph after it begins with. "viz|zsel" is a doubled zs and
	// "vis|zsel" is an s standing in front of an ordinary one.
}

// Pinyin: "tú’àn" divides as "tú-" and "àn". The apostrophe is there to stop
// two syllables being read as one, and a hyphen at the same place does that
// work, so it goes. The suite's hyphens-i18n-manual-003.
func TestPinyinDropsTheSyllableSeparator(t *testing.T) {
	for _, tc := range []struct {
		after string
		want  int
		what  string
	}{
		{"’àn", len("’"), "the typographic apostrophe, which the suite writes"},
		{"'àn", 1, "the one a keyboard produces"},
		{"àn", 0, "no separator to drop"},
		{"", 0, "nothing after the break"},
		{"‐fēnmíng", 0, "a hyphen the document wrote stays where it is"},
	} {
		got := OrthographyPinyin.HyphenateBetween("tú", tc.after)
		if got.Dropped != tc.want {
			t.Errorf("tú|%q: dropped %d bytes, want %d — %s",
				tc.after, got.Dropped, tc.want, tc.what)
		}
		if got.Restored != "" || got.Character != "" || got.Lead != "" {
			t.Errorf("tú|%q: pinyin asked for more than a character dropped: %+v",
				tc.after, got)
		}
	}
}

// Uyghur hyphenates with U+0640 ARABIC TATWEEL and keeps the letters joined
// across the break, which is what §6.3's own note asks for: "when shaping
// scripts such as Arabic are allowed to break within words due to hyphenation,
// the characters are still shaped as if the word were not broken". The suite's
// hyphens-i18n-manual-005.
func TestUyghurHyphenatesWithATatweelAndKeepsTheLettersJoined(t *testing.T) {
	got := OrthographyUyghur.HyphenateBetween("دامي", "دى")
	if got.Character != "ـ" {
		t.Errorf("the hyphen is %q, want a tatweel", got.Character)
	}
	if got.Restored != "‍" || got.Lead != "‍" {
		t.Errorf("the joiners are %q and %q, want one either side of the break",
			got.Restored, got.Lead)
	}
	if got.Dropped != 0 {
		t.Errorf("Uyghur dropped %d bytes; it takes nothing away", got.Dropped)
	}
	// A break beside a character that joins nothing has nothing to keep joined,
	// and a tatweel drawn there is a stroke hanging off nothing.
	if got := OrthographyUyghur.HyphenateBetween("abc", "دى"); got.Any() {
		t.Errorf("after Latin: %+v, want nothing", got)
	}
	if got := OrthographyUyghur.HyphenateBetween("دامي", "abc"); got.Lead != "" {
		t.Errorf("before Latin: the joiner is %q, want none — there is nothing "+
			"on the far side for it to join to", got.Lead)
	}
}

// The soft hyphen is at the end of the text before the break, because a piece
// keeps the character that marked it. So are the joiners and the other controls
// that draw nothing, and none of them is the letter these rules are about.
func TestTheMarkThatAskedForTheBreakIsNotReadAsALetter(t *testing.T) {
	if got := OrthographyHungarian.HyphenateBetween("Ös­", "szeg"); got.Restored != "z" {
		t.Errorf("with the soft hyphen still on it: restored %q, want %q — the "+
			"rule read the mark as the letter before the break", got.Restored, "z")
	}
	if got := OrthographyUyghur.HyphenateBetween("دامي­", "دى"); got.Character == "" {
		t.Error("with the soft hyphen still on it, Uyghur asked for no tatweel")
	}
}

// And the value that changes nothing, which is almost every document.
func TestAPlainOrthographyAsksForNothing(t *testing.T) {
	if got := OrthographyPlain.HyphenateBetween("Un", "broken"); got.Any() {
		t.Errorf("English asked for %+v, want nothing", got)
	}
	if (Hyphenation{}).Any() {
		t.Error("an empty Hyphenation reports that it asks for something")
	}
}
