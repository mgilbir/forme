package shape

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// putBe16 appends a big-endian uint16, which is how every count and offset in
// these tables is written. (be16 in this package reads one.)
func putBe16(b []byte, v int) []byte { return binary.BigEndian.AppendUint16(b, uint16(v)) }

// TestAScriptListBeyondTheBoundStopsBeingRead is the bound on how many scripts
// one ScriptList may name.
//
// The count is two bytes of the font and each record is six more. Past the
// bound the rest are not read, so a script declared there is not found — which
// is what this asserts, by asking for one that lives past it.
//
// Nothing had declared that many: every fixture names one or two scripts, so
// the shape suite passes with maxScripts raised.
func TestAScriptListBeyondTheBoundStopsBeingRead(t *testing.T) {
	// A literal, not maxScripts+n. Sizing the fixture from the constant means a
	// plant that raises it also grows the fixture — past a thousand records the
	// tags below stop being four bytes, the reader stores a truncated key, and
	// the lookup misses for a reason that has nothing to do with the bound. It
	// passed with maxScripts at a million that way.
	//
	// This is above the bound of 256 and below the thousand that breaks the
	// tags, so raising the bound reads the whole list and the last script is
	// found, which is the failure this is here to produce.
	const declared = 320
	tagAt := func(i int) string { return fmt.Sprintf("S%03d", i) }

	// The records first, then one Script table they all point at. Every offset
	// is from the start of the list, which is why the table goes at the end.
	head := putBe16(nil, declared)
	scriptOff := 2 + 6*declared
	for i := 0; i < declared; i++ {
		head = append(head, tagAt(i)...)
		head = putBe16(head, scriptOff)
	}
	// A Script table: no default LangSys, no LangSys records.
	list := append(head, 0, 0, 0, 0)

	got := scriptOffsets(list)
	if _, ok := got[tagAt(0)]; !ok {
		t.Fatalf("the first of %d scripts was not found; the fixture is not a "+
			"ScriptList this reader walks", declared)
	}
	if _, ok := got[tagAt(declared-1)]; ok {
		t.Errorf("the script declared at position %d was found; past the bound of "+
			"%d the list stops being read", declared-1, maxScripts)
	}
	if len(got) >= declared {
		t.Errorf("%d of %d declared scripts were read; the list is bounded",
			len(got), declared)
	}
}

// TestALangSysListBeyondTheBoundStopsBeingRead is the same question one level
// down: how many language systems one script may name.
//
// A language system selects which features apply, so one found past the bound
// would be one the reader was never meant to walk to.
func TestALangSysListBeyondTheBoundStopsBeingRead(t *testing.T) {
	// A literal, for the reason the test above gives.
	const declared = 320
	tagAt := func(i int) string { return fmt.Sprintf("L%03d", i) }

	// A Script table: no default LangSys, then the records, then one LangSys
	// table they all point at.
	head := putBe16(nil, 0)
	head = putBe16(head, declared)
	lsOff := 4 + 6*declared
	for i := 0; i < declared; i++ {
		head = append(head, tagAt(i)...)
		head = putBe16(head, lsOff)
	}
	// A LangSys table: lookupOrder, requiredFeatureIndex, featureIndexCount.
	script := append(head, 0, 0, 0, 1, 0, 0)

	// The LangSys every record names requires feature 1, which is how finding
	// it is told from the empty one a script with no default falls back to.
	if ls, _ := readLangSys(script, []string{tagAt(0)}); ls.required != 1 {
		t.Fatalf("the first of %d language systems was not found; the fixture is "+
			"not a Script table this reader walks", declared)
	}
	// One past the bound is not found, so the reader falls back to the default
	// LangSys — which this table does not have, so the answer is the empty one.
	if ls, _ := readLangSys(script, []string{tagAt(declared - 1)}); ls.required == 1 {
		t.Errorf("the language system declared at position %d was found; past the "+
			"bound of %d the list stops being read", declared-1, maxLangSys)
	}
}

// TestAFeatureSubstitutionListBeyondTheBoundStopsBeingRead is the bound on how
// many features one FeatureVariations record may substitute.
//
// A variable font states, for a region of its design space, that some features
// are replaced by others — "A real face states a handful of records — Noto Sans
// Oriya states one — and each names a few conditions and a few substituted
// features." The count is two bytes of the font.
//
// This is the one bound in the audit whose guarded path the suite never reached
// at all: lowering it to one changed nothing, because no fixture here has a
// FeatureVariations table. So this is the first table of its kind in the
// package, and it is built by hand for the same reason the script list is —
// the reader takes raw bytes, so the fixture is the table.
func TestAFeatureSubstitutionListBeyondTheBoundStopsBeingRead(t *testing.T) {
	// A literal above the bound, for the reason the script list test gives: a
	// fixture sized from the constant grows when the constant does.
	const declared = 320

	// FeatureTableSubstitution: version 1.0, a count, then the records. Each
	// record is a feature index and a 32-bit offset to the feature that
	// replaces it; they all point at one, placed after the records.
	head := putBe16(nil, 1) // majorVersion, which the reader requires
	head = putBe16(head, 0) // minorVersion
	head = putBe16(head, declared)
	altOff := 6 + 6*declared
	for i := 0; i < declared; i++ {
		head = putBe16(head, i) // the feature index this record replaces
		head = append(head, 0, 0)
		head = putBe16(head, altOff) // the low half of the 32-bit offset
	}
	// An AlternateFeatureTable: a parameters offset, then one lookup index.
	ts := append(head, 0, 0, 0, 1, 0, 7)

	// The table is placed at a non-zero offset within the FeatureVariations
	// bytes: the reader treats an offset of zero as "no table", which is the
	// format's own way of saying a record substitutes nothing.
	const at = 4
	fv := append(make([]byte, at), ts...)

	got := featureTableSubstitution(fv, at)
	if got == nil {
		t.Fatal("a FeatureTableSubstitution of 320 records read as nothing; the " +
			"fixture is not a table this reader walks")
	}
	if _, ok := got[0]; !ok {
		t.Fatalf("the first of %d substitutions was not read; the fixture does not "+
			"reach what the bound is being asked about", declared)
	}
	if _, ok := got[declared-1]; ok {
		t.Errorf("the substitution declared at position %d was read; past the bound "+
			"of %d the list stops being read", declared-1, maxFeatureSubsts)
	}
	if len(got) >= declared {
		t.Errorf("%d of %d declared substitutions were read; the list is bounded",
			len(got), declared)
	}
}
