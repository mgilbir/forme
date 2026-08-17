package style

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guard against a property being registered and then never read.
//
// This is the failure this package got wrong sixteen times over, and it is worth
// being precise about why it is invisible without a test. Registering a property
// is the *first* step of implementing one, and it is also the step that switches
// off the unsupported-property finding — because being in the registry is what
// "supported" means. Whoever adds the entry means to write the code that reads
// it. If they stop there, the engine accepts the declaration, computes a value,
// inherits it, and then lays the page out as though it were never written, with
// nothing to say so.
//
// Nothing else catches it. A reftest cannot: it renders both documents with the
// same engine, so a property ignored in both moves neither. A unit test for the
// property is not written, because the property was not implemented. The
// declaration parses, the cascade is correct, and every test passes.
//
// So the check is on the source itself: a registered property must appear as a
// literal string somewhere that reads it, or be named as one whose name is built
// rather than written, or be admitted as unimplemented — which puts the finding
// back.

// sourceDirs are the packages that read computed values.
//
// paragraph joined the list when the white space processing and the text
// transforms moved out of layout, and it joined it because this check noticed:
// "overflow-wrap" and "word-wrap" were still read, by the same code as before,
// and the scan could no longer see where. A property whose reader moves to a
// package nobody looks in is indistinguishable here from a property with no
// reader at all, which is the whole thing this file exists to catch — so the
// list has to follow the code, and a failure here is the reminder to move it.
var sourceDirs = []string{".", "../layout", "../paragraph"}

// registryFiles hold the tables themselves, where every property name appears by
// definition and so proves nothing.
var registryFiles = map[string]bool{
	"property.go":      true,
	"shorthand.go":     true,
	"unimplemented.go": true,
	// inert.go is a table of the same kind, and naming a property there says
	// the opposite of reading it: an entry is that property's initial value,
	// recorded so that a declaration setting it to that value is not reported.
	// It is only ever consulted for a property the engine does not implement.
	// TestEveryInertPropertyIsUnimplemented is what holds it to that, and is
	// why this exclusion is not a hole.
	"inert.go": true,
}

func readSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			if registryFiles[name] {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			all.Write(data)
		}
	}
	if all.Len() == 0 {
		t.Fatal("no sources were read; the guard would pass vacuously")
	}
	return all.String()
}

func TestEveryRegisteredPropertyIsReadOrAdmitted(t *testing.T) {
	src := readSources(t)

	for name := range properties {
		if _, isShorthand := shorthands[name]; isShorthand {
			// A shorthand is never read directly: it is expanded into longhands
			// during the cascade, and those are what layout asks for.
			continue
		}
		if fragment, ok := readByConstruction[name]; ok {
			// The claim is that the name is assembled from this fragment, so
			// the fragment itself must be in the source. Otherwise this map is
			// simply a way to silence the guard.
			if !strings.Contains(src, `"`+fragment+`"`) {
				t.Errorf("%q is listed as assembled from %q, but no source contains "+
					"that literal — the entry is waving the property through rather "+
					"than describing how it is read", name, fragment)
			}
			continue
		}
		if _, ok := unimplementedProperties[name]; ok {
			continue
		}
		if strings.Contains(src, `"`+name+`"`) {
			continue
		}
		t.Errorf("the property %q is registered as understood, but no source outside "+
			"the registry names it — so a declaration of it is accepted, cascaded, and "+
			"then ignored with nothing reported. Implement it, or add it to "+
			"unimplementedProperties so that it is reported, or to readByConstruction "+
			"if its name is assembled rather than written.", name)
	}
}

