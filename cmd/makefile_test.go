package cmd

import (
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func needMake(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make on this machine to ask")
	}
}

// makeValue asks make what a variable expands to, with any variables given as
// "NAME=value" overridden on its command line as a caller would override them.
func makeValue(t *testing.T, v string, overrides ...string) string {
	t.Helper()
	args := append([]string{"-C", "..", "--no-print-directory", "-s",
		"--eval", "print-var: ; @echo $(" + v + ")", "print-var"}, overrides...)
	out, err := exec.Command("make", args...).Output()
	if err != nil {
		t.Fatalf("make could not expand %s: %v", v, err)
	}
	return strings.TrimSpace(string(out))
}

// TestTheCorpusTargetsFetchEveryCorpus asks make what test-corpora and race
// depend on, and requires every corpus CORPORA names to be among it.
//
// make expands a rule's prerequisites when it reads the rule, so a rule written
// above the variables it names depends on nothing they will later hold. That is
// how test-corpora and race came to run with TABLE_INPUTS=required having
// fetched none of the table inputs: CORPORA named TABLE_SOURCES and
// HTML_ENTITIES, both defined further down the file, and CI failed every table
// whose input is fetched with "not here". Reading the Makefile's text cannot
// see that — the rule said $(CORPORA) either way — so this asks make, which is
// the only thing whose answer matters.
func TestTheCorpusTargetsFetchEveryCorpus(t *testing.T) {
	needMake(t)
	expand := func(v string) []string { return strings.Fields(makeValue(t, v)) }
	corpora := expand("CORPORA")
	for _, v := range []string{"TABLE_SOURCES", "HTML_ENTITIES"} {
		if len(expand(v)) == 0 {
			t.Fatalf("%s expands to nothing, so this test would pass whatever the rules said", v)
		}
	}

	// -p prints make's database with every prerequisite expanded, and -q runs
	// nothing: it answers whether the target is up to date, which it is not, so
	// the exit status is ignored and only the database is read.
	out, _ := exec.Command("make", "-C", "..", "--no-print-directory", "-p", "-q",
		"test-corpora").Output()
	deps := map[string]map[string]bool{"test-corpora": {}, "race": {}}
	for _, line := range strings.Split(string(out), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if _, want := deps[name]; !ok || !want || strings.HasPrefix(rest, "=") {
			continue
		}
		for _, d := range strings.Fields(rest) {
			deps[name][d] = true
		}
	}
	for target, have := range deps {
		if len(have) == 0 {
			t.Fatalf("make's database has no rule for %s", target)
		}
		for _, c := range corpora {
			if !have[c] {
				t.Errorf("%s does not depend on %s, which CORPORA names: it runs without "+
					"fetching it (make expands a rule's prerequisites where the rule is "+
					"read, so the rule must come after every variable CORPORA uses)", target, c)
			}
		}
	}
}

