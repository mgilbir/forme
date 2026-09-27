package paragraph

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/segment"
)

// Unicode's own conformance suite for UAX #14, run through this engine.
//
// LineBreakTest.txt states, for every position in 19,338 crafted strings,
// whether a line may break there, and it is built to exercise every rule
// against every class. It is the oracle for the line breaker in the way
// GraphemeBreakTest.txt is for package segment. It is fetched with the rest of
// the database — `make ucd` — and held to the release the tables are.
//
// Two things are run over it. The rules alone, which must agree with every
// case: uax14.go is UAX #14 and nothing else. And SplitAtBreaks, which is what
// layout asks, under each value of line-break: it must agree with every case
// too, except where CSS Text changes the answer, and each change it makes is
// named below with the sentence that makes it. A disagreement no entry
// explains is a failure. So is an entry that explains nothing, because an
// allowance nothing uses is one that would hide the next departure.

// minLineBreakCases is how many cases the file is expected to hold. A suite that
// silently shrank — a truncated fetch, a parser that stopped early — would pass
// every assertion and prove nothing.
const minLineBreakCases = 19000

// lineBreakSuite finds LineBreakTest.txt. Absent is a skip, because a developer
// without the database should still be able to run the package — unless
// TABLE_INPUTS=required, which `make test-corpora` sets having fetched it, and
// where a skip would be a check that did not run.
func lineBreakSuite(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "testdata", "ucd", "auxiliary", "LineBreakTest.txt")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if os.Getenv("TABLE_INPUTS") == "required" {
		t.Fatalf("%s is not here, and TABLE_INPUTS=required says the database was "+
			"fetched; `make ucd` fetches it", path)
	}
	t.Skipf("%s is not in this checkout; `make ucd` fetches it", path)
	return ""
}

// lbCase is one line of the file: the text, and for each position between two
// of its characters whether a line may break there and which rule says so.
type lbCase struct {
	line  string
	text  []rune
	want  []bool   // want[i]: a break before text[i], for 0 < i < len(text)
	rules []string // rules[i]: the rule the file names for that position
}

var lbRuleMark = regexp.MustCompile(`[×÷] \[([0-9.]+)\]`)

