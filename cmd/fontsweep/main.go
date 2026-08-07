// Read every font in a directory and say what happened.
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
package main

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
			switch strings.ToLower(filepath.Ext(p)) {
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
	var loaded, failed, panicked int
	reasons := map[string]int{}
	for _, p := range paths {
		r := read(p)
		switch r.result {
		case "ok":
			loaded++
		case "panic":
			panicked++
			reasons[r.detail]++
		default:
			failed++
			reasons[r.detail]++
		}
		fmt.Printf("%s\t%s\t%s\t%d\t%d\t%t\t%d\t%s\t%s\n",
			p, r.result, r.tables, r.upem, r.glyphs, r.cff, r.scripts, r.name, r.detail)
	}

	fmt.Fprintf(os.Stderr, "\n%d fonts: %d loaded, %d refused, %d panicked\n",
		len(paths), loaded, failed, panicked)
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

type row struct {
	result  string // ok, refused, panic, unreadable
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
		return row{result: "unreadable", detail: err.Error()}
	}
	r.tables = interesting(data)
	defer func() {
		if p := recover(); p != nil {
			r.result, r.detail = "panic", fmt.Sprint(p)
			r.detail = strings.SplitN(r.detail, "\n", 2)[0]
		}
	}()
	f, err := shape.Load(data)
	if err != nil {
		return row{result: "refused", detail: err.Error(), tables: r.tables}
	}
	return row{
		result: "ok", tables: r.tables,
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
