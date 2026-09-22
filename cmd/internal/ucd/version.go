// Package ucd is the small part of reading the Unicode Character Database that
// every generator in cmd/ has in common.
//
// Which is, at present, one thing: what version of the database a file came
// from. Six generators worked it out from the file's first line and three typed
// it into their output — "Unicode 17.0.0", regardless of what they were handed —
// so a table regenerated from a later release would carry a header naming the
// release before it, and nothing downstream would say otherwise. A provenance
// line that is a constant is worse than none: it is read as a fact.
package ucd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Declared is the version a UCD file names, and whether it names one.
//
// Almost every file in the database begins with a comment naming itself and the
// release: "# LineBreak-17.0.0.txt". UnicodeData.txt is the exception — it is
// pure data with no header at all — which is why the generators that read only
// that file are told the version rather than reading it.
func Declared(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return "", false
	}
	line := strings.TrimSpace(sc.Text())
	if !strings.HasPrefix(line, "#") || !strings.HasSuffix(line, ".txt") {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "#")), ".txt")
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return "", false
	}
	v := name[i+1:]
	// A version and not the rest of a name: "emoji-data.txt" ends in a hyphen
	// group too, and what follows it is "data".
	if v == "" || v[0] < '0' || v[0] > '9' {
		return "", false
	}
	return v, true
}

// Check verifies that every one of the given files is from the release want,
// which is the release the output will name.
//
// Two files from two releases is the failure it exists for: a table built from
// LineBreak.txt of one release and Scripts.txt of another is wrong for every
// character the two disagree about, and it is wrong in a way no test downstream
// would name.
//
// Every file has to say which release it is from, and a file that says nothing
// is refused rather than waved through. This used to skip such a file and
// promise, in this comment, that at least one of the inputs was checked; it
// checked none when none declared one, so a file that was not from the
// database at all — a truncated fetch, the wrong path — passed as readily as a
// right one. Two files in the database are exceptions, and they are named:
//
//   - UnicodeData.txt is pure data with no header at all. It is the one file a
//     generator is told the release of rather than reading it, and a
//     generator that reads only UnicodeData.txt is checked against nothing.
//     That is stated, not hidden: it is what -version is for.
//   - emoji/emoji-data.txt names its release on a "# Version: 17.0" line of
//     its own, in the Emoji numbering, which since Emoji 11 is the Unicode
//     major and minor version. It is checked against those.
func Check(want string, paths ...string) error {
	if want == "" {
		return fmt.Errorf("no Unicode version given; pass -version, which the " +
			"Makefile fills in from UNICODE_VERSION")
	}
	for _, p := range paths {
		switch filepath.Base(p) {
		case "UnicodeData.txt":
			if _, err := os.Stat(p); err != nil {
				return err
			}
			continue
		case "emoji-data.txt":
			got, ok := declaredEmoji(p)
			if !ok {
				return fmt.Errorf("%s names no \"# Version:\" line, so which release "+
					"it is from cannot be checked", p)
			}
			if !strings.HasPrefix(want, got+".") {
				return fmt.Errorf("%s is from Emoji %s and this run was told Unicode %s;\n"+
					"the files are not all from the release the output would name",
					p, got, want)
			}
			continue
		}
		got, ok := Declared(p)
		if !ok {
			return fmt.Errorf("%s names no release on its first line, as every file "+
				"of the database does; it is not a file from the database this was "+
				"pointed at, or not the whole of one", p)
		}
		if got != want {
			return fmt.Errorf("%s is from Unicode %s and this run was told %s;\n"+
				"the files are not all from the release the output would name",
				p, got, want)
		}
	}
	return nil
}

// declaredEmoji is the release emoji-data.txt names on its "# Version:" line,
// which is in its header and not on its first line.
func declaredEmoji(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "#") {
			// The header is over, and it named nothing.
			return "", false
		}
		if v, ok := strings.CutPrefix(line, "# Version:"); ok {
			v = strings.TrimSpace(v)
			if v == "" || v[0] < '0' || v[0] > '9' {
				return "", false
			}
			return v, true
		}
	}
	return "", false
}