func readLineBreakSuite(t *testing.T) []lbCase {
	t.Helper()
	path := lineBreakSuite(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cases []lbCase
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			want := "# LineBreakTest-" + lineBreakUnicodeVersion + ".txt"
			if strings.TrimSpace(line) != want {
				t.Fatalf("%s begins %q, and the tables are Unicode %s: a character "+
					"whose class changed between releases is a stale table, not a "+
					"defect", path, line, lineBreakUnicodeVersion)
			}
		}
		if line == "" || line[0] == '#' {
			continue
		}
		data, comment, _ := strings.Cut(line, "#")
		c := lbCase{line: strings.TrimSpace(data)}
		for _, tok := range strings.Fields(data) {
			switch tok {
			case "×":
				c.want = append(c.want, false)
			case "÷":
				c.want = append(c.want, true)
			default:
				v, err := strconv.ParseUint(tok, 16, 32)
				if err != nil {
					t.Fatalf("%s: %q is not a code point", line, tok)
				}
				c.text = append(c.text, rune(v))
			}
		}
		for _, m := range lbRuleMark.FindAllStringSubmatch(comment, -1) {
			c.rules = append(c.rules, m[1])
		}
		if len(c.want) != len(c.text)+1 || len(c.rules) != len(c.want) {
			t.Fatalf("cannot read %q: %d characters, %d marks, %d rules",
				line, len(c.text), len(c.want), len(c.rules))
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(cases) < minLineBreakCases {
		t.Fatalf("%s has %d cases, and the suite has at least %d: a truncated file "+
			"passes every assertion and proves nothing", path, len(cases), minLineBreakCases)
	}
	return cases
}

// TestTheRulesAreUAX14 runs uax14.go alone over every case, with no tailoring:
// CJ resolved to NS, which is LB1's default and the answer the file gives.
func TestTheRulesAreUAX14(t *testing.T) {
	cases := readLineBreakSuite(t)
	tl := lbTailoring{lb: LineBreak{Strict: true}}
	wrong := 0
	for _, c := range cases {
		var ctx BreakContext
		for i, r := range c.text {
			ch := tl.char(r)
			rest := string(c.text[i+1:])
			brk, rule := ctx.decide(ch, lbAhead{t: tl, rest: rest})
			if i > 0 && (brk != lbProhibited) != c.want[i] {
				if wrong++; wrong <= 20 {
					t.Errorf("%s: at %d the rules say %v by %d, the file %v by [%s]",
						c.line, i, brk != lbProhibited, rule, c.want[i], c.rules[i])
				}
			}
			ctx.advance(ch, Hyphens{})
		}
	}
	if wrong > 20 {
		t.Errorf("and %d more", wrong-20)
	}
	t.Logf("%d cases", len(cases))
}

// breaksOfCase is where SplitAtBreaks lets a line break in one case's text,
// under white-space: pre-wrap so that every character the file writes reaches
// it as written: a position is in the set where a piece begins that may begin a
// line, and after every forced break.
func breaksOfCase(text []rune, lb LineBreak) map[int]bool {
	pieces, _ := SplitAtBreaks(string(text), WhiteSpace{PreserveBreaks: true, Wrap: true},
		WordBreak{}, lb, Hyphens{}, WritingSystemOther)
	return piecesToBreaks(text, pieces)
}

func piecesToBreaks(text []rune, pieces []Piece) map[int]bool {
	out := map[int]bool{}
	at := 0
	for i, p := range pieces {
		n := utf8.RuneCountInString(p.Text)
		// A carriage return and the line feed after it are one break and one
		// piece, written "\n".
		if p.Segment && p.Text == "\n" && at+1 < len(text) && text[at] == '\r' &&
			text[at+1] == '\n' {
			n = 2
		}
		if i > 0 && p.BreakBefore && !p.LastResort {
			out[at] = true
		}
		if p.Segment {
			out[at+n] = true
		}
		at += n
	}
	return out
}

// cssChange is one thing CSS Text says differently from UAX #14: a name, the
// sentence, and whether it accounts for a disagreement at position i of a case.
type cssChange struct {
	name, cite string
	explains   func(c lbCase, i int, lb LineBreak) bool
}

// unitBase is the base of the unit in front of position i: LB9's X in "X CM*",
// found by stepping back over the marks and joiners that belong to it.
func unitBase(text []rune, i int) rune {
	j := i - 1
	for j > 0 {
		switch lineBreakClass(text[j]) {
		case lbCM, lbZWJ:
			switch lineBreakClass(text[j-1]) {
			case lbBK, lbCR, lbLF, lbNL, lbSP, lbZW:
				return text[j]
			}
			j--
			continue
		}
		break
	}
	return text[j]
}

var cssChanges = []cssChange{
	{
		name: "no opportunity inside a typographic character unit",
		cite: `CSS Text 3 §5, Line Breaking Details: "CSS never allows soft wrap ` +
			`opportunities within typographic character units", which are UAX #29's ` +
			`extended grapheme clusters. UAX #14 alone breaks between a space and a ` +
			`combining mark after it (LB10), and before a spacing mark or an emoji ` +
			`modifier after a mark.`,
		explains: func(c lbCase, i int, _ LineBreak) bool {
			s := string(c.text)
			at := len(string(c.text[:i]))
			for _, b := range segment.Boundaries(nil, s) {
				if b == at {
					return false
				}
			}
			return c.want[i]
		},
	},
	{
		name: "the note's deviations from UAX #14",
		cite: `CSS Text 3, the note to line-break: "the following deviations could ` +
			`be desirable for maximum interoperability with existing implementations" ` +
			`— no opportunity between U+0021, U+002F or U+007C and a letter, and one ` +
			`after U+002D before a digit where the hyphen follows a letter or digit.`,
		explains: func(c lbCase, i int, _ LineBreak) bool {
			left := unitBase(c.text, i)
			if c.want[i] {
				return (left == '!' || left == '/' || left == '|') &&
					charprop.Is(c.text[i], charprop.L)
			}
			return c.rules[i] == "25.13" && left == '-' && i >= 2 &&
				charprop.Is(c.text[i-2], charprop.L|charprop.Nd)
		},
	},
	{
		name: "CJ resolved to ID except under strict",
		cite: `UAX #14 LB1 leaves CJ to a tailoring, "resolve CJ to NS" for strict ` +
			`line breaking and to ID for normal; CSS Text's line-break is that ` +
			`tailoring. The file resolves it to NS. (The current draft of CSS Text 3 ` +
			`forbids a break before CJ under normal as well; this engine keeps the ` +
			`resolution its suite's line-break-normal tests assert.)`,
		explains: func(c lbCase, i int, lb LineBreak) bool {
			return !lb.Strict && (lineBreakClass(c.text[i]) == lbCJ ||
				lineBreakClass(unitBase(c.text, i)) == lbCJ)
		},
	},
	{
		name: "loose: a hyphen may begin a line after an ideograph",
		cite: `CSS Text 3, line-break: "The following breaks are allowed for loose ` +
			`line breaking if the preceding character belongs to the Unicode line ` +
			`breaking class ID ... breaks before hyphens: ‐ U+2010, – U+2013".`,
		explains: func(c lbCase, i int, lb LineBreak) bool {
			return lb.Loose && !c.want[i] && lineBreakClass(c.text[i]) == lbHH &&
				lineBreakClass(unitBase(c.text, i)) == lbID
		},
	},
	{
		name: "loose: iteration marks and inseparable characters",
		cite: `CSS Text 3, line-break: "The following breaks are forbidden for normal ` +
			`and strict line breaking and allowed in loose: ... breaks before ` +
			`iteration marks: 々 U+3005, 〻 U+303B, ゝ U+309D, ゞ U+309E, ヽ U+30FD, ヾ ` +
			`U+30FE; breaks between inseparable characters".`,
		explains: func(c lbCase, i int, lb LineBreak) bool {
			if !lb.Loose || c.want[i] {
				return false
			}
			r := c.text[i]
			return isIterationMark(r) || (lineBreakClass(r) == lbIN &&
				lineBreakClass(unitBase(c.text, i)) == lbIN)
		},
	},
	{
		name: "the hyphen-like characters of Chinese and Japanese",
		cite: `CSS Text 3, line-break: "The following breaks are allowed for normal ` +
			`and loose line breaking if the writing system is Chinese or Japanese, ` +
			`and are otherwise forbidden: breaks before certain CJK hyphen-like ` +
			`characters: 〜 U+301C, ゠ U+30A0".`,
		explains: func(c lbCase, i int, lb LineBreak) bool {
			r := c.text[i]
			return (lb.Normal || lb.Loose) && lb.ChineseOrJapanese && !c.want[i] &&
				(r == 0x301C || r == 0x30A0)
		},
	},
	{
		name: "loose, in Chinese and Japanese: centred punctuation, suffixes, prefixes",
		cite: `CSS Text 3, line-break: "The following breaks are allowed for loose if ` +
			`the writing system is Chinese or Japanese and are otherwise forbidden: ` +
			`breaks before certain centered punctuation marks ...; breaks before ` +
			`suffixes: [class PO with East Asian Width] Ambiguous, Fullwidth, or Wide; ` +
			`breaks after prefixes: [class PR, the same]".`,
		explains: func(c lbCase, i int, lb LineBreak) bool {
			if !lb.Loose || !lb.ChineseOrJapanese || c.want[i] {
				return false
			}
			r, l := c.text[i], unitBase(c.text, i)
			wide := func(r rune) bool {
				return inRanges(r, eastAsianAmbiguousRanges[:]) ||
					(inRanges(r, eastAsianWideRanges[:]) && !inRanges(r, eastAsianHalfwidthRanges[:]))
			}
			return isCentredPunctuation(r) ||
				(lineBreakClass(r) == lbPO && wide(r)) ||
				(lineBreakClass(l) == lbPR && wide(l))
		},
	},
}

// TestSplitAtBreaksIsUAX14ButForCSS.
//
// Every value of line-break, in text that is and is not Chinese or Japanese,
// because the value and the writing system are what CSS's changes are keyed
// on. word-break and white-space are held at the values that change nothing:
// break-all, keep-all and anywhere are not tailorings of UAX #14 but
// replacements of parts of it, and each has its own tests.
func TestSplitAtBreaksIsUAX14ButForCSS(t *testing.T) {
	cases := readLineBreakSuite(t)
	used := map[string]int{}
	for _, v := range []struct {
		name string
		lb   LineBreak
	}{
		{"auto", LineBreak{}},
		{"normal", LineBreak{Normal: true}},
		{"strict", LineBreak{Strict: true}},
		{"loose", LineBreak{Loose: true}},
	} {
		for _, cj := range []bool{false, true} {
			lb := v.lb
			lb.ChineseOrJapanese = cj
			name := v.name
			if cj {
				name += ", Chinese or Japanese"
			}
			perRule := map[string]int{}
			wrong := 0
			for _, c := range cases {
				got := breaksOfCase(c.text, lb)
				for i := 1; i < len(c.text); i++ {
					if got[i] == c.want[i] {
						continue
					}
					explained := false
					for _, ch := range cssChanges {
						if ch.explains(c, i, lb) {
							used[ch.name]++
							explained = true
							break
						}
					}
					if explained {
						perRule[c.rules[i]]++
						continue
					}
					if wrong++; wrong <= 20 {
						t.Errorf("%s: %s: at %d this engine says %v and the file %v "+
							"by [%s], and no rule of CSS Text accounts for it",
							name, c.line, i, got[i], c.want[i], c.rules[i])
					}
				}
			}
			if wrong > 20 {
				t.Errorf("%s: and %d more", name, wrong-20)
			}
			var rules []string
			for r := range perRule {
				rules = append(rules, r)
			}
			sort.Strings(rules)
			var b strings.Builder
			for _, r := range rules {
				fmt.Fprintf(&b, " [%s] %d", r, perRule[r])
			}
			t.Logf("%s: departures CSS makes, by the rule the file names:%s", name, b.String())
		}
	}
	for _, ch := range cssChanges {
		if used[ch.name] == 0 {
			t.Errorf("%q explains no disagreement anywhere in the file: an allowance "+
				"nothing needs is one that would hide the next departure", ch.name)
		}
		t.Logf("%s: %d positions", ch.name, used[ch.name])
	}
}

// TestABoxBoundaryIsNotABoundaryToTheRules cuts every case at every position
// into two texts, as two inline boxes would hand them over, and asks for the
// breaks of the whole.
//
// It is what the context and the lookahead are for: "OP SP* ×" across a
// boundary inside the spaces, a number that runs over one, a pair of regional
// indicators written in two spans, "× QU_Pf" with the quotation mark at the end
// of a box and the character that decides it in the next.
func TestABoxBoundaryIsNotABoundaryToTheRules(t *testing.T) {
	cases := readLineBreakSuite(t)
	ws := WhiteSpace{PreserveBreaks: true, Wrap: true}
	wrong := 0
	for _, c := range cases {
		whole := breaksOfCase(c.text, LineBreak{})
		for cut := 1; cut < len(c.text); cut++ {
			// A carriage return in one box and the line feed after it in the
			// next are two segment breaks: CSS reads a CR LF pair as one only
			// inside a text, and HTML's parser has made every one of them a
			// line feed before there are boxes to split it between.
			if c.text[cut-1] == '\r' && c.text[cut] == '\n' {
				continue
			}
			a, b := string(c.text[:cut]), string(c.text[cut:])
			pa, tail := SplitAtBreaksAfter(a, ws, WordBreak{}, LineBreak{}, Hyphens{},
				WritingSystemOther, Carried{Ahead: b})
			base, _ := LastAutospaceBase(a)
			pb, _ := SplitAtBreaksAfter(b, ws, WordBreak{}, LineBreak{}, Hyphens{},
				WritingSystemOther, Carried{
					Context: tail.Context, Clusters: tail.Clusters, Offered: tail.Offered,
					Deferred: tail.Deferred, Held: tail.Held, Taken: tail.Taken,
					Prev: c.text[cut-1], PrevBase: base,
				})
			got := piecesToBreaks(c.text, append(pa[:len(pa):len(pa)], pb...))
			for i := 1; i < len(c.text); i++ {
				if got[i] != whole[i] {
					if wrong++; wrong <= 20 {
						t.Errorf("%s cut at %d: at %d the two boxes say %v and the "+
							"one says %v", c.line, cut, i, got[i], whole[i])
					}
				}
			}
		}
	}
	if wrong > 20 {
		t.Errorf("and %d more", wrong-20)
	}
}
