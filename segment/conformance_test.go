package segment

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// Unicode's own conformance suite for UAX #29's grapheme rules.
//
// GraphemeBreakTest.txt states, for every position in several hundred crafted
// strings, whether a boundary falls there — and it is built to exercise each
// rule against each class rather than to look like text, so it reaches the
// combinations no document in this repository contains. It is the oracle for
// this package in the way BidiCharacterTest.txt is the oracle for internal/bidi,
// and for the same reason: the answer comes from the Consortium rather than from
// this repository's reading of the specification.
//
// Fetched rather than committed, as the bidi suites are — `make grapheme-tests`.

const graphemeEnv = "UNICODE_GRAPHEME_TESTS"

// The number of cases the file is expected to hold. A suite that silently
// shrank — a truncated download, a parser that stopped early — would pass every
// assertion and prove nothing, which is the failure mode a conformance test is
// most exposed to.
const minGraphemeCases = 700

// graphemeSuite finds GraphemeBreakTest.txt, and distinguishes "nobody fetched
// it" from "somebody pointed at the wrong place".
//
// The difference matters because a skip is indistinguishable from a pass in the
// output: a mistyped path turns the oracle off silently and the run still looks
// green. Unset and absent is a skip, because a developer without the file
// should still be able to run the suite; set and wrong is a failure, because
// somebody meant to run this and did not. bidi's testDir has said so since it
// was written, and this said the opposite — it skipped for a wrong path, and it
// skipped for an unset one even with the file sitting in the checkout where
// `make grapheme-tests` puts it.
func graphemeSuite(t *testing.T) string {
	t.Helper()
	const marker = "GraphemeBreakTest.txt"

	env := os.Getenv(graphemeEnv)
	dir := env
	if env == "" {
		dir = filepath.Join("..", "testdata", "unicode-grapheme")
	}
	path := filepath.Join(dir, marker)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if env == "" {
		t.Skipf("Unicode grapheme conformance data not present; run `make grapheme-tests`")
	}
	t.Fatalf("%s is set to %q, and there is no %s there.\n"+
		"Failing rather than skipping: this is the oracle for UAX #29, and a skip\n"+
		"would report success having checked nothing.", graphemeEnv, env, marker)
	return ""
}

