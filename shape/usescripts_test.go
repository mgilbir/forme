package shape

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestTheEngineSetsHarfBuzzsScripts holds universalScripts to the scripts
// HarfBuzz's categorize sends to the universal engine, as its own source says,
// recorded by testdata/harfbuzz/usescripts.py.
//
// The list was typed out once and drifted: New Tai Lue was in it, which
// HarfBuzz sets with the default model — so a tone mark written first was
// drawn against a dotted circle, 51 strings of the Google Fonts sweep — and
// the eleven scripts of Unicode 16 and 17 HarfBuzz added were not.
//
// A script the file names and the scripts table does not know is one from a
// later Unicode than the table's; no character is of it, so nothing can reach
// the engine through it, and it is logged rather than required.
func TestTheEngineSetsHarfBuzzsScripts(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "usescripts.expected.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()

	// The OpenType tag for an ISO 15924 code is the code in lower case for
	// every script here but N'Ko, whose tag is 'nko ' — which is how the
	// package's own table spells it, and so how it is compared.
	known := map[string]bool{}
	for _, tags := range scriptOpenTypeTags {
		for _, tag := range tags {
			known[tag] = true
		}
	}
	theirs := map[string]bool{}
	var later []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tag := strings.ToLower(line)
		if tag == "nkoo" {
			tag = "nko "
		}
		if !known[tag] {
			later = append(later, line)
			continue
		}
		theirs[tag] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(theirs) < 80 {
		t.Fatalf("only %d scripts read from %s; it has stopped being read", len(theirs), path)
	}
	for tag := range theirs {
		if !universalScripts[tag] {
			t.Errorf("HarfBuzz sets %q with the universal engine and this package does not", tag)
		}
	}
	for tag := range universalScripts {
		if !theirs[tag] {
			t.Errorf("this package sets %q with the universal engine and HarfBuzz does not", tag)
		}
	}
	slices.Sort(later)
	t.Logf("%d scripts agree; %d are of a Unicode later than the scripts table's: %v",
		len(theirs), len(later), later)
}
