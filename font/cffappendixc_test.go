package font

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The predefined charsets against Adobe's own specification.
//
// The tables in cffcharsets.go came from a transcription of TN #5176 Appendix C,
// and everything else in cffcharsets_test.go checks them for self-consistency:
// that the names resolve, that the subset is a subset, that the lengths are the
// ones the specification gives. None of that can catch the two tables being
// consistently wrong, which is what a transcription error produces.
//
// So the appendix itself is checked in — testdata/cff-appendix-c.txt, one
// "<charset> <SID> <name>" per line in GID order — and this compares against it.
//
// # The control
//
// The file carries ISOAdobe as well, which the parser does not need: it is the
// identity, and that is knowable without reading the table. So a transcription
// that mangled the appendix is caught by a fact the file does not carry, and the
// two charsets that *are* load-bearing are read from the same lines by the same
// code. It has already earned its place — the first extraction read the PDF's
// three columns row-wise, and ISOAdobe came back 1, 21, 41, 2, 22, 42.

type charsetEntry struct {
	sid  int
	name string
}

// readAppendixC returns the three charsets in GID order, starting at GID 1. The
// specification does not list GID 0, which is .notdef in every charset.
func readAppendixC(t *testing.T) map[string][]charsetEntry {
	t.Helper()
	f, err := os.Open("testdata/cff-appendix-c.txt")
	if err != nil {
		t.Fatalf("the appendix this checks against is missing: %v", err)
	}
	defer f.Close()

	out := map[string][]charsetEntry{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			t.Fatalf("cannot read %q as <charset> <SID> <name>", line)
		}
		sid, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatalf("cannot read the SID in %q: %v", line, err)
		}
		out[parts[0]] = append(out[parts[0]], charsetEntry{sid, parts[2]})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading the appendix: %v", err)
	}
	return out
}

// TestTheAppendixTranscriptionIsSound is the control, and it runs first because
// nothing below it means anything if this fails.
//
// ISOAdobe is the identity: GID n is SID n. That is true of the charset itself
// and is nowhere stated in the lines this reads, so a file whose entries came out
// in the wrong order, or short, or doubled, cannot satisfy it.
func TestTheAppendixTranscriptionIsSound(t *testing.T) {
	iso := readAppendixC(t)["ISOAdobe"]
	if got, want := len(iso), isoAdobeCharsetLen-1; got != want {
		t.Fatalf("ISOAdobe has %d entries, want %d — the appendix lists GID 1 up, "+
			"and GID 0 is the .notdef it does not list", got, want)
	}
	for i, e := range iso {
		if gid := i + 1; e.sid != gid {
			t.Fatalf("ISOAdobe GID %d has SID %d; the charset is the identity, so "+
				"the transcription is not in GID order", gid, e.sid)
		}
		if want := cffStandardStrings[e.sid]; e.name != want {
			t.Errorf("ISOAdobe GID %d: the appendix names SID %d %q, this module "+
				"names it %q", i+1, e.sid, e.name, want)
		}
	}
}

// TestThePredefinedCharsetsMatchTheSpecification compares both tables against
// Appendix C entry by entry: the name the table holds, and the SID this module
// resolves it to.
//
// The second half of that is a check on cffStandardStrings as much as on the
// charsets — the appendix states the SID for every one of these names, and a
// standard-strings table off by an entry anywhere in the expert range would put
// every glyph after it under the wrong name.
func TestThePredefinedCharsetsMatchTheSpecification(t *testing.T) {
	appendix := readAppendixC(t)
	for _, tc := range []struct {
		what  string
		key   string
		id    int
		names []string
	}{
		{"Expert", "Expert", 1, cffExpertCharsetNames},
		{"Expert Subset", "ExpertSubset", 2, cffExpertSubsetCharsetNames},
	} {
		want := appendix[tc.key]
		if len(want) == 0 {
			t.Fatalf("%s: the appendix carries no entries for it", tc.what)
		}
		sids, ok := cffPredefinedCharset(tc.id)
		if !ok {
			t.Fatalf("%s: charset id %d is not predefined", tc.what, tc.id)
		}
		// GID 0 is .notdef, which the appendix does not list.
		if tc.names[0] != ".notdef" || sids[0] != 0 {
			t.Errorf("%s: GID 0 is %q / SID %d, want .notdef / 0",
				tc.what, tc.names[0], sids[0])
		}
		if got, want := len(tc.names), len(want)+1; got != want {
			t.Errorf("%s: the table has %d entries and the appendix %d including "+
				"the implicit .notdef", tc.what, got, want)
		}
		for i, e := range want {
			gid := i + 1
			if gid >= len(tc.names) {
				break
			}
			if tc.names[gid] != e.name {
				t.Errorf("%s GID %d: the table names it %q, the specification %q",
					tc.what, gid, tc.names[gid], e.name)
			}
			if sids[gid] != e.sid {
				t.Errorf("%s GID %d (%q): this module resolves SID %d, the "+
					"specification states %d", tc.what, gid, e.name, sids[gid], e.sid)
			}
		}
	}
}