// TestEveryFetchStampMovesWithWhatItFetches is the fault that left
// ScriptExtensions.txt unfetched. A fetched set is marked done by a stamp file,
// and make fetches the set only when the stamp is missing, so a stamp has to be
// named for everything the set is fetched from. The Unicode database's was
// ".ok" alone: a file added to UCD_FILES was never fetched in a checkout that
// already had one, and the reftest corpus's, the conformance sets' and the CSS
// parsing tests' were the same, whatever their pin or list became.
//
// So each stamp is asked for twice, once as the Makefile has it and once with
// one of its variables given another entry on make's command line, and the two
// must differ. A stamp that does not move is a set make will call current when
// it is not.
//
// And every stamp in the Makefile has to be in the list below, which is what
// keeps a new fetch from being written the old way: a rule whose target is a
// ".ok" file that is not one of these stamps fails here by name.
func TestEveryFetchStampMovesWithWhatItFetches(t *testing.T) {
	needMake(t)
	keyedOn := map[string][]string{
		"UCD_STAMP":           {"UNICODE_VERSION", "UCD_FILES"},
		"BIDI_STAMP":          {"UNICODE_VERSION", "BIDI_FILES"},
		"GRAPHEME_STAMP":      {"UNICODE_VERSION"},
		"NORMALIZATION_STAMP": {"UNICODE_VERSION"},
		"DICT_STAMP":          {"ICU_COMMIT", "ICU_DICT_FILES"},
		"BUDOUX_STAMP":        {"BUDOUX_COMMIT", "BUDOUX_FILES"},
		"HYPHEN_STAMP":        {"TEX_HYPHEN_COMMIT", "HYPHEN_FILES"},
		"AFM_STAMP":           {"MATPLOTLIB_COMMIT", "AFM_FILES"},
		"BROTLI_STAMP":        {"BROTLI_COMMIT", "BROTLI_FILES"},
		"AGL_STAMP":           {"AGL_COMMIT"},
		"MSUSE_STAMP":         {"HARFBUZZ_VERSION", "MSUSE_FILES"},
		"CSS_TESTS_STAMP":     {"CSS_TESTS_COMMIT"},
		"CSS_COLOR_STAMP":     {"CSSWG_COMMIT"},
		"NOTO_STAMP": {"NOTO_COMMIT", "NOTO_HINTED", "NOTO_CJK_COMMIT", "NOTO_PATHS",
			"NOTO_LICENSE", "UNIFONT_VER", "UNIFONT_SHA256", "UNIFONT_UPPER_SHA256",
			"IPAFONT_URL", "IPAFONT_SHA256"},
		"CJK_STAMP": {"NOTO_CJK_COMMIT", "CJK_FACES"},
		"WPT_STAMP": {"WPT_COMMIT", "WPT_DIRS"},
	}
	stamps := map[string]string{}
	for stamp, vars := range keyedOn {
		base := makeValue(t, stamp)
		if !strings.Contains(base, "/.ok-") {
			t.Errorf("%s is %q, which is not a stamp", stamp, base)
			continue
		}
		stamps[base] = stamp
		for _, v := range vars {
			val := makeValue(t, v)
			if val == "" {
				t.Errorf("%s expands to nothing, so %s cannot be keyed on it", v, stamp)
				continue
			}
			if moved := makeValue(t, stamp, v+"="+val+" one-more"); moved == base {
				t.Errorf("%s is %s whatever %s holds: a checkout with the stamp "+
					"fetches nothing when %s gains an entry", stamp, base, v, v)
			}
		}
	}

	// Every stamp is a rule, and every rule that makes a ".ok" file is a stamp
	// the list above names.
	out, _ := exec.Command("make", "-C", "..", "--no-print-directory", "-p", "-q",
		"test-corpora").Output()
	var rules []string
	for _, line := range strings.Split(string(out), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(rest, "=") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "\t") || !strings.Contains(name, "/.ok") {
			continue
		}
		rules = append(rules, name)
		if _, known := stamps[name]; !known {
			t.Errorf("the Makefile has a rule for %s, which is not one of the stamps "+
				"this test knows: name it with $(call stamp,...) and add it here", name)
		}
	}
	for path, stamp := range stamps {
		if !slices.Contains(rules, path) {
			t.Errorf("%s is %s, and no rule makes it", stamp, path)
		}
	}
	if len(rules) < len(keyedOn) {
		t.Fatalf("make's database has %d stamp rules for %d stamps, so the database "+
			"was not read", len(rules), len(keyedOn))
	}
}

// TestNoFetchNamesABranch is the fault that made the reftest baseline a
// function of the day the fonts were fetched. The Noto faces came from
// notofonts.github.io's main branch, which a bot rebuilds most nights, and the
// Google Fonts and CJK libraries came from their main branches too, so two
// checkouts made a fortnight apart shaped the same document with different
// fonts and the sweeps counted differences over different families. Only a CI
// cache was holding any of it still.
//
// A branch name is a pin that moves. So make's database is read — every
// variable as it expands and every recipe as written — and no line of it may
// fetch from one: no /main/ or /master/ in a URL, no "latest", and no git fetch
// or checkout of a branch. A pin is a commit, a release or a digest.
func TestNoFetchNamesABranch(t *testing.T) {
	needMake(t)
	out, _ := exec.Command("make", "-C", "..", "--no-print-directory", "-p", "-q",
		"test-corpora").Output()
	moving := regexp.MustCompile(`/(main|master|latest)(/|$|\s)|origin[ /](main|master)\b|-B (main|master)\b`)
	lines := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "https://") || strings.Contains(line, "git -C") {
			lines++
		}
		if m := moving.FindString(line); m != "" {
			t.Errorf("the Makefile fetches from a branch (%q), which moves: pin it to a "+
				"commit and hold each file to its SHA-256\n\t%s", m, strings.TrimSpace(line))
		}
	}
	if lines < 20 {
		t.Fatalf("make's database has %d lines that fetch anything, so it was not read", lines)
	}
}
