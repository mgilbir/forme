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

// CJKEnv names the variable holding the CID-keyed CJK faces.
const CJKEnv = "NOTO_CJK"

// corpus is one fetched font library: the variable that names it, where the
// Makefile puts it, and the face whose presence says a directory is it.
//
// The marker is a face rather than the directory itself, because a directory is
// created by the first half of a fetch that then failed.
type corpus struct {
	env     string
	dir     string
	marker  string
	fetches string // the Makefile target that puts it there
}

// noto is the fallback library, and cjk the CID-keyed faces the CFF reader is
// checked against. They are variables rather than constants so that the rule
// itself can be tested against a directory that is not there.
var (
	noto = corpus{
		env:     NotoEnv,
		dir:     filepath.Join("..", "testdata", "fonts-noto"),
		marker:  "NotoSans-Regular.ttf",
		fetches: "make noto-fonts",
	}
	cjk = corpus{
		env:     CJKEnv,
		dir:     filepath.Join("..", "testdata", "notocjk"),
		marker:  "NotoSansJP-Regular.otf",
		fetches: "make notocjk",
	}
)

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

// root resolves a corpus's directory without deciding anything: the variable if
// it is set, and the fetch directory otherwise. It reports which of the two it
// returned, because the answer to a directory that is not there depends on
// whether somebody asked for it.
func (c corpus) root() (dir string, set bool) {
	if dir := os.Getenv(c.env); dir != "" {
		return dir, true
	}
	return c.dir, false
}

// present reports whether a directory holds this corpus.
func (c corpus) present(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, c.marker))
	return err == nil
}

// at returns the directory holding the corpus, skipping the test when there is
// none to read and failing it when one was named and is not there.
func (c corpus) at(t TB) string {
	t.Helper()
	dir, set := c.root()
	switch NotoDecide(set, c.present(dir)) {
	case NotoUse:
		return dir
	case NotoSkip:
		t.Skipf("these faces are not in this checkout; run `%s` to fetch them "+
			"into %s", c.fetches, c.dir)
	default:
		t.Fatalf("%s is set to %q, and there is no %s there.\n"+
			"It must name the directory `%s` fetches into.\n"+
			"Failing rather than skipping: these tests are about what a real\n"+
			"face does, and a skip would report success having read no font.",
			c.env, dir, c.marker, c.fetches)
	}
	return ""
}

// file reads one of the fetched faces by name.
//
// A face that is absent from a corpus that is present fails rather than
// skipping: the Makefile fetches every name asked for here, so this is a fetch
// that did not finish, and a test that quietly stops checking a script is how
// that goes unnoticed.
func (c corpus) file(t TB, name string) []byte {
	t.Helper()
	dir := c.at(t)
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s out of %s: %v\n"+
			"`%s` fetches this face; a directory holding the corpus but not "+
			"this file is an unfinished fetch.", name, c.env, err, c.fetches)
		return nil
	}
	return data
}

// NotoRoot is the fallback library's directory and whether NOTO_FONTS named it.
//
// Exported for the WPT harness, which loads the library outside any one test
// and so has no *testing.T to stop.
func NotoRoot() (dir string, set bool) { return noto.root() }

// NotoPresent reports whether a directory holds the fallback library.
func NotoPresent(dir string) bool { return noto.present(dir) }

// NotoDir is the fetched fallback faces, or the reason there are none.
func NotoDir(t TB) string { t.Helper(); return noto.at(t) }

// NotoFile reads one of the fetched fallback faces by name.
func NotoFile(t TB, name string) []byte { t.Helper(); return noto.file(t, name) }

// CJKDir is the fetched CID-keyed faces, or the reason there are none.
//
// They are a corpus of their own because what they are for is different: a
// CID-keyed CFF has a charset that is not the identity, which is the structure
// no synthetic font here builds and the one the reader has to get right. CI
// fetches them and names the directory, so a run where these tests skip is a
// run that checked nothing and said so — which is the whole point of naming it.
func CJKDir(t TB) string { t.Helper(); return cjk.at(t) }

// CJKFile reads one of the fetched CID-keyed faces by name.
func CJKFile(t TB, name string) []byte { t.Helper(); return cjk.file(t, name) }
