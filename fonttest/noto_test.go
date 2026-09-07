package fonttest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule in noto.go, checked in all four of its cases.
//
// The one that matters is set-and-absent, and it is the one a helper cannot
// prove by being used: a test that skips looks exactly like a test that passed,
// so a helper that skips where it should fail is invisible in every run of
// every suite that depends on it. That is what this file is for.

func TestNotoRule(t *testing.T) {
	for _, c := range []struct {
		name         string
		set, present bool
		want         NotoOutcome
	}{
		{"nobody asked and there is nothing", false, false, NotoSkip},
		{"nobody asked and the checkout has it", false, true, NotoUse},
		{"asked for, and it is there", true, true, NotoUse},
		{"asked for, and it is not there", true, false, NotoFail},
	} {
		if got := NotoDecide(c.set, c.present); got != c.want {
			t.Errorf("%s: NotoDecide(%v, %v) = %v, want %v",
				c.name, c.set, c.present, got, c.want)
		}
	}
}

// recorder is a TB that records rather than stopping, so that the two ways
// NotoDir refuses can both be observed from inside one test.
//
// Helper calls that would stop a real test return here, so every method that
// stops one panics with a sentinel and the caller recovers it — otherwise
// NotoDir would carry on past its own Fatalf and read a directory it has just
// said is not there.
type recorder struct {
	skipped, failed string
}

type stopped struct{}

func (r *recorder) Helper() {}

func (r *recorder) Skipf(format string, args ...any) {
	r.skipped = fmt.Sprintf(format, args...)
	panic(stopped{})
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failed = fmt.Sprintf(format, args...)
	panic(stopped{})
}

// run calls f with a recording TB and returns what it did.
func run(f func(TB)) (rec *recorder) {
	rec = &recorder{}
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(stopped); !ok {
				panic(p)
			}
		}
	}()
	f(rec)
	return rec
}

// library writes a directory that looks like the fetched font library. The
// contents do not matter to the rule — only whether the marker face is there.
func library(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, notoMarker), []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAWrongFontLibraryFailsRatherThanSkipping(t *testing.T) {
	t.Setenv(NotoEnv, filepath.Join(t.TempDir(), "no-such-directory"))

	rec := run(func(tb TB) { NotoDir(tb) })
	if rec.skipped != "" {
		t.Errorf("a directory that is set and wrong skipped: %s\n"+
			"A skip is indistinguishable from a pass, so sixty tests about "+
			"real faces would report success having opened none.", rec.skipped)
	}
	if !strings.Contains(rec.failed, NotoEnv) {
		t.Errorf("the failure does not name %s, so a reader cannot tell what "+
			"to fix: %q", NotoEnv, rec.failed)
	}
}

func TestAnAbsentLibraryNobodyAskedForSkips(t *testing.T) {
	t.Setenv(NotoEnv, "")
	defer swapDefault(t, filepath.Join(t.TempDir(), "no-such-directory"))()

	rec := run(func(tb TB) { NotoDir(tb) })
	if rec.failed != "" {
		t.Errorf("a checkout that never fetched the fonts failed: %s\n"+
			"Nobody asked for a library here, and `go test ./...` has to run "+
			"without one.", rec.failed)
	}
	if rec.skipped == "" {
		t.Error("an absent library neither skipped nor failed")
	}
}

func TestTheCheckoutsOwnLibraryIsUsedWithoutBeingNamed(t *testing.T) {
	dir := library(t)
	t.Setenv(NotoEnv, "")
	defer swapDefault(t, dir)()

	var got string
	rec := run(func(tb TB) { got = NotoDir(tb) })
	if got != dir {
		t.Errorf("NotoDir came back with %q, want the fetch directory %q "+
			"(skipped: %q, failed: %q)", got, dir, rec.skipped, rec.failed)
	}
}

func TestAFaceMissingFromAPresentLibraryFails(t *testing.T) {
	t.Setenv(NotoEnv, library(t))

	rec := run(func(tb TB) { NotoFile(tb, "NotoSansArabic-Regular.ttf") })
	if rec.skipped != "" {
		t.Errorf("a face missing from a library that is there skipped: %s\n"+
			"The Makefile fetches every face asked for, so this is an "+
			"unfinished fetch and not a difference between checkouts.", rec.skipped)
	}
	if !strings.Contains(rec.failed, "NotoSansArabic-Regular.ttf") {
		t.Errorf("the failure does not name the face: %q", rec.failed)
	}
}

func TestAPresentFaceIsRead(t *testing.T) {
	t.Setenv(NotoEnv, library(t))

	var got []byte
	rec := run(func(tb TB) { got = NotoFile(tb, notoMarker) })
	if string(got) != "not a font" {
		t.Errorf("NotoFile came back with %q (skipped: %q, failed: %q)",
			got, rec.skipped, rec.failed)
	}
}

// swapDefault points the fetch directory somewhere else for one test, and
// returns the call that puts it back.
func swapDefault(t *testing.T, dir string) func() {
	t.Helper()
	was := notoDefault
	notoDefault = dir
	return func() { notoDefault = was }
}