// runGraphemeSuite reads every case of the file and reports how many there were
// and which of them the given implementation got wrong.
//
// It takes the implementation rather than calling Boundaries, because the only
// way to know the sweep would catch a mistake is to hand it one. See
// TestTheConformanceSuiteHasTeeth.
func runGraphemeSuite(t *testing.T, path string,
	boundaries func(dst []int, s string) []int) (cases int, wrong []string) {

	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		if strings.TrimSpace(text) == "" || strings.Contains(text, "@Part") {
			continue
		}
		s, wantBreaks, err := parseCase(text)
		if err != nil {
			t.Fatalf("%s:%d: %v", path, line, err)
		}
		cases++
		if got := boundaries(nil, s); !equal(got, wantBreaks) {
			wrong = append(wrong, fmt.Sprintf("%s:%d: %s\n  boundaries %v, want %v",
				path, line, describe(s), got, wantBreaks))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if cases < minGraphemeCases {
		t.Fatalf("only %d conformance cases in %s, expected at least %d — the "+
			"suite is truncated or the parser is stopping early, and a shrunken "+
			"suite passes without proving anything", cases, path, minGraphemeCases)
	}
	return cases, wrong
}

func TestGraphemeConformance(t *testing.T) {
	path := graphemeSuite(t)
	cases, wrong := runGraphemeSuite(t, path, Boundaries)
	for i, w := range wrong {
		if i >= 10 {
			t.Errorf("... and %d more", len(wrong)-10)
			break
		}
		t.Error(w)
	}
	t.Logf("%d conformance cases, %d wrong", cases, len(wrong))
}

// parseCase reads one line of the suite: a chain of code points separated by
// U+00F7 DIVISION SIGN where a boundary falls and U+00D7 MULTIPLICATION SIGN
// where one does not, with a division sign at each end.
//
// It returns the string and the byte offsets *inside* it at which a cluster
// begins, which is what Boundaries returns — the leading and trailing marks are
// dropped rather than compared, because they are the two positions that are not
// choices.
func parseCase(line string) (string, []int, error) {
	var b strings.Builder
	var want []int
	for _, tok := range strings.Fields(line) {
		switch tok {
		case "÷":
			if b.Len() > 0 {
				want = append(want, b.Len())
			}
		case "×":
			// no boundary here
		default:
			n, err := strconv.ParseUint(tok, 16, 32)
			if err != nil {
				return "", nil, err
			}
			b.WriteRune(rune(n))
		}
	}
	s := b.String()
	// The final division sign is the end of the string, which is not a choice.
	if n := len(want); n > 0 && want[n-1] == len(s) {
		want = want[:n-1]
	}
	return s, want, nil
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func describe(s string) string {
	var parts []string
	for _, r := range s {
		parts = append(parts, strconv.FormatInt(int64(r), 16))
	}
	return strings.Join(parts, " ")
}

// graphemeRules is UAX #29's break rules, one predicate each, in the order
// grapheme.go's decide applies them.
//
// A second statement of that switch, and it is here on purpose: taking a rule
// away is the only way to see the conformance sweep fail, and a knob in the
// engine for a test to turn would be a branch on every character of every
// document. The cost of the copy is that it can go stale, and that is checked
// rather than hoped for — TestTheConformanceSuiteHasTeeth first requires the
// copy with every rule in place to agree with the engine on all several
// thousand cases of the file.
//
// GB1 and GB999 are not here. They are the two defaults — a boundary at the
// start of the text, and a boundary anywhere no rule spoke — and there is
// nothing to remove.
var graphemeRules = []struct {
	name    string
	applies func(s *scanner, p, c props) bool
	brk     bool
}{
	{"GB3", func(_ *scanner, p, c props) bool { return p.br == CR && c.br == LF }, false},
	{"GB4", func(_ *scanner, p, _ props) bool {
		return p.br == Control || p.br == CR || p.br == LF
	}, true},
	{"GB5", func(_ *scanner, _, c props) bool {
		return c.br == Control || c.br == CR || c.br == LF
	}, true},
	{"GB6", func(_ *scanner, p, c props) bool {
		return p.br == HangulL &&
			(c.br == HangulL || c.br == HangulV || c.br == HangulLV || c.br == HangulLVT)
	}, false},
	{"GB7", func(_ *scanner, p, c props) bool {
		return (p.br == HangulLV || p.br == HangulV) && (c.br == HangulV || c.br == HangulT)
	}, false},
	{"GB8", func(_ *scanner, p, c props) bool {
		return (p.br == HangulLVT || p.br == HangulT) && c.br == HangulT
	}, false},
	{"GB9", func(_ *scanner, _, c props) bool { return c.br == Extend || c.br == ZWJ }, false},
	{"GB9a", func(_ *scanner, _, c props) bool { return c.br == SpacingMark }, false},
	{"GB9b", func(_ *scanner, p, _ props) bool { return p.br == Prepend }, false},
	{"GB9c", func(s *scanner, _, c props) bool {
		return s.conj == conjLinked && c.cj == conjunctConsonant
	}, false},
	{"GB11", func(s *scanner, _, c props) bool { return s.pict == pictJoined && c.pict }, false},
	{"GB12/GB13", func(s *scanner, p, c props) bool {
		return p.br == RegionalIndicator && c.br == RegionalIndicator && s.ri%2 == 1
	}, false},
}

// boundariesWithout is Boundaries decided by the rules above, with one of them
// taken away. An empty name takes none away, which is the copy against the
// engine.
func boundariesWithout(dst []int, str, skip string) []int {
	var sc scanner
	for i, r := range str {
		if r == utf8.RuneError {
			if _, n := utf8.DecodeRuneInString(str[i:]); n == 1 {
				if i > 0 {
					dst = append(dst, i)
				}
				sc = scanner{}
				continue
			}
		}
		cur := propsOf(r)
		brk := true // GB1 and GB999
		if sc.set {
			for _, rule := range graphemeRules {
				if rule.name == skip || !rule.applies(&sc, sc.prev, cur) {
					continue
				}
				brk = rule.brk
				break
			}
		}
		sc.advance(cur)
		if brk && i > 0 {
			dst = append(dst, i)
		}
	}
	return dst
}

// TestTheConformanceSuiteHasTeeth takes each rule away in turn and requires the
// sweep above to notice.
//
// A conformance run that has never been seen to fail proves nothing: a parser
// that read no cases, or a comparison that always agreed, would be green and
// would say so in the same words. This is what makes the run mean something,
// and what it used to be was fourteen assertions against Boundaries — a fine
// unit test, planting nothing, running the sweep not at all, and gated on an
// environment variable it never read a file with.
func TestTheConformanceSuiteHasTeeth(t *testing.T) {
	path := graphemeSuite(t)

	// The rules below have to be the rules the engine applies, or every
	// planting after this is vacuous.
	cases, wrong := runGraphemeSuite(t, path, func(dst []int, s string) []int {
		return boundariesWithout(dst, s, "")
	})
	if len(wrong) > 0 {
		t.Fatalf("the copy of the rules in this file disagrees with the engine on "+
			"%d of %d cases, so taking a rule away from it proves nothing about "+
			"the engine. First:\n%s", len(wrong), cases, wrong[0])
	}

	for _, rule := range graphemeRules {
		_, wrong := runGraphemeSuite(t, path, func(dst []int, s string) []int {
			return boundariesWithout(dst, s, rule.name)
		})
		if len(wrong) == 0 {
			t.Errorf("with %s taken away, the suite agreed on all %d cases — it "+
				"does not test that rule, and a run of it proves nothing about one",
				rule.name, cases)
			continue
		}
		t.Logf("%s: %d of %d cases rejected", rule.name, len(wrong), cases)
	}
}

// TestEachRuleDecidesTheCaseItIsFor is the same rules stated one case each, so
// that a failure names the rule rather than a line number in a downloaded file.
// It needs no suite and runs in every checkout.
func TestEachRuleDecidesTheCaseItIsFor(t *testing.T) {
	for _, c := range []struct {
		rule string
		s    string
		want []int
	}{
		{"GB3 keeps CRLF together", "\r\n", nil},
		{"GB4 cuts after a control", "\na", []int{1}},
		{"GB5 cuts before a control", "a\n", []int{1}},
		{"GB6 joins L to V", "\uAC00", nil},
		{"GB7 joins V to T", "\u1161\u11A8", nil},
		{"GB8 joins LVT to T", "\uAC01\u11A8", nil},
		{"GB9 attaches a combining mark", "a\u0300", nil},
		{"GB9a attaches a spacing mark", "\u0915\u093E", nil},
		{"GB9b attaches to a prepend", "\u0600a", nil},
		{"GB9c holds a conjunct across its virama", "\u0915\u094D\u0915", nil},
		{"GB11 holds an emoji ZWJ sequence", "\U0001F468\u200D\U0001F469", nil},
		{"GB12 pairs two regional indicators", "\U0001F1E6\U0001F1E7", nil},
		{"GB13 starts a new pair at the third", "\U0001F1E6\U0001F1E7\U0001F1E8", []int{8}},
		{"GB999 cuts between two letters", "ab", []int{1}},
	} {
		if got := Boundaries(nil, c.s); !equal(got, c.want) {
			t.Errorf("%s: %s has boundaries %v, want %v",
				c.rule, describe(c.s), got, c.want)
		}
	}
}
