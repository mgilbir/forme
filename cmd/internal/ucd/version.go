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

// Check verifies that every one of the given files that declares a version
// declares this one, and that at least one of them does when any could.
//
// Two files from two releases is the failure it exists for: a table built from
// LineBreak.txt of one release and Scripts.txt of another is wrong for every
// character the two disagree about, and it is wrong in a way no test downstream
// would name. A caller that passes only UnicodeData.txt gets no check, because
// there is nothing there to check against.
func Check(want string, paths ...string) error {
	if want == "" {
		return fmt.Errorf("no Unicode version given; pass -version, which the " +
			"Makefile fills in from UNICODE_VERSION")
	}
	for _, p := range paths {
		got, ok := Declared(p)
		if !ok {
			continue
		}
		if got != want {
			return fmt.Errorf("%s is from Unicode %s and this run was told %s;\n"+
				"the files are not all from the release the output would name",
				p, got, want)
		}
	}
	return nil
}
