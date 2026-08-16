package layout

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The breakdown behind the ratchet.
//
// TestWPTReftests answers one question — how many pairs agree with nothing
// unsupported reported — and that number is the thing not to regress. It cannot
// answer the next question, which is what the rest are waiting on, because it
// reduces every finding to a bool before anything can count them.
//
// This runs the same sweep and keeps the findings. It asserts almost nothing:
// its output is a measurement, and a measurement that fails the build when the
// engine improves is a measurement nobody will run twice. What it does assert
// is that its own totals match the ratchet's, because two sweeps of one suite
// disagreeing would make every number below meaningless.
//
// Gated on WPT_BREAKDOWN because it prints some hundreds of lines:
//
//	make wpt noto-fonts
//	WPT_TESTS=$PWD/testdata/wpt NOTO_FONTS=$PWD/testdata/fonts-noto \
//	  WPT_BREAKDOWN=1 go test ./layout/ -run TestWPTBreakdown -count=1 -v

// tally counts tests by key, keeping pass and fail apart, because the two mean
// different things: a taint on a passing test costs a clean pass, and a taint
// on a failing one may or may not be the reason it failed.
type tally struct {
	pass, fail map[string]int
}

func newTally() *tally {
	return &tally{pass: map[string]int{}, fail: map[string]int{}}
}

func (t *tally) add(key string, passed bool) {
	if passed {
		t.pass[key]++
		return
	}
	t.fail[key]++
}

// top returns the keys by total, largest first, at most n of them, and how many
// tests the ones it dropped account for.
func (t *tally) top(n int) (rows []string, dropped, droppedTests int) {
	keys := make([]string, 0, len(t.pass)+len(t.fail))
	seen := map[string]bool{}
	for _, m := range []map[string]int{t.pass, t.fail} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	total := func(k string) int { return t.pass[k] + t.fail[k] }
	sort.Slice(keys, func(i, j int) bool {
		if a, b := total(keys[i]), total(keys[j]); a != b {
			return a > b
		}
		return keys[i] < keys[j]
	})
	for i, k := range keys {
		if i >= n {
			dropped++
			droppedTests += total(k)
			continue
		}
		rows = append(rows, fmt.Sprintf("  %5d  %5d  %5d  %s",
			total(k), t.pass[k], t.fail[k], k))
	}
	return rows, dropped, droppedTests
}

func (t *tally) report(tb testing.TB, title string, n int) {
	tb.Helper()
	rows, dropped, droppedTests := t.top(n)
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n  total   pass   fail  what\n", title)
	for _, r := range rows {
		b.WriteString(r)
		b.WriteByte('\n')
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "  … and %d more accounting for %d tests\n", dropped, droppedTests)
	}
	tb.Log(b.String())
}

// elementOf pulls the element name out of an unsupported-element message, which
// the HTML parser writes as "<name> is …".
func elementOf(msg string) string {
	if !strings.HasPrefix(msg, "<") {
		return msg
	}
	if i := strings.Index(msg, ">"); i > 1 {
		return msg[:i+1]
	}
	return msg
}

// suiteOf is the part of a test's path that names which corner of the suite it
// came from — enough to see whether 199 failures are one area or twelve.
func suiteOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	// css/CSS2/floats-clear/foo.xht → CSS2/floats-clear
	if len(parts) >= 3 && parts[0] == "css" {
		return strings.Join(parts[1:len(parts)-1], "/")
	}
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "/")
	}
	return rel
}

// familyOf strips a trailing sequence number, so that white-space-pre-001 …
// -005 count as one family and a long tail is visible as a long tail.
func familyOf(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	for len(base) > 0 {
		c := base[len(base)-1]
		if c >= '0' && c <= '9' || c == '-' || c == '_' {
			base = base[:len(base)-1]
			continue
		}
		break
	}
	if base == "" {
		return "(numbered only)"
	}
	return base
}

