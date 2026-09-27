package paragraph

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A grapheme cluster written across a box boundary is one cluster, by every rule
// of UAX #29.
//
// The line breaker takes an opportunity only at a cluster boundary — CSS Text:
// "CSS never allows soft wrap opportunities within typographic character units"
// — and a box's scan of its own text used to begin afresh, answering the
// boundary at its first character from the character before by the rules one
// character can answer (GB3 to GB9b). GB9c's conjuncts, GB11's emoji sequences
// and GB12 and GB13's regional indicator pairs look further back, and a box
// boundary inside one of them was a place a line could be cut: a conjunct
// broken at its virama, a family emoji at a joiner, two flags read as "🇷 🇺🇸"
// when the first was written in a span of its own.
//
// Unicode's GraphemeBreakTest.txt is the oracle, as it is for package segment.
// Each case is cut at every position into two boxes, as layout hands them over,
// and laid out under line-break: anywhere — which offers an opportunity at every
// cluster boundary and nowhere else — so the opportunities are the clusters.

// graphemeSuiteForBoxes finds GraphemeBreakTest.txt the way package segment's
// test does: UNICODE_GRAPHEME_TESTS, or the checkout's testdata. Unset and absent
// is a skip; set and wrong is a failure, because somebody meant to run this.
func graphemeSuiteForBoxes(t *testing.T) string {
	t.Helper()
	env := os.Getenv("UNICODE_GRAPHEME_TESTS")
	dir := env
	if env == "" {
		dir = filepath.Join("..", "testdata", "unicode-grapheme")
	}
	path := filepath.Join(dir, "GraphemeBreakTest.txt")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if env == "" {
		t.Skip("GraphemeBreakTest.txt is not here; `make grapheme-tests` fetches it")
	}
	t.Fatalf("UNICODE_GRAPHEME_TESTS is %q and there is no GraphemeBreakTest.txt there", env)
	return ""
}

// graphemeCase is one line of the file: the text, and whether a cluster
// boundary falls before each of its characters after the first.
type graphemeCase struct {
	text []rune
	want []bool
}

func readGraphemeCases(t *testing.T) []graphemeCase {
	t.Helper()
	f, err := os.Open(graphemeSuiteForBoxes(t))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []graphemeCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		if strings.TrimSpace(line) == "" {
			continue
		}
		var c graphemeCase
		for _, tok := range strings.Fields(line) {
			switch tok {
			case "÷":
				c.want = append(c.want, true)
			case "×":
				c.want = append(c.want, false)
			default:
				v, err := strconv.ParseUint(tok, 16, 32)
				if err != nil {
					t.Fatalf("%q: %v", line, err)
				}
				c.text = append(c.text, rune(v))
			}
		}
		if len(c.want) != len(c.text)+1 {
			t.Fatalf("cannot read %q", line)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	// The same floor package segment holds the file to: a truncated suite
	// passes everything and proves nothing.
	if len(cases) < 700 {
		t.Fatalf("GraphemeBreakTest.txt has %d cases; it has more than 700", len(cases))
	}
	return cases
}

// TestAClusterContinuesAcrossABoxEdgeByEveryRule.
func TestAClusterContinuesAcrossABoxEdgeByEveryRule(t *testing.T) {
	cases := readGraphemeCases(t)
	ws := WhiteSpace{PreserveBreaks: true, Wrap: true}
	anywhere := LineBreak{Anywhere: true}
	wrong, checked := 0, 0
	for _, c := range cases {
		for cut := 1; cut < len(c.text); cut++ {
			// A carriage return in one box and the line feed after it in the
			// next are two segment breaks; see TestABoxBoundaryIsNotABoundaryToTheRules.
			if c.text[cut-1] == '\r' && c.text[cut] == '\n' {
				continue
			}
			a, b := string(c.text[:cut]), string(c.text[cut:])
			pa, tail := SplitAtBreaksAfter(a, ws, WordBreak{}, anywhere, Hyphens{},
				WritingSystemOther, Carried{})
			base, _ := LastAutospaceBase(a)
			pb, _ := SplitAtBreaksAfter(b, ws, WordBreak{}, anywhere, Hyphens{},
				WritingSystemOther, Carried{
					Context: tail.Context, Clusters: tail.Clusters, Offered: tail.Offered,
					Deferred: tail.Deferred, Taken: tail.Taken, Prev: c.text[cut-1],
					PrevBase: base,
				})
			got := piecesToBreaks(c.text, append(pa[:len(pa):len(pa)], pb...))
			checked++
			for i := 1; i < len(c.text); i++ {
				want := c.want[i]
				// The one tailoring of the cluster CSS lets line breaking make:
				// a mark or a joiner after a space is a unit of its own (UAX
				// #14's LB10), so the line may end at the space. See
				// TestAMarkAfterASpaceBeginsItsOwnUnit.
				if c.text[i-1] == ' ' {
					want = true
				}
				if got[i] != want {
					if wrong++; wrong <= 20 {
						t.Errorf("%+q cut at %d: at %d a line may break = %v, and UAX #29 "+
							"says a cluster boundary = %v", string(c.text), cut, i, got[i], want)
					}
				}
			}
		}
	}
	if wrong > 20 {
		t.Errorf("and %d more", wrong-20)
	}
	t.Logf("%d cases cut %d ways", len(cases), checked)
}

// TestTheScanHandedOnHasReadEveryCharacter. A carriage return and the line feed
// after it are read together, and the scan handed on has to have read both: a
// scan that stopped at the carriage return would take a line feed at the next
// box's start as the second half of the pair (GB3), and put no cluster boundary
// in front of it.
func TestTheScanHandedOnHasReadEveryCharacter(t *testing.T) {
	for _, text := range []string{"a\r\n", "a   "} {
		_, tail := SplitAtBreaks(text, WhiteSpace{PreserveBreaks: true, Wrap: true},
			WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		if !tail.Clusters.Boundary('\n') {
			t.Errorf("%+q: the scan handed on continues a cluster with a line feed", text)
		}
	}
}
