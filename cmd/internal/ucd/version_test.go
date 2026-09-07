package ucd

import (
	"os"
	"path/filepath"
	"testing"
)

// What a UCD file's first line says, and what it does not.
//
// The "does not" half is the one worth writing down: emoji-data.txt begins
// "# emoji-data.txt", which has a hyphen in it and no release, and a reader
// that took what followed the last hyphen would call the version "data" and
// then fail every check against it.

func write(t *testing.T, first string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(path, []byte(first+"\n# Date: 2025-07-25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheDeclaredVersion(t *testing.T) {
	for _, c := range []struct {
		first string
		want  string
		ok    bool
	}{
		{"# LineBreak-17.0.0.txt", "17.0.0", true},
		{"# DerivedBidiClass-16.0.0.txt", "16.0.0", true},
		{"#EastAsianWidth-17.0.0.txt", "17.0.0", true},
		// No release in it, and a hyphen that is part of the name.
		{"# emoji-data.txt", "", false},
		// No header at all, which is UnicodeData.txt.
		{"0000;<control>;Cc;0;BN;;;;;N;NULL;;;;", "", false},
		{"# something", "", false},
		{"", "", false},
	} {
		got, ok := Declared(write(t, c.first))
		if got != c.want || ok != c.ok {
			t.Errorf("%q declares (%q, %v), want (%q, %v)", c.first, got, ok, c.want, c.ok)
		}
	}
	if _, ok := Declared(filepath.Join(t.TempDir(), "no-such-file")); ok {
		t.Error("a file that is not there declared a version")
	}
}

// TestFilesFromTwoReleasesAreRefused is what the check is for. A table built
// from one release's LineBreak.txt and another's Scripts.txt is wrong for every
// character the two disagree about, and it is wrong in a way no test downstream
// would name.
func TestFilesFromTwoReleasesAreRefused(t *testing.T) {
	a := write(t, "# EastAsianWidth-17.0.0.txt")
	b := write(t, "# Scripts-16.0.0.txt")
	none := write(t, "0000;<control>;Cc;0;BN;;;;;N;NULL;;;;")

	if err := Check("17.0.0", a, none); err != nil {
		t.Errorf("a file from the named release and one that names none were "+
			"refused: %v", err)
	}
	if err := Check("17.0.0", a, b); err == nil {
		t.Error("a file from 16.0.0 passed a run told it was 17.0.0")
	}
	if err := Check("", a); err == nil {
		t.Error("a run with no version at all was accepted; the release the " +
			"output names would be the empty string")
	}
}