func TestWPTBreakdown(t *testing.T) {
	if os.Getenv("WPT_BREAKDOWN") == "" {
		t.Skip("set WPT_BREAKDOWN=1 to run the breakdown sweep")
	}
	root := wptDir(t)
	tests := findReftests(t, root)
	if len(tests) == 0 {
		t.Fatalf("no reftests found under %s; is the sparse checkout set?", root)
	}

	var cleanPass, vacuousPass, fail, broke int

	byRule := newTally()         // every unsupported rule, by rule
	elements := newTally()       // #25
	glyphs := newTally()         // #24
	properties := newTally()     // #26, unsupported-property
	values := newTally()         // #26, unsupported-value
	resources := newTally()      // #27
	silentSuites := newTally()   // #23
	silentFamilies := newTally() // #23
	onlyTaint := newTally()      // passing tests held back by exactly one rule
	// The same tests broken down by what exactly the rule named, which is the
	// actionable list: each of these is a clean pass waiting on one thing.
	onlyDetail := newTally()
	var silent, silentBlank int

	for _, rt := range tests {
		got, gotFindings, gotBlank, err := renderForCompareDetail(root, rt.test)
		if err != nil {
			broke++
			continue
		}
		want, wantFindings, wantBlank, err := renderForCompareDetail(root, rt.ref)
		if err != nil {
			broke++
			continue
		}
		passed := pictureEqual(got, want, pageClip()) != rt.mismatch

		// The rules that fired anywhere in the pair. A test is held back by a
		// taint on either document, so the two are counted together.
		rules := map[Rule]bool{}
		var all []Finding
		all = append(all, gotFindings...)
		all = append(all, wantFindings...)
		unsupported := false
		for _, f := range all {
			if !f.Unsupported() {
				continue
			}
			unsupported = true
			rules[f.Rule] = true
		}
		blank := gotBlank || wantBlank

		switch {
		case !passed:
			fail++
		case !unsupported && !blank:
			cleanPass++
		default:
			vacuousPass++
		}

		for r := range rules {
			byRule.add(string(r), passed)
		}
		if passed && len(rules) == 1 && !blank {
			for r := range rules {
				onlyTaint.add(string(r), passed)
			}
		}

		// #23: failures the engine had nothing to say about.
		if !passed && !unsupported {
			silent++
			if blank {
				silentBlank++
			}
			silentSuites.add(suiteOf(root, rt.test), passed)
			silentFamilies.add(suiteOf(root, rt.test)+"/"+familyOf(rt.test), passed)
		}

		// The per-rule detail. Counted once per test per distinct key, so a
		// document naming <marquee> forty times is one test.
		// A passing test whose only obstacle is one rule. Whatever that rule
		// named is the whole of what stands between it and the clean count.
		soleRule := Rule("")
		if passed && len(rules) == 1 && !blank {
			for r := range rules {
				soleRule = r
			}
		}
		seen := map[string]bool{}
		mark := func(tl *tally, key string) {
			if seen[key] {
				return
			}
			seen[key] = true
			tl.add(key, passed)
			if soleRule != "" && strings.HasPrefix(key, ruleKeyPrefix(soleRule)) {
				onlyDetail.add(key, passed)
			}
		}
		for _, f := range all {
			switch f.Rule {
			case RuleUnsupportedElement:
				mark(elements, "element "+elementOf(f.Message))
			case RuleGlyphMissing:
				mark(glyphs, "glyph "+f.Message)
			case RuleUnsupportedProperty:
				mark(properties, "property "+nameOrMessage(f))
			case RuleUnsupportedValue:
				mark(values, "value "+nameOrMessage(f))
			case RuleResourceBlocked, RuleImageUndecodable:
				mark(resources, string(f.Rule)+" "+resourceKindOf(f))
			}
		}
	}

	t.Logf("%d reftests: %d passed cleanly, %d passed with something unsupported, "+
		"%d failed, %d could not be read",
		len(tests), cleanPass, vacuousPass, fail, broke)

	// The sweep must agree with the ratchet, or nothing below means anything.
	if cleanPass != wptCleanPassBaseline {
		t.Errorf("the breakdown counts %d clean passes and the ratchet's baseline "+
			"is %d; the two sweeps disagree and every number below is suspect",
			cleanPass, wptCleanPassBaseline)
	}

	t.Logf("\n#23: %d failures report nothing unsupported (%d of them paint nothing)",
		silent, silentBlank)
	silentSuites.report(t, "#23 by suite", 30)
	silentFamilies.report(t, "#23 by family", 40)

	byRule.report(t, "every unsupported rule, by tests it appears in", 30)
	onlyTaint.report(t, "passing tests held back by exactly one rule", 30)
	onlyDetail.report(t, "…and what that one rule named — the actionable list", 40)
	elements.report(t, "#25 unsupported-element, by element", 40)
	properties.report(t, "#26 unsupported-property, by property", 40)
	values.report(t, "#26 unsupported-value, by property", 40)
	glyphs.report(t, "#24 glyph-missing, by face and character", 40)
	resources.report(t, "#27 resource-blocked and image-undecodable", 40)
}

// nameOrMessage prefers the structured property name and falls back to the
// message, so a rule that does not fill Property is visible rather than lumped
// under an empty key.
func nameOrMessage(f Finding) string {
	if f.Property != "" {
		return f.Property
	}
	return "(no property) " + f.Message
}

// resourceKindOf reduces a blocked-resource message to what kind of reference
// it was, which is the split #27 asks for: an @font-face the engine did not
// load is a different piece of work from an image it could not decode.
func resourceKindOf(f Finding) string {
	m := strings.ToLower(f.Message)
	switch {
	case strings.Contains(m, "@font-face"), strings.Contains(m, "font face"),
		strings.Contains(m, "font-face"):
		return "@font-face"
	case strings.Contains(m, "background"):
		return "background-image"
	case strings.Contains(m, "<img"), strings.Contains(m, "image"):
		return "image"
	}
	return "other: " + f.Message
}

// ruleKeyPrefix is how the tally keys above are prefixed, so a sole-rule test
// can be attributed to the right detail line and not to a coincidental one.
func ruleKeyPrefix(r Rule) string {
	switch r {
	case RuleUnsupportedElement:
		return "element "
	case RuleGlyphMissing:
		return "glyph "
	case RuleUnsupportedProperty:
		return "property "
	case RuleUnsupportedValue:
		return "value "
	case RuleResourceBlocked, RuleImageUndecodable:
		return string(r) + " "
	}
	return "\x00never"
}
