package ucd

import (
	"fmt"
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
	data := writeNamed(t, "UnicodeData.txt", "0000;<control>;Cc;0;BN;;;;;N;NULL;;;;")

	if err := Check("17.0.0", a, data); err != nil {
		t.Errorf("a file from the named release and UnicodeData.txt, which "+
			"names none, were refused: %v", err)
	}
	if err := Check("17.0.0", a, b); err == nil {
		t.Error("a file from 16.0.0 passed a run told it was 17.0.0")
	}
	if err := Check("", a); err == nil {
		t.Error("a run with no version at all was accepted; the release the " +
			"output names would be the empty string")
	}
}

// TestAFileThatNamesNoReleaseIsRefused is the half Check's comment used to
// promise and not keep. It skipped a file that declared nothing, so a run handed
// only such files — a truncated fetch, an HTML error page saved under the right
// name — checked nothing and labelled its output with whatever it was told.
func TestAFileThatNamesNoReleaseIsRefused(t *testing.T) {
	a := write(t, "# EastAsianWidth-17.0.0.txt")
	for _, first := range []string{
		"<!DOCTYPE html>",
		"0000;<control>;Cc;0;BN;;;;;N;NULL;;;;", // UnicodeData.txt's shape, under another name
		"# emoji-data.txt",                      // a header, with no release in it
		"",
	} {
		if err := Check("17.0.0", a, write(t, first)); err == nil {
			t.Errorf("a file beginning %q passed as a file from Unicode 17.0.0", first)
		}
	}
	if err := Check("17.0.0", filepath.Join(t.TempDir(), "UnicodeData.txt")); err == nil {
		t.Error("a UnicodeData.txt that is not there passed")
	}
}

// TestEmojiDataIsCheckedAgainstItsOwnVersionLine: emoji-data.txt names its
// release in the Emoji numbering, a few lines into its header, and that is
// what it is checked by.
func TestEmojiDataIsCheckedAgainstItsOwnVersionLine(t *testing.T) {
	header := "# emoji-data.txt\n# Date: 2025-07-25\n#\n# Emoji Data for UTS #51\n# Version: %s\n#\n0023 ; Emoji\n"
	emoji := func(v string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "emoji-data.txt")
		if err := os.WriteFile(path, []byte(fmt.Sprintf(header, v)), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := Check("17.0.0", emoji("17.0")); err != nil {
		t.Errorf("Emoji 17.0 was refused for Unicode 17.0.0: %v", err)
	}
	for _, v := range []string{"16.0", "1", "17.00", ""} {
		if err := Check("17.0.0", emoji(v)); err == nil {
			t.Errorf("Emoji %q passed for Unicode 17.0.0", v)
		}
	}
}

func writeNamed(t *testing.T, name, first string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(first+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
