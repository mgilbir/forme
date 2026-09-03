package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fallback face that does not load says so.
//
// The harness lends the engine a library of about fourteen faces, fetched from
// upstreams that move. A face that goes missing changes what every document
// holding text it covers is set in — and the loader used to skip one with a
// bare "continue", so a fetch that half worked was indistinguishable from an
// engine that had got worse.
//
// It happened: two of the fourteen were absent from a CI run, a hundred
// documents stopped passing cleanly, and the ratchet reported "this is a layout
// regression". That is the one thing it must never say wrongly, because the
// reading it invites is to lower the number.

// TestAFaceThatWillNotLoadIsRecorded.
func TestAFaceThatWillNotLoadIsRecorded(t *testing.T) {
	if os.Getenv(notoEnv) == "" {
		t.Skipf("set %s (or run `make test-wpt`)", notoEnv)
	}
	// A directory holding one real face and one file that is not a font. The
	// loader has to come back with the first and a complaint about the second.
	dir := t.TempDir()
	real, err := os.ReadFile(filepath.Join(os.Getenv(notoEnv), "NotoSans-Regular.ttf"))
	if err != nil {
		t.Skipf("the library has no NotoSans-Regular.ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "NotoSans-Regular.ttf"), real, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Unifont-Regular.otf"),
		[]byte("this is not a font"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := len(missingFallbackFaces.list())
	t.Setenv(notoEnv, dir)
	faces := notoFaces()
	if len(faces) != 1 {
		t.Errorf("the loader came back with %d faces, want 1 — the readable one", len(faces))
	}
	gone := missingFallbackFaces.list()
	if len(gone) <= before {
		t.Fatal("nothing was recorded about the file that is not a font; a face " +
			"skipped in silence is a hundred documents changed in silence")
	}
	var said string
	for _, g := range gone[before:] {
		if strings.Contains(g, "Unifont-Regular.otf") {
			said = g
		}
	}
	if said == "" {
		t.Errorf("the record is %q and names no Unifont-Regular.otf", gone[before:])
	}
	// And the record says *why*, because "two faces are missing" is a question
	// and "it could not be read" is an answer.
	if !strings.Contains(said, "(") {
		t.Errorf("the record is %q and carries no reason", said)
	}
	// A face that is simply absent is recorded the same way: the twelve names
	// the list asks for and this directory does not have.
	if len(gone[before:]) < 2 {
		t.Errorf("only %d faces were recorded; every name the loader asked for and "+
			"did not get should be", len(gone[before:]))
	}
}

// TestTheSameFaceIsRecordedOnce, because the loader runs once per test that
// wants the library and the list is read once at the end.
func TestTheSameFaceIsRecordedOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(notoEnv, dir)
	notoFaces()
	first := len(missingFallbackFaces.list())
	notoFaces()
	if got := len(missingFallbackFaces.list()); got != first {
		t.Errorf("asking twice recorded %d names and then %d; a face is one "+
			"complaint however many times it is asked for", first, got)
	}
}
