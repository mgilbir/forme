package cmd

import (
	"os/exec"
	"strings"
	"testing"
)

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
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make on this machine to ask")
	}
	expand := func(v string) []string {
		t.Helper()
		out, err := exec.Command("make", "-C", "..", "--no-print-directory", "-s",
			"--eval", "print-var: ; @echo $("+v+")", "print-var").Output()
		if err != nil {
			t.Fatalf("make could not expand %s: %v", v, err)
		}
		return strings.Fields(string(out))
	}
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
