package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A glyph's 'rtlm' form, held to HarfBuzz.
//
// testdata/harfbuzz/mirroredform.py shapes every character each face maps,
// alone and left to right, with and without 'rtlm' turned on for the run, and
// writes down each character whose glyph the feature changes and what to. Here
// every character the face maps is asked Face.MirroredForm: a listed one must
// answer its glyph, and every other one must answer none. The faces are
// MirroredForms.ttf, built by mirroredform_fixture.py to state the feature in
// each way the answer depends on; the suite's radical-rtlm.woff (WPT_TESTS);
// and Noto Sans Math, which states 182 forms, and STIX Two Math, which states
// none, from the Google Fonts library (`make googlefonts`). A corpus that is
// not there skips its face and says so.

type hbMirrored struct {
	name, sum string
	forms     map[rune]int
	skipped   int
	asked     int
}

func readMirroredGolden(t *testing.T) []*hbMirrored {
	t.Helper()
	path := filepath.Join(harfbuzzDir, "mirroredform.expected.txt")
	refuseUnpinnedOracle(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRun `make hbmirroredform` to generate it.", path, err)
	}
	defer file.Close()
	var faces []*hbMirrored
	var cur *hbMirrored
	sc := bufio.NewScanner(file)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		kind, rest, _ := strings.Cut(text, " ")
		if kind == "face" {
			name, sum, _ := strings.Cut(rest, " ")
			cur = &hbMirrored{name: name, sum: sum, forms: map[rune]int{}}
			faces = append(faces, cur)
			continue
		}
		if cur == nil {
			t.Fatalf("%s:%d: %q before any face", path, line, kind)
		}
		var n []int
		for _, f := range strings.Fields(rest) {
			v, err := strconv.Atoi(f)
			if err != nil {
				t.Fatalf("%s:%d: %v", path, line, err)
			}
			n = append(n, v)
		}
		switch {
		case kind == "M" && len(n) == 2:
			cur.forms[rune(n[0])] = n[1]
		case kind == "S" && len(n) == 1:
			cur.skipped = n[0]
		case kind == "N" && len(n) == 1:
			cur.asked = n[0]
		default:
			t.Fatalf("%s:%d: cannot read %q", path, line, text)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return faces
}

func TestMirroredFormAgreesWithHarfBuzz(t *testing.T) {
	faces := readMirroredGolden(t)
	if len(faces) != 4 {
		t.Fatalf("the expectations hold %d faces, want 4", len(faces))
	}
	for _, want := range faces {
		t.Run(want.name, func(t *testing.T) {
			var data []byte
			if want.name == "MirroredForms.ttf" {
				data = harfbuzzFont(t, want.name)
			} else {
				data = mathFaceFile(t, want.name)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != want.sum {
				t.Fatalf("%s is not the file the expectations were generated from "+
					"(sha256 %s, want %s); regenerate with `make hbmirroredform`", want.name, got, want.sum)
			}
			face, err := Load(data)
			if err != nil {
				t.Fatal(err)
			}
			asked, errs := 0, 0
			for r, g := range face.Cmap() {
				if r >= 0xD800 && r <= 0xDFFF {
					continue
				}
				// A character whose glyph shaping changed before HarfBuzz
				// could be asked about 'rtlm' was left out of the file. It
				// is the same question this face is asked, so it is left
				// out here the same way.
				if gs, missing := face.ShapeGlyphs(string(r)); missing != 0 || len(gs) != 1 || gs[0].GID != g {
					continue
				}
				asked++
				got, ok := face.MirroredForm(r)
				wantG, stated := want.forms[r]
				if ok != stated || (ok && got != wantG) {
					if errs++; errs <= 20 {
						t.Errorf("MirroredForm(U+%04X) = %d (%v), want %d (%v)", r, got, ok, wantG, stated)
					}
				}
			}
			if errs > 20 {
				t.Errorf("and %d more", errs-20)
			}
			if asked != want.asked {
				t.Errorf("asked about %d characters, and HarfBuzz was asked about %d", asked, want.asked)
			}
		})
	}
}
