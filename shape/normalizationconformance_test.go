package shape

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Unicode's own normalisation conformance suite, run against
// ComposeCanonically.
//
// NormalizationTest.txt gives five spellings of the same text per line — the
// source and its NFC, NFD, NFKC and NFKD — and states the invariants an
// implementation has to satisfy. The two this file is about are the NFC ones:
//
//	c2 == toNFC(c1) == toNFC(c2) == toNFC(c3)
//	c4 == toNFC(c4) == toNFC(c5)
//
// Both halves matter. The first says the source, its composed form and its
// decomposed form all compose to the same thing, which is what "canonically
// equivalent" means and what the hyphenation dictionary is relying on. The
// second says the compatibility forms are left where they are: NFC is not NFKC,
// and a normaliser that folded a ligature or a superscript would fail it.
//
// Fetched rather than vendored, like every other upstream data file here:
// `make normalization-tests`, and `make test-normalization` to run this.

// normalizationCase is one line of the suite.
type normalizationCase struct {
	line          int
	c1, c2, c3    []rune
	c4, c5        []rune
	part, comment string
}

// normalizationSuite reads the file, or skips the test where it is not there.
func normalizationSuite(t *testing.T) []normalizationCase {
	t.Helper()
	dir := os.Getenv("UNICODE_NORMALIZATION_TESTS")
	if dir == "" {
		t.Skip("set UNICODE_NORMALIZATION_TESTS to the directory holding " +
			"NormalizationTest.txt; `make test-normalization` does it")
	}
	name := filepath.Join(dir, "NormalizationTest.txt")
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer f.Close()

	var out []normalizationCase
	part := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.HasPrefix(line, "@") {
			part = strings.TrimSpace(line)
			continue
		}
		body, comment, _ := strings.Cut(line, "#")
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		fields := strings.Split(body, ";")
		if len(fields) < 5 {
			t.Fatalf("%s:%d: %d columns, want five: %q", name, n, len(fields), line)
		}
		c := normalizationCase{line: n, part: part, comment: strings.TrimSpace(comment)}
		for i, dst := range []*[]rune{&c.c1, &c.c2, &c.c3, &c.c4, &c.c5} {
			*dst = codePoints(t, name, n, fields[i])
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("%v", err)
	}
	// The file is twenty thousand lines and every release adds to it. A parse
	// that quietly matched nothing is the failure mode this guards: a sweep over
	// no cases passes.
	if len(out) < 10000 {
		t.Fatalf("%s holds %d cases; the suite is far larger than that, so this "+
			"read it wrongly", name, len(out))
	}
	return out
}

// codePoints reads one column: space-separated hexadecimal scalar values.
func codePoints(t *testing.T, name string, line int, field string) []rune {
	t.Helper()
	var out []rune
	for _, f := range strings.Fields(field) {
		v, err := strconv.ParseUint(f, 16, 32)
		if err != nil {
			t.Fatalf("%s:%d: %q: %v", name, line, f, err)
		}
		out = append(out, rune(v))
	}
	return out
}

// TestNFCMatchesUnicodesOwnSuite is the conformance sweep.
func TestNFCMatchesUnicodesOwnSuite(t *testing.T) {
	cases := normalizationSuite(t)
	readmeSaysCases(t, len(cases))
	if bad := sweepNFC(t, cases, func(r []rune) []rune {
		got, _ := ComposeCanonically(r)
		return got
	}); bad != 0 {
		t.Errorf("%d of %d cases came out wrong", bad, len(cases))
	}
}

// sweepNFC checks both invariants over every case and returns how many failed,
// reporting the first few.
func sweepNFC(t *testing.T, cases []normalizationCase, nfc func([]rune) []rune) int {
	t.Helper()
	bad := 0
	for _, c := range cases {
		for _, in := range [][]rune{c.c1, c.c2, c.c3} {
			if got := nfc(in); string(got) != string(c.c2) {
				bad++
				if bad <= 8 {
					t.Logf("line %d %s: NFC(%s) is %s, want %s  # %s",
						c.line, c.part, hexOf(in), hexOf(got), hexOf(c.c2), c.comment)
				}
			}
		}
		for _, in := range [][]rune{c.c4, c.c5} {
			if got := nfc(in); string(got) != string(c.c4) {
				bad++
				if bad <= 8 {
					t.Logf("line %d %s: NFC(%s) is %s, want %s  # %s",
						c.line, c.part, hexOf(in), hexOf(got), hexOf(c.c4), c.comment)
				}
			}
		}
	}
	return bad
}

// TestTheNormalizationSuiteHasTeeth is the check on the check.
//
// A sweep that compares nothing, or that was handed no cases, passes in silence.
// So the same sweep is run against a normaliser that returns its input
// unchanged, and it has to reject it: the suite holds thousands of lines where
// the source is not already in NFC, and an implementation that did nothing at
// all would have to fail every one of them.
func TestTheNormalizationSuiteHasTeeth(t *testing.T) {
	cases := normalizationSuite(t)
	quiet := &testing.T{}
	if bad := sweepNFC(quiet, cases, func(r []rune) []rune { return r }); bad < 1000 {
		t.Errorf("a normaliser that does nothing failed only %d of %d cases; the "+
			"sweep is not comparing what it should be", bad, len(cases))
	}
}

// readmeSaysCases keeps the number in the README the number that ran.
//
// A count in prose is a claim, and a claim nothing checks goes stale at the next
// Unicode release — the suite grows every year. This is the same arrangement the
// reftest ratchet is under, and for the same reason.
func readmeSaysCases(t *testing.T, n int) {
	t.Helper()
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := "all " + withThousands(n) + " cases of `NormalizationTest.txt`"
	if !strings.Contains(string(readme), want) {
		t.Errorf("the README does not say %q; the suite that ran holds %d cases",
			want, n)
	}
}

// withThousands writes a number the way the README does.
func withThousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// hexOf writes a sequence the way the suite does.
func hexOf(rs []rune) string {
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.FormatUint(uint64(r), 16))
	}
	if b.Len() == 0 {
		return "<empty>"
	}
	return strings.ToUpper(b.String())
}
