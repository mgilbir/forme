package ucd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Char is what UnicodeData.txt says about one code point: the fields a
// generator here asks about, and none of the others.
type Char struct {
	// Category is the General_Category, "Lu", "Mn" and so on.
	Category string
	// Upper, Lower and Title are the simple case mappings, or zero where the
	// file gives none. Title is the file's own field, which is empty for most
	// characters; UnicodeData reads an empty one as "the same as Upper", as
	// UAX #44 defines it, so a caller never has to know that rule.
	Upper, Lower, Title rune
}

// UnicodeData reads UnicodeData.txt: every assigned code point, including the
// ones the file states as a "<..., First>" and "<..., Last>" pair of lines
// rather than one line each — the ideographs, the Hangul syllables, the
// private use areas. A generator that walked the lines alone would see two
// characters of each of those blocks, and every property of the rest would be
// whatever its default said.
//
// It exists so that a generator can answer a property question from the
// database it was pointed at. The alternative every generator here once took
// was Go's package unicode, which is whichever release the toolchain shipped —
// 15.0.0 for Go 1.26 — while the tables said 17.0.0: case pairs and combining
// marks that Unicode added in between were missing from tables labelled with
// the release that added them. See cmd/generators_test.go, which keeps package
// unicode out of every generator.
func UnicodeData(path string) (map[rune]Char, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[rune]Char{}
	first := rune(-1)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.TrimSpace(text) == "" {
			continue
		}
		fields := strings.Split(text, ";")
		if len(fields) != 15 {
			return nil, fmt.Errorf("%s:%d: %d fields where UnicodeData.txt has 15", path, line, len(fields))
		}
		r, err := codePoint(fields[0])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, line, err)
		}
		c := Char{Category: fields[2]}
		for _, m := range []struct {
			field string
			into  *rune
		}{{fields[12], &c.Upper}, {fields[13], &c.Lower}, {fields[14], &c.Title}} {
			if m.field == "" {
				continue
			}
			if *m.into, err = codePoint(m.field); err != nil {
				return nil, fmt.Errorf("%s:%d: %v", path, line, err)
			}
		}
		if c.Title == 0 {
			c.Title = c.Upper
		}
		switch name := fields[1]; {
		case strings.HasSuffix(name, ", First>"):
			if first >= 0 {
				return nil, fmt.Errorf("%s:%d: a range opens inside another", path, line)
			}
			first = r
			out[r] = c
		case strings.HasSuffix(name, ", Last>"):
			if first < 0 || first > r || out[first] != c {
				return nil, fmt.Errorf("%s:%d: a range closes that did not open, "+
					"or closes with properties it did not open with", path, line)
			}
			for k := first; k <= r; k++ {
				out[k] = c
			}
			first = -1
		default:
			if first >= 0 {
				return nil, fmt.Errorf("%s:%d: a range is still open", path, line)
			}
			out[r] = c
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if first >= 0 {
		return nil, fmt.Errorf("%s: the file ends inside a range", path)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no characters", path)
	}
	return out, nil
}

func codePoint(s string) (rune, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil {
		return 0, err
	}
	if v > 0x10FFFF {
		return 0, fmt.Errorf("U+%X is past the end of the code space", v)
	}
	return rune(v), nil
}
