// Command fontsweep reads every font in a directory and says what happened.
//
// The six faces the checked-in oracle uses are chosen, reviewed and small
// enough to vendor, which is what makes them an oracle. They are also six. A
// reader that is right about them can still be wrong about the shapes nobody
// picked — a table longer than a format's own limit, an axis record a
// specification added later, a charstring operator only one foundry emits — and
// the only way to find that out is to read a great many fonts that nobody chose.
//
// So this reads a directory of them and reports, per font, whether it loaded and
// what it looked like. It asserts nothing: there is no expected answer for an
// arbitrary font, and the value is in the distribution rather than in any one
// line. What it is for is sizing a gap — how many faces a limitation actually
// costs — before deciding whether to close it.
//
//	go run ./cmd/fontsweep testdata/googlefonts/ofl > sweep.tsv
//
// A panic is reported rather than allowed to end the run, because one font that
// crashes the reader must not hide the three thousand after it. Every panic here
// is a defect, and the fuzzer in shape/panic_test.go is where it should end up.
//
// Each file lands in one of five buckets, because they are five answers and the
// count of each is what the sweep is for: loaded, refused by shape.Load,
// panicked, a font collection — a .ttc or .otc, several faces in one file,
// which nothing in this module reads — and unreadable, a file the operating
// system would not hand over. The last two used to be counted as refused, so a
// directory of collections sized a gap in the reader that was a format it does
// not claim to read.
package main

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fontsweep <directory>...")
		os.Exit(2)
	}
	var paths []string
	for _, root := range os.Args[1:] {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			switch ascii.Lower(filepath.Ext(p)) {
			case ".ttf", ".otf", ".ttc", ".otc":
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "walking %s: %v\n", root, err)
			os.Exit(1)
		}
	}
	sort.Strings(paths)

	fmt.Println("path\tresult\ttables\tupem\tglyphs\tcff\tscripts\tname\tdetail")
	var rows []row
	for _, p := range paths {
		r := read(p)
		rows = append(rows, r)
		fmt.Printf("%s\t%s\t%s\t%d\t%d\t%t\t%d\t%s\t%s\n",
			p, r.result, r.tables, r.upem, r.glyphs, r.cff, r.scripts, r.name, r.detail)
	}

	counts, reasons := tally(rows)
	fmt.Fprintf(os.Stderr, "\n%d files: %d loaded, %d refused, %d panicked, "+
		"%d collections (not read), %d unreadable\n",
		len(paths), counts[resultOK], counts[resultRefused], counts[resultPanic],
		counts[resultCollection], counts[resultUnreadable])
	if len(reasons) > 0 {
		fmt.Fprintln(os.Stderr, "\nby reason:")
		type kv struct {
			k string
			n int
		}
		var all []kv
		for k, n := range reasons {
			all = append(all, kv{k, n})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
		for _, e := range all {
			fmt.Fprintf(os.Stderr, "%6d  %s\n", e.n, e.k)
		}
	}
}

// The buckets a file lands in.
const (
	resultOK         = "ok"
	resultRefused    = "refused"
	resultPanic      = "panic"
	resultCollection = "collection"
	resultUnreadable = "unreadable"
)

// tally counts the rows by result, and the refusals and panics by what they
// said. A collection or an unreadable file is not a reason the reader gave, so
// it is not among the reasons.
func tally(rows []row) (map[string]int, map[string]int) {
	counts, reasons := map[string]int{}, map[string]int{}
	for _, r := range rows {
		counts[r.result]++
		if r.result == resultRefused || r.result == resultPanic {
			reasons[r.detail]++
		}
	}
	return counts, reasons
}

type row struct {
	result  string // one of the result constants above
	detail  string
	tables  string // the tags that decide what kind of font this is
	upem    int
	glyphs  int
	cff     bool
	scripts int
	name    string
}

// read loads one font, surviving whatever it does.
//
// The recover is not defensive programming for its own sake. A reader of
// untrusted bytes that panics is reporting a defect, and the run has to continue
// past it for the report to be worth anything.
func read(path string) (r row) {
	data, err := os.ReadFile(path)
	if err != nil {
		return row{result: resultUnreadable, detail: err.Error()}
	}
	r.tables = interesting(data)
	if len(data) >= 12 && string(data[:4]) == "ttcf" {
		// Several faces in one file, which shape.Load does not read; asking it
		// would count a format this module does not claim as a font it refused.
		return row{result: resultCollection, tables: r.tables,
			detail: fmt.Sprintf("%d faces", binary.BigEndian.Uint32(data[8:]))}
	}
	defer func() {
		if p := recover(); p != nil {
			r.result, r.detail = resultPanic, fmt.Sprint(p)
			r.detail = strings.SplitN(r.detail, "\n", 2)[0]
		}
	}()
	f, err := shape.Load(data)
	if err != nil {
		return row{result: resultRefused, detail: err.Error(), tables: r.tables}
	}
	return row{
		result: resultOK, tables: r.tables,
		upem: f.UnitsPerEm(), glyphs: f.NumGlyphs(), cff: f.IsCFF(),
		scripts: len(f.Scripts()), name: f.Name(),
	}
}

// interesting names the tables that say what kind of font this is, so a refusal
// can be read against the shape of the file rather than against its name.
//
// fvar is the one that matters most here: a face with it is variable, and what
// this module hands back is the outlines as stored, which is the default
// instance. How many faces that is, and how far their default sits from where a
// caller would want them, is the question the variable-font issue asks.
func interesting(data []byte) string {
	var have []string
	for _, tag := range []string{"fvar", "gvar", "avar", "CFF ", "CFF2", "glyf", "GSUB", "GPOS"} {
		if hasTable(data, tag) {
			have = append(have, strings.TrimSpace(tag))
		}
	}
	return strings.Join(have, ",")
}

// hasTable reports whether an sfnt's table directory names a tag. It reads only
// the directory, so a font this module refuses is still described.
func hasTable(data []byte, tag string) bool {
	if len(data) < 12 {
		return false
	}
	off := 0
	if string(data[:4]) == "ttcf" {
		if len(data) < 16 {
			return false
		}
		off = int(binary.BigEndian.Uint32(data[12:]))
		if off < 0 || off+12 > len(data) {
			return false
		}
	}
	n := int(binary.BigEndian.Uint16(data[off+4:]))
	for i := 0; i < n; i++ {
		e := off + 12 + i*16
		if e+4 > len(data) {
			return false
		}
		if string(data[e:e+4]) == tag {
			return true
		}
	}
	return false
}
