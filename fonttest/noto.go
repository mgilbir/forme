package fonttest

import (
	"os"
	"path/filepath"
)

// Where the fetched Noto faces are, and what to do when they are not there.
//
// # Why this is not four lines at each call site
//
// It was, in about thirty places, and every one of them read:
//
//	dir := os.Getenv("NOTO_FONTS")
//	if dir == "" {
//		t.Skip("set NOTO_FONTS")
//	}
//	data, err := os.ReadFile(filepath.Join(dir, name))
//	if err != nil {
//		t.Skipf("not in this checkout: %v", err)
//	}
//
// which skips on a directory that is set and wrong. That is the case that
// matters: somebody naming a directory is asserting the faces are in it, and a
// mistyped path then turns sixty tests about what a real font does into sixty
// lines of "--- SKIP" that nothing reads. A skip is indistinguishable from a
// pass in the output, so the tests go on reporting success having opened no
// font at all — which is how a CI job can be green while checking nothing.
//
// Every other corpus in this repository already refuses that: bidi's testDir
// and segment's graphemeSuite both fail when their variable is set and the data
// is not where it points. This is the same rule for the font library, in one
// place so that the next test to want a face inherits it.
//
// # The three cases
//
//   - Unset, and the checkout has no faces: skip. A checkout that has not run
//     `make noto-fonts` cannot be blamed for having no fonts.
//   - Unset, and the checkout has them: use them. The fetch directory is where
//     the Makefile puts them and there is no reason to make a caller say so.
//   - Set: use it, and fail if the faces are not there.
//
// The same applies to a single face within a present library. Every name asked
// for is one the Makefile fetches, so a directory that has the library but not
// that file is an incomplete fetch rather than a difference between checkouts.

// NotoEnv names the variable holding the fallback font library.
const NotoEnv = "NOTO_FONTS"

// notoDefault is where `make noto-fonts` fetches, relative to a package
// directory — which is where `go test` runs.
var notoDefault = filepath.Join("..", "testdata", "fonts-noto")

// notoMarker is the face whose presence says a directory is the library. It is
// the first one the Makefile fetches and the one most tests here ask for.
const notoMarker = "NotoSans-Regular.ttf"

// TB is the part of *testing.T these helpers use, named here so that this
// package does not have to import testing — it is linked into cmd/genwoff2hmtx,
// which is a program rather than a test.
type TB interface {
	Helper()
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// NotoOutcome is what a caller should do about the font library.
type NotoOutcome int

const (
	// NotoUse: the library is there and is to be read.
	NotoUse NotoOutcome = iota
	// NotoSkip: nobody asked for a library and this checkout has none.
	NotoSkip
	// NotoFail: somebody named a directory and the faces are not in it.
	NotoFail
)

// NotoDecide is the rule above as a value.
//
// It is separate from NotoDir so that the rule can be tested — a helper that
// stops the test is a helper whose decision cannot be observed from inside one,
// and the decision is the entire point of this file. See TestNotoRule.
func NotoDecide(set, present bool) NotoOutcome {
	switch {
	case present:
		return NotoUse
	case set:
		return NotoFail
	default:
		return NotoSkip
	}
}

// NotoRoot resolves the library's directory without deciding anything: the
// variable if it is set, and the fetch directory otherwise. It reports which of
// the two it returned, because the answer to a directory that is not there
// depends on whether somebody asked for it.
//
// It is exported for the WPT harness, which loads the library outside any one
// test and so has no *testing.T to stop.
func NotoRoot() (dir string, set bool) {
	if dir := os.Getenv(NotoEnv); dir != "" {
		return dir, true
	}
	return notoDefault, false
}

// NotoPresent reports whether a directory holds the font library.
func NotoPresent(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, notoMarker))
	return err == nil
}

// NotoDir returns the directory holding the fetched Noto faces, skipping the
// test when there is no library to read and failing it when one was named and
// is not there.
func NotoDir(t TB) string {
	t.Helper()
	dir, set := NotoRoot()
	switch NotoDecide(set, NotoPresent(dir)) {
	case NotoUse:
		return dir
	case NotoSkip:
		t.Skipf("the fallback font library is not in this checkout; run "+
			"`make noto-fonts` to fetch it into %s", notoDefault)
	default:
		t.Fatalf("%s is set to %q, and there is no %s there.\n"+
			"It must name the directory `make noto-fonts` fetches into.\n"+
			"Failing rather than skipping: these tests are about what a real\n"+
			"face does, and a skip would report success having read no font.",
			NotoEnv, dir, notoMarker)
	}
	return ""
}

// NotoFile reads one of the fetched faces by name.
//
// A face that is absent from a library that is present fails rather than
// skipping: the Makefile fetches every name asked for here, so this is a fetch
// that did not finish, and a test that quietly stops checking a script is how
// that goes unnoticed.
func NotoFile(t TB, name string) []byte {
	t.Helper()
	dir := NotoDir(t)
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s from the font library: %v\n"+
			"`make noto-fonts` fetches this face; a directory holding the "+
			"library but not this file is an unfinished fetch.", name, err)
		return nil
	}
	return data
}
