package style

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The properties the README says this engine does something with.
//
// "What it does" is prose, and prose about a program goes out of date in one
// direction quietly: a feature named there and then removed leaves a claim with
// nothing behind it, and nothing fails. The section went out of date in the
// other direction too — flexbox, grid, multiple columns and the vertical writing
// modes were all implemented and none of them was mentioned.
//
// What can be checked is the part that names a property. Every backticked
// property in that section has to be one the cascade knows: registered as a
// longhand, or expanded as a shorthand. A claim about a property this engine has
// never heard of is a claim about something else's engine.

// readmeProperty matches a backticked property name, with or without a value:
// `line-clamp`, `text-wrap: balance`, `Page.MinScale` (which it does not match,
// because that is a Go field and not a property).
var readmeProperty = regexp.MustCompile("`([a-z]+(?:-[a-z0-9]+)+)(?::[^`]*)?`")

// TestTheReadmeNamesPropertiesThisEngineHas.
func TestTheReadmeNamesPropertiesThisEngineHas(t *testing.T) {
	src, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "## What it does")
	if start < 0 {
		t.Fatal("the README has no \"What it does\" section, so this checks nothing")
	}
	end := strings.Index(text[start+3:], "\n## ")
	if end < 0 {
		t.Fatal("the section does not end")
	}
	section := text[start : start+3+end]

	named := 0
	for _, m := range readmeProperty.FindAllStringSubmatch(section, -1) {
		name := m[1]
		if _, ok := properties[name]; ok {
			named++
			continue
		}
		if _, ok := shorthands[name]; ok {
			named++
			continue
		}
		t.Errorf("the README says %q and the cascade has never heard of it: it is "+
			"neither a registered property nor a shorthand this engine expands", name)
	}
	// The pattern is what the check rests on. The section names several
	// properties, and one that matched none of them would pass in silence.
	if named < 5 {
		t.Errorf("only %d properties were found in \"What it does\"; the pattern "+
			"that reads them off is not matching what it should", named)
	}
}