// TestTheGuardHasTeeth checks the check, because a source scan that matched
// everything would pass silently and for ever.
func TestTheGuardHasTeeth(t *testing.T) {
	src := readSources(t)

	// A name no source can contain must not be found.
	if strings.Contains(src, `"not-a-real-property-xyzzy"`) {
		t.Fatal("the scan found a property that does not exist; it is not reading source")
	}
	// And a property that really is read must be found, or the guard would be
	// admitting everything for the wrong reason.
	for _, known := range []string{"text-align", "white-space", "display"} {
		if !strings.Contains(src, `"`+known+`"`) {
			t.Errorf("%q is implemented but the scan did not find it; the guard would "+
				"report a property that is genuinely read", known)
		}
	}
}

// TestUnimplementedPropertiesAreRegistered pins that the admissions list is
// about real properties. An entry naming a property that is not registered is a
// leftover from one that was implemented or renamed, and it would sit here
// suppressing a check that no longer applies to anything.
func TestUnimplementedPropertiesAreRegistered(t *testing.T) {
	src := readSources(t)
	for name := range unimplementedProperties {
		if _, ok := properties[name]; !ok {
			t.Errorf("%q is admitted as unimplemented but is not a registered property; "+
				"the entry is stale", name)
		}
		// The other way an entry goes stale, and the one that actually
		// happened: the property gets implemented and nobody removes the
		// admission. It then reports a gap that has been closed, which is worse
		// than the silence it was added to fix — an author is told a
		// declaration did nothing while looking at the thing it did, and the
		// reftest oracle counts every document using it as tainted.
		//
		// It went unnoticed through a merge because nothing checked it. Being
		// read somewhere and being admitted as unread are contradictory claims,
		// so holding both is an error whichever one is wrong.
		if strings.Contains(src, `"`+name+`"`) {
			t.Errorf("%q is admitted as unimplemented, but source outside the "+
				"registry reads it — if it is implemented, delete the admission so "+
				"it stops being reported", name)
		}
	}
	for name := range readByConstruction {
		if _, ok := properties[name]; !ok {
			t.Errorf("%q is listed as read by construction but is not registered", name)
		}
	}
}

// TestEveryInertPropertyIsUnimplemented holds inert.go to its own premise.
//
// The table records initial values so that a declaration setting a property to
// its own initial value is not reported as unapplied. That reasoning depends
// entirely on the property being one the engine does not act on: for a property
// it *does* implement, the initial value is a real value with a real effect, and
// suppressing the finding would be suppressing nothing — but the entry would sit
// there claiming otherwise, and the next person to read it would believe it.
//
// It is also what keeps the registryFiles exclusion above honest. Naming a
// property in inert.go is exempt from the source scan, so without this a name
// could be moved there to silence the scan rather than to state a fact.
func TestEveryInertPropertyIsUnimplemented(t *testing.T) {
	for name := range inertValues {
		_, registered := properties[name]
		_, admitted := unimplementedProperties[name]
		if registered && !admitted {
			t.Errorf("%q has an entry in inert.go, but the engine implements "+
				"it — the entry suppresses a finding about a property that is applied, "+
				"and it reads as documentation that is not true", name)
		}
	}
}

// TestTheInertGuardHasTeeth checks the check. A property the engine really does
// implement must be seen to fail it, or the test above would pass for every
// table anyone wrote.
func TestTheInertGuardHasTeeth(t *testing.T) {
	implemented := ""
	for _, name := range []string{"display", "text-align", "white-space", "color"} {
		if _, ok := properties[name]; ok {
			if _, admitted := unimplementedProperties[name]; !admitted {
				implemented = name
				break
			}
		}
	}
	if implemented == "" {
		t.Fatal("no implemented property was found to test the guard with")
	}
	if _, ok := inertValues[implemented]; ok {
		t.Fatalf("%q is implemented and is already in inert.go", implemented)
	}
	// The condition the test above applies, evaluated by hand on a property that
	// must trip it.
	_, registered := properties[implemented]
	_, admitted := unimplementedProperties[implemented]
	if !(registered && !admitted) {
		t.Errorf("%q is implemented, and the condition TestEveryInertPropertyIsUnimplemented "+
			"tests would not have caught it in the table", implemented)
	}
}
